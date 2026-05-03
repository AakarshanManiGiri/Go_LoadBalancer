package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthChecker performs health checks on backends
type HealthChecker struct {
	pool         *BackendPool
	config       HealthCheckConfig
	client       *http.Client
	stopChan     chan struct{}
	wg           sync.WaitGroup
	failureCount map[string]int
	successCount map[string]int
	mu           sync.Mutex
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(pool *BackendPool, config HealthCheckConfig) *HealthChecker {
	if config.Method == "" {
		config.Method = "GET"
	}
	if config.Path == "" {
		config.Path = "/"
	}
	if config.ExpectedStatus == 0 {
		config.ExpectedStatus = 200
	}

	hc := &HealthChecker{
		pool:         pool,
		config:       config,
		stopChan:     make(chan struct{}),
		failureCount: make(map[string]int),
		successCount: make(map[string]int),
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}

	return hc
}

// Start begins health checking
func (hc *HealthChecker) Start() {
	if !hc.config.Enabled {
		return
	}

	hc.wg.Add(1)
	go hc.checkLoop()
}

// Stop stops health checking
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
}

// checkLoop performs periodic health checks
func (hc *HealthChecker) checkLoop() {
	defer hc.wg.Done()

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-hc.stopChan:
			return
		case <-ticker.C:
			hc.checkAllBackends()
		}
	}
}

// checkAllBackends checks all backends
func (hc *HealthChecker) checkAllBackends() {
	backends := hc.pool.GetBackends()
	var wg sync.WaitGroup

	for _, b := range backends {
		wg.Add(1)
		go func(backend *Backend) {
			defer wg.Done()
			hc.checkBackend(backend)
		}(b)
	}

	wg.Wait()
}

// checkBackend checks a single backend
func (hc *HealthChecker) checkBackend(backend *Backend) {
	healthy := hc.performHealthCheck(backend)

	hc.mu.Lock()
	defer hc.mu.Unlock()

	if healthy {
		hc.successCount[backend.ID]++
		hc.failureCount[backend.ID] = 0

		// Mark healthy if threshold is reached
		if hc.successCount[backend.ID] >= hc.config.HealthyThreshold && !backend.IsHealthy() {
			hc.pool.MarkHealthy(backend)
			fmt.Printf("[HEALTH] Backend %s marked HEALTHY\n", backend.ID)
		}
	} else {
		hc.failureCount[backend.ID]++
		hc.successCount[backend.ID] = 0

		// Mark unhealthy if threshold is reached
		if hc.failureCount[backend.ID] >= hc.config.UnhealthyThreshold && backend.IsHealthy() {
			hc.pool.MarkUnhealthy(backend)
			fmt.Printf("[HEALTH] Backend %s marked UNHEALTHY\n", backend.ID)
		}
	}
}

// performHealthCheck performs a single health check
func (hc *HealthChecker) performHealthCheck(backend *Backend) bool {
	url := backend.GetURL().String() + hc.config.Path

	ctx, cancel := context.WithTimeout(context.Background(), hc.config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, hc.config.Method, url, nil)
	if err != nil {
		return false
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == hc.config.ExpectedStatus
}

// OnProxyError is called when a proxy error occurs (passive health check)
func (hc *HealthChecker) OnProxyError(backend *Backend) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	hc.failureCount[backend.ID]++
	hc.successCount[backend.ID] = 0

	if hc.failureCount[backend.ID] >= hc.config.UnhealthyThreshold && backend.IsHealthy() {
		hc.pool.MarkUnhealthy(backend)
		fmt.Printf("[PASSIVE HEALTH] Backend %s marked UNHEALTHY after %d errors\n", backend.ID, hc.failureCount[backend.ID])
	}
}
