package main

import (
	"fmt"
	"net/url"
	"sync"
	"time"
)

// Backend represents a backend server
type Backend struct {
	ID           string
	URL          *url.URL
	Healthy      bool
	Connections  int
	Weight       int
	MaxConns     int
	mu           sync.RWMutex
	LastHealthAt time.Time
}

// BackendPool manages a pool of backend servers
type BackendPool struct {
	backends        []*Backend
	mu              sync.RWMutex
	currentIndex    int // For round-robin
	healthyCount    int
	lastUpdatedTime time.Time
}

// NewBackendPool creates a new backend pool
func NewBackendPool(configs []BackendConfig) (*BackendPool, error) {
	pool := &BackendPool{
		backends: make([]*Backend, 0, len(configs)),
	}

	for _, cfg := range configs {
		urlStr := fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
		u, err := url.Parse(urlStr)
		if err != nil {
			return nil, fmt.Errorf("invalid backend URL %s: %w", urlStr, err)
		}

		backend := &Backend{
			ID:       cfg.ID,
			URL:      u,
			Healthy:  true,
			Weight:   cfg.Weight,
			MaxConns: cfg.MaxConns,
		}
		if backend.Weight == 0 {
			backend.Weight = 1
		}
		pool.backends = append(pool.backends, backend)
	}

	pool.healthyCount = len(pool.backends)
	return pool, nil
}

// GetBackends returns a copy of all backends
func (p *BackendPool) GetBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	backends := make([]*Backend, len(p.backends))
	copy(backends, p.backends)
	return backends
}

// GetHealthyBackends returns only healthy backends
func (p *BackendPool) GetHealthyBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healthy := make([]*Backend, 0, len(p.backends))
	for _, b := range p.backends {
		if b.IsHealthy() {
			healthy = append(healthy, b)
		}
	}
	return healthy
}

// MarkHealthy marks a backend as healthy
func (p *BackendPool) MarkHealthy(backend *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()

	backend.mu.Lock()
	wasHealthy := backend.Healthy
	backend.Healthy = true
	backend.LastHealthAt = time.Now()
	backend.mu.Unlock()

	if !wasHealthy {
		p.healthyCount++
	}
}

// MarkUnhealthy marks a backend as unhealthy
func (p *BackendPool) MarkUnhealthy(backend *Backend) {
	p.mu.Lock()
	defer p.mu.Unlock()

	backend.mu.Lock()
	wasHealthy := backend.Healthy
	backend.Healthy = false
	backend.LastHealthAt = time.Now()
	backend.mu.Unlock()

	if wasHealthy {
		p.healthyCount--
	}
}

// HealthyCount returns the number of healthy backends
func (p *BackendPool) HealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.healthyCount
}

// IncrementConnections increments the connection count for a backend
func (b *Backend) IncrementConnections() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.MaxConns > 0 && b.Connections >= b.MaxConns {
		return fmt.Errorf("backend %s has reached max connections", b.ID)
	}
	b.Connections++
	return nil
}

// DecrementConnections decrements the connection count for a backend
func (b *Backend) DecrementConnections() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Connections > 0 {
		b.Connections--
	}
}

// GetConnections returns the current connection count
func (b *Backend) GetConnections() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Connections
}

// IsHealthy returns whether the backend is healthy
func (b *Backend) IsHealthy() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Healthy
}

// GetURL returns the backend URL
func (b *Backend) GetURL() *url.URL {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.URL
}

// String returns a string representation of the backend
func (b *Backend) String() string {
	return fmt.Sprintf("%s (%s, healthy=%v, conns=%d)", b.ID, b.URL.String(), b.IsHealthy(), b.GetConnections())
}
