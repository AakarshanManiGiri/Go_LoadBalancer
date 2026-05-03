package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"
)

// LoadBalancer represents the HTTP load balancer
type LoadBalancer struct {
	config             *LoadBalancerConfig
	pool               *BackendPool
	algorithm          RoutingAlgorithm
	healthChecker      *HealthChecker
	stickySessionMgr   *StickySessionManager
	reverseProxies     map[string]*httputil.ReverseProxy
	mu                 sync.RWMutex
	passiveErrorCounts map[string]int // For passive health checks
	stopChan           chan struct{}
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(config *LoadBalancerConfig) (*LoadBalancer, error) {
	// Create backend pool
	pool, err := NewBackendPool(config.Backends)
	if err != nil {
		return nil, fmt.Errorf("failed to create backend pool: %w", err)
	}

	// Create routing algorithm
	algorithm, err := NewRoutingAlgorithm(config.Algorithm)
	if err != nil {
		return nil, fmt.Errorf("failed to create routing algorithm: %w", err)
	}

	// Create reverse proxies for each backend
	reverseProxies := make(map[string]*httputil.ReverseProxy)
	for _, backend := range pool.GetBackends() {
		proxy := httputil.NewSingleHostReverseProxy(backend.URL)
		// Customize the reverse proxy transport for better control
		proxy.Transport = &customTransport{
			roundTripper: http.DefaultTransport,
		}
		reverseProxies[backend.ID] = proxy
	}

	// Create health checker
	healthChecker := NewHealthChecker(pool, config.HealthCheck)

	// Create sticky session manager
	stickySessionMgr := NewStickySessionManager(config.StickySession)

	lb := &LoadBalancer{
		config:             config,
		pool:               pool,
		algorithm:          algorithm,
		healthChecker:      healthChecker,
		stickySessionMgr:   stickySessionMgr,
		reverseProxies:     reverseProxies,
		passiveErrorCounts: make(map[string]int),
		stopChan:           make(chan struct{}),
	}

	return lb, nil
}

// Start starts the load balancer
func (lb *LoadBalancer) Start() error {
	fmt.Printf("Starting load balancer on port %d with %s algorithm\n",
		lb.config.Port, lb.config.Algorithm)

	// Start health checking
	lb.healthChecker.Start()

	// Setup HTTP handler
	http.HandleFunc("/", lb.handleRequest)
	http.HandleFunc("/health", lb.handleHealthStatus)
	http.HandleFunc("/stats", lb.handleStats)

	addr := fmt.Sprintf(":%d", lb.config.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      http.DefaultServeMux,
		ReadTimeout:  lb.config.RequestTimeout,
		WriteTimeout: lb.config.RequestTimeout,
		IdleTimeout:  lb.config.IdleTimeout,
	}

	go func() {
		<-lb.stopChan
		server.Close()
	}()

	return server.ListenAndServe()
}

// Stop stops the load balancer
func (lb *LoadBalancer) Stop() {
	lb.healthChecker.Stop()
	close(lb.stopChan)
}

// handleRequest handles incoming requests
func (lb *LoadBalancer) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Try to get backend from sticky session
	sessionID := ""
	backend, sessionID := lb.stickySessionMgr.GetBackendFromRequest(r, lb.pool)

	// If no sticky backend or it's not healthy, select one using the algorithm
	if backend == nil {
		var err error
		backend, err = lb.algorithm.SelectBackend(lb.pool)
		if err != nil {
			http.Error(w, fmt.Sprintf("Service Unavailable: %v", err), http.StatusServiceUnavailable)
			return
		}

		// Generate session ID if sticky sessions are enabled
		if lb.config.StickySession.Enabled && sessionID == "" {
			sessionID = lb.stickySessionMgr.GenerateSessionID(r)
			lb.stickySessionMgr.SetBackendForSession(sessionID, backend)
		}
	}

	// Check connection limits for least connections algorithm
	if err := backend.IncrementConnections(); err != nil {
		http.Error(w, "Service Unavailable: Backend at capacity", http.StatusServiceUnavailable)
		return
	}
	defer backend.DecrementConnections()

	// Set session cookie if needed
	lb.stickySessionMgr.SetCookie(w, sessionID)

	// Proxy the request
	lb.proxyRequest(w, r, backend)
}

// proxyRequest proxies a request to a backend using reverse proxy
func (lb *LoadBalancer) proxyRequest(w http.ResponseWriter, r *http.Request, backend *Backend) {
	proxy, exists := lb.reverseProxies[backend.ID]
	if !exists {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Custom error handler for passive health checks
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		fmt.Printf("[ERROR] Error proxying to %s: %v\n", backend.ID, err)
		lb.healthChecker.OnProxyError(backend)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

// handleHealthStatus returns the status of all backends
func (lb *LoadBalancer) handleHealthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	backends := lb.pool.GetBackends()
	fmt.Fprintf(w, "{\n")
	fmt.Fprintf(w, "  \"load_balancer\": \"OK\",\n")
	fmt.Fprintf(w, "  \"algorithm\": \"%s\",\n", lb.algorithm.Name())
	fmt.Fprintf(w, "  \"healthy_backends\": %d,\n", lb.pool.HealthyCount())
	fmt.Fprintf(w, "  \"total_backends\": %d,\n", len(backends))
	fmt.Fprintf(w, "  \"backends\": [\n")

	for i, backend := range backends {
		fmt.Fprintf(w, "    {\n")
		fmt.Fprintf(w, "      \"id\": \"%s\",\n", backend.ID)
		fmt.Fprintf(w, "      \"url\": \"%s\",\n", backend.GetURL().String())
		fmt.Fprintf(w, "      \"healthy\": %v,\n", backend.IsHealthy())
		fmt.Fprintf(w, "      \"connections\": %d\n", backend.GetConnections())
		fmt.Fprintf(w, "    }")
		if i < len(backends)-1 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, "\n")
	}
	fmt.Fprintf(w, "  ]\n")
	fmt.Fprintf(w, "}\n")
}

// handleStats returns statistics about the load balancer
func (lb *LoadBalancer) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	backends := lb.pool.GetBackends()
	totalConns := 0
	for _, b := range backends {
		totalConns += b.GetConnections()
	}

	fmt.Fprintf(w, "{\n")
	fmt.Fprintf(w, "  \"total_connections\": %d,\n", totalConns)
	fmt.Fprintf(w, "  \"healthy_backends\": %d,\n", lb.pool.HealthyCount())
	fmt.Fprintf(w, "  \"total_backends\": %d,\n", len(backends))
	fmt.Fprintf(w, "  \"algorithm\": \"%s\",\n", lb.algorithm.Name())
	fmt.Fprintf(w, "  \"uptime\": \"N/A\"\n")
	fmt.Fprintf(w, "}\n")
}

// customTransport wraps http.Transport for custom error handling
type customTransport struct {
	roundTripper http.RoundTripper
}

// RoundTrip implements http.RoundTripper
func (ct *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return ct.roundTripper.RoundTrip(req)
}
