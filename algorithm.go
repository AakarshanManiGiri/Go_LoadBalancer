package main

import (
	"fmt"
	"sync"
)

// RoutingAlgorithm defines the interface for routing algorithms
type RoutingAlgorithm interface {
	// SelectBackend selects a backend from the pool
	SelectBackend(pool *BackendPool) (*Backend, error)
	// Name returns the name of the algorithm
	Name() string
}

// RoundRobinAlgorithm implements round-robin routing
type RoundRobinAlgorithm struct {
	mu    sync.Mutex
	index int
}

// SelectBackend selects the next backend in the round-robin order
func (rr *RoundRobinAlgorithm) SelectBackend(pool *BackendPool) (*Backend, error) {
	healthy := pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy backends available")
	}

	rr.mu.Lock()
	rr.index = (rr.index + 1) % len(healthy)
	idx := rr.index
	rr.mu.Unlock()

	return healthy[idx], nil
}

// Name returns the algorithm name
func (rr *RoundRobinAlgorithm) Name() string {
	return "round_robin"
}

// WeightedRoundRobinAlgorithm implements weighted round-robin routing
type WeightedRoundRobinAlgorithm struct {
	mu            sync.Mutex
	index         int
	totalWeight   int
	currentWeight int
}

// SelectBackend selects a backend based on weight
func (wrr *WeightedRoundRobinAlgorithm) SelectBackend(pool *BackendPool) (*Backend, error) {
	healthy := pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy backends available")
	}

	wrr.mu.Lock()
	defer wrr.mu.Unlock()

	// Calculate total weight on first call
	if wrr.totalWeight == 0 {
		for _, b := range healthy {
			wrr.totalWeight += b.Weight
		}
	}

	// Smooth weighted round-robin algorithm
	maxWeight := 0
	var selectedBackend *Backend

	for _, b := range healthy {
		b.mu.Lock()
		b.Weight += b.Weight
		if b.Weight > maxWeight {
			maxWeight = b.Weight
			selectedBackend = b
		}
		b.mu.Unlock()
	}

	if selectedBackend != nil {
		selectedBackend.mu.Lock()
		selectedBackend.Weight -= wrr.totalWeight
		selectedBackend.mu.Unlock()
	}

	return selectedBackend, nil
}

// Name returns the algorithm name
func (wrr *WeightedRoundRobinAlgorithm) Name() string {
	return "weighted_round_robin"
}

// LeastConnectionsAlgorithm implements least connections routing
type LeastConnectionsAlgorithm struct{}

// SelectBackend selects the backend with the least connections
func (lc *LeastConnectionsAlgorithm) SelectBackend(pool *BackendPool) (*Backend, error) {
	healthy := pool.GetHealthyBackends()
	if len(healthy) == 0 {
		return nil, fmt.Errorf("no healthy backends available")
	}

	var selected *Backend
	minConns := int(^uint(0) >> 1) // Max int

	for _, b := range healthy {
		conns := b.GetConnections()
		if conns < minConns {
			minConns = conns
			selected = b
		}
	}

	return selected, nil
}

// Name returns the algorithm name
func (lc *LeastConnectionsAlgorithm) Name() string {
	return "least_connections"
}

// NewRoutingAlgorithm creates a new routing algorithm based on the name
func NewRoutingAlgorithm(name string) (RoutingAlgorithm, error) {
	switch name {
	case "round_robin":
		return &RoundRobinAlgorithm{}, nil
	case "weighted_round_robin":
		return &WeightedRoundRobinAlgorithm{}, nil
	case "least_connections":
		return &LeastConnectionsAlgorithm{}, nil
	default:
		return nil, fmt.Errorf("unknown routing algorithm: %s", name)
	}
}
