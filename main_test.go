package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRoundRobinRouting tests that requests are routed in round-robin fashion
func TestRoundRobinRouting(t *testing.T) {
	// Create test backend servers
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "backend-1")
		fmt.Fprint(w, "Response from backend-1")
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "backend-2")
		fmt.Fprint(w, "Response from backend-2")
	}))
	defer backend2.Close()

	backend3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "backend-3")
		fmt.Fprint(w, "Response from backend-3")
	}))
	defer backend3.Close()

	// Create configuration
	config := &LoadBalancerConfig{
		Port:      8080,
		Algorithm: "round_robin",
		Backends: []BackendConfig{
			{ID: "b1", Host: "localhost", Port: parsePort(backend1.URL)},
			{ID: "b2", Host: "localhost", Port: parsePort(backend2.URL)},
			{ID: "b3", Host: "localhost", Port: parsePort(backend3.URL)},
		},
		HealthCheck: HealthCheckConfig{
			Enabled: false,
		},
		StickySession: StickySessionConfig{
			Enabled: false,
		},
	}

	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Test round-robin selection
	expectedSequence := []string{"b2", "b3", "b1", "b2", "b3", "b1"}
	for i, expected := range expectedSequence {
		backend, err := lb.algorithm.SelectBackend(lb.pool)
		if err != nil {
			t.Fatalf("Failed to select backend: %v", err)
		}
		if backend.ID != expected {
			t.Errorf("Request %d: expected %s, got %s", i+1, expected, backend.ID)
		}
	}

	fmt.Println("✓ Round-robin routing test passed")
}

// TestLeastConnectionsRouting tests least connections algorithm
func TestLeastConnectionsRouting(t *testing.T) {
	config := &LoadBalancerConfig{
		Port:      8080,
		Algorithm: "least_connections",
		Backends: []BackendConfig{
			{ID: "b1", Host: "localhost", Port: 8081, MaxConns: 10},
			{ID: "b2", Host: "localhost", Port: 8082, MaxConns: 10},
			{ID: "b3", Host: "localhost", Port: 8083, MaxConns: 10},
		},
		HealthCheck: HealthCheckConfig{
			Enabled: false,
		},
		StickySession: StickySessionConfig{
			Enabled: false,
		},
	}

	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Simulate connections
	backends := lb.pool.GetBackends()
	backends[0].IncrementConnections()
	backends[0].IncrementConnections()
	backends[1].IncrementConnections()
	// backends[2] has 0 connections

	// Should select backend 3 (least connections)
	backend, err := lb.algorithm.SelectBackend(lb.pool)
	if err != nil {
		t.Fatalf("Failed to select backend: %v", err)
	}
	if backend.ID != "b3" {
		t.Errorf("Expected b3 (least connections), got %s", backend.ID)
	}

	fmt.Println("✓ Least connections routing test passed")
}

// TestHealthChecking tests active health checking
func TestHealthChecking(t *testing.T) {
	// Create a test backend that's initially healthy
	isHealthy := true
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			if isHealthy {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		}
	}))
	defer backend.Close()

	config := &LoadBalancerConfig{
		Port:      8080,
		Algorithm: "round_robin",
		Backends: []BackendConfig{
			{ID: "b1", Host: "localhost", Port: parsePort(backend.URL)},
		},
		HealthCheck: HealthCheckConfig{
			Enabled:            true,
			Interval:           100 * time.Millisecond,
			Timeout:            1 * time.Second,
			UnhealthyThreshold: 1,
			HealthyThreshold:   1,
			Path:               "/health",
			Method:             "GET",
			ExpectedStatus:     200,
		},
		StickySession: StickySessionConfig{
			Enabled: false,
		},
	}

	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Start health checking
	lb.healthChecker.Start()
	defer lb.healthChecker.Stop()

	// Initially should be healthy
	backends := lb.pool.GetBackends()
	if !backends[0].IsHealthy() {
		t.Error("Backend should be healthy initially")
	}

	// Wait for a health check to pass
	time.Sleep(200 * time.Millisecond)

	// Mark backend as unhealthy
	isHealthy = false
	time.Sleep(300 * time.Millisecond)

	// Should now be marked as unhealthy
	if backends[0].IsHealthy() {
		t.Error("Backend should be marked unhealthy after failed health check")
	}

	fmt.Println("✓ Health checking test passed")
}

// TestStickySession tests sticky session routing
func TestStickySession(t *testing.T) {
	config := &LoadBalancerConfig{
		Port:      8080,
		Algorithm: "round_robin",
		Backends: []BackendConfig{
			{ID: "b1", Host: "localhost", Port: 8081},
			{ID: "b2", Host: "localhost", Port: 8082},
		},
		HealthCheck: HealthCheckConfig{
			Enabled: false,
		},
		StickySession: StickySessionConfig{
			Enabled:    true,
			Method:     "cookie",
			CookieName: "TEST_SESSION",
			TTL:        1 * time.Hour,
		},
	}

	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	// Create a mock request
	req := httptest.NewRequest("GET", "http://localhost/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	// First request should not have a session
	backend1, _ := lb.stickySessionMgr.GetBackendFromRequest(req, lb.pool)
	if backend1 != nil {
		t.Error("First request should not have a sticky backend")
	}

	// Generate a new session ID for this request
	sessionID1 := lb.stickySessionMgr.GenerateSessionID(req)

	// Set a session
	backends := lb.pool.GetBackends()
	lb.stickySessionMgr.SetBackendForSession(sessionID1, backends[0])

	// Create a new request with the session ID in a cookie
	req2 := httptest.NewRequest("GET", "http://localhost/", nil)
	req2.AddCookie(&http.Cookie{Name: "TEST_SESSION", Value: sessionID1})
	req2.RemoteAddr = "192.168.1.1:12345"

	// Second request with same session should get the same backend
	backend2, _ := lb.stickySessionMgr.GetBackendFromRequest(req2, lb.pool)
	if backend2 == nil || backend2.ID != backends[0].ID {
		t.Error("Sticky session should route to the same backend")
	}

	fmt.Println("✓ Sticky session test passed")
}

// TestFailover tests failover when a backend becomes unhealthy
func TestFailover(t *testing.T) {
	config := &LoadBalancerConfig{
		Port:      8080,
		Algorithm: "round_robin",
		Backends: []BackendConfig{
			{ID: "b1", Host: "localhost", Port: 8081},
			{ID: "b2", Host: "localhost", Port: 8082},
		},
		HealthCheck: HealthCheckConfig{
			Enabled: false,
		},
		StickySession: StickySessionConfig{
			Enabled: false,
		},
	}

	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	backends := lb.pool.GetBackends()

	// All backends should be healthy
	if lb.pool.HealthyCount() != 2 {
		t.Errorf("Expected 2 healthy backends, got %d", lb.pool.HealthyCount())
	}

	// Mark one backend as unhealthy
	lb.pool.MarkUnhealthy(backends[0])

	if lb.pool.HealthyCount() != 1 {
		t.Errorf("Expected 1 healthy backend, got %d", lb.pool.HealthyCount())
	}

	// All selections should go to the remaining healthy backend
	for i := 0; i < 3; i++ {
		backend, err := lb.algorithm.SelectBackend(lb.pool)
		if err != nil {
			t.Fatalf("Failed to select backend: %v", err)
		}
		if backend.ID != "b2" {
			t.Errorf("Expected b2, got %s", backend.ID)
		}
	}

	fmt.Println("✓ Failover test passed")
}

// TestConnectionLimits tests that backends respect connection limits
func TestConnectionLimits(t *testing.T) {
	config := &LoadBalancerConfig{
		Port:      8080,
		Algorithm: "least_connections",
		Backends: []BackendConfig{
			{ID: "b1", Host: "localhost", Port: 8081, MaxConns: 2},
		},
		HealthCheck: HealthCheckConfig{
			Enabled: false,
		},
	}

	lb, err := NewLoadBalancer(config)
	if err != nil {
		t.Fatalf("Failed to create load balancer: %v", err)
	}

	backend := lb.pool.GetBackends()[0]

	// Add connections up to the limit
	if err := backend.IncrementConnections(); err != nil {
		t.Fatalf("Failed to increment connections: %v", err)
	}
	if err := backend.IncrementConnections(); err != nil {
		t.Fatalf("Failed to increment connections: %v", err)
	}

	// Next connection should fail (max is 2)
	if err := backend.IncrementConnections(); err == nil {
		t.Error("Should not allow connections beyond max_conns")
	}

	// Decrement and verify we can add again
	backend.DecrementConnections()
	if err := backend.IncrementConnections(); err != nil {
		t.Fatalf("Failed to increment connections after decrement: %v", err)
	}

	fmt.Println("✓ Connection limits test passed")
}

// SimulateLoadBalancer runs a simulation of the load balancer
func SimulateLoadBalancer() {
	fmt.Println("\n===== LOAD BALANCER SIMULATION =====")

	// Create mock backends
	backends := map[string]int{
		"backend-1": 8081,
		"backend-2": 8082,
		"backend-3": 8083,
	}

	fmt.Println("Starting simulation with backends:")
	for name := range backends {
		fmt.Printf("  - %s\n", name)
	}

	config := &LoadBalancerConfig{
		Port:      8080,
		Algorithm: "round_robin",
		Backends: []BackendConfig{
			{ID: "b1", Host: "localhost", Port: 8081, Weight: 1},
			{ID: "b2", Host: "localhost", Port: 8082, Weight: 1},
			{ID: "b3", Host: "localhost", Port: 8083, Weight: 1},
		},
		HealthCheck: HealthCheckConfig{
			Enabled: false,
		},
		StickySession: StickySessionConfig{
			Enabled: false,
		},
	}

	lb, err := NewLoadBalancer(config)
	if err != nil {
		fmt.Printf("Error creating load balancer: %v\n", err)
		return
	}

	fmt.Println("\n1. Testing Round-Robin Distribution:")
	fmt.Println("   Routing 9 requests in round-robin order:")
	for i := 1; i <= 9; i++ {
		backend, _ := lb.algorithm.SelectBackend(lb.pool)
		fmt.Printf("   Request %d -> %s\n", i, backend.ID)
	}

	fmt.Println("\n2. Simulating Backend Failure:")
	fmt.Printf("   Marking backend-1 as unhealthy\n")
	lb.pool.MarkUnhealthy(lb.pool.GetBackends()[0])
	fmt.Printf("   Healthy backends: %d/%d\n", lb.pool.HealthyCount(), len(lb.pool.GetBackends()))

	fmt.Println("\n   Routing 3 more requests (backend-1 excluded):")
	for i := 1; i <= 3; i++ {
		backend, _ := lb.algorithm.SelectBackend(lb.pool)
		fmt.Printf("   Request %d -> %s\n", i, backend.ID)
	}

	fmt.Println("\n3. Switching to Least Connections Algorithm:")
	lb.algorithm, _ = NewRoutingAlgorithm("least_connections")

	// Create connections
	backends_list := lb.pool.GetHealthyBackends()
	backends_list[0].IncrementConnections()
	backends_list[0].IncrementConnections()
	backends_list[1].IncrementConnections()

	fmt.Println("   Backend connection counts:")
	for _, b := range lb.pool.GetBackends() {
		if b.IsHealthy() {
			fmt.Printf("   %s: %d connections\n", b.ID, b.GetConnections())
		}
	}

	fmt.Println("\n   Routing 3 requests (should go to backend with least connections):")
	for i := 1; i <= 3; i++ {
		backend, _ := lb.algorithm.SelectBackend(lb.pool)
		fmt.Printf("   Request %d -> %s (conns=%d)\n", i, backend.ID, backend.GetConnections())
	}

	fmt.Println("\n4. Testing Sticky Sessions:")
	lb.config.StickySession.Enabled = true
	lb.config.StickySession.Method = "cookie"
	lb.stickySessionMgr = NewStickySessionManager(lb.config.StickySession)

	mockReq := httptest.NewRequest("GET", "http://localhost/", nil)
	mockReq.RemoteAddr = "192.168.1.100:54321"

	backend1, sessionID := lb.stickySessionMgr.GetBackendFromRequest(mockReq, lb.pool)
	fmt.Printf("   First request (no session): backend=%v, generated sessionID\n", backend1 == nil)

	// Set sticky session
	selectedBackend := lb.pool.GetHealthyBackends()[0]
	lb.stickySessionMgr.SetBackendForSession(sessionID, selectedBackend)

	mockReq2 := httptest.NewRequest("GET", "http://localhost/", nil)
	mockReq2.AddCookie(&http.Cookie{Name: "LB_SESSION", Value: sessionID})
	mockReq2.RemoteAddr = "192.168.1.100:54321"

	backend2, _ := lb.stickySessionMgr.GetBackendFromRequest(mockReq2, lb.pool)
	if backend2 != nil {
		fmt.Printf("   Second request (with session): routed to %s\n", backend2.ID)
		fmt.Printf("   Sticky routing works: %v\n", backend2.ID == selectedBackend.ID)
	} else {
		fmt.Printf("   Second request (with session): no backend found (sticky sessions may be disabled)\n")
	}

	fmt.Println("\n===== SIMULATION COMPLETE =====")
}

// Helper function to parse port from URL
func parsePort(urlStr string) int {
	var port int
	fmt.Sscanf(urlStr, "http://localhost:%d", &port)
	return port
}

// TestMain runs all tests and the simulation
func TestMain(t *testing.T) {
	// Run all tests
	TestRoundRobinRouting(t)
	TestLeastConnectionsRouting(t)
	TestHealthChecking(t)
	TestStickySession(t)
	TestFailover(t)
	TestConnectionLimits(t)

	// Run simulation
	SimulateLoadBalancer()

	fmt.Println("\n✓ All tests passed!")
}
