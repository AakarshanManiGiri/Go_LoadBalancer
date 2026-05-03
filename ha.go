package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// HAConfig represents high availability configuration
type HAConfig struct {
	Enabled           bool          `yaml:"enabled"`
	Mode              string        `yaml:"mode"` // "active_passive" or "active_active"
	NodeID            string        `yaml:"node_id"`
	HeartbeatFile     string        `yaml:"heartbeat_file"`
	LeaderLockFile    string        `yaml:"leader_lock_file"`
	StateStoreFile    string        `yaml:"state_store_file"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	HeartbeatTimeout  time.Duration `yaml:"heartbeat_timeout"`
}

// HACoordinator manages high availability
type HACoordinator struct {
	config     HAConfig
	isLeader   bool
	pool       *BackendPool
	mu         sync.RWMutex
	stopChan   chan struct{}
	stateStore *StateStore
}

// StateStore manages shared state between load balancers
type StateStore struct {
	BackendStates map[string]BackendState `json:"backend_states"`
	Timestamp     time.Time               `json:"timestamp"`
}

// BackendState represents the state of a backend
type BackendState struct {
	ID          string    `json:"id"`
	Healthy     bool      `json:"healthy"`
	LastUpdated time.Time `json:"last_updated"`
}

// NewHACoordinator creates a new HA coordinator
func NewHACoordinator(config HAConfig, pool *BackendPool) *HACoordinator {
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 5 * time.Second
	}
	if config.HeartbeatTimeout == 0 {
		config.HeartbeatTimeout = 15 * time.Second
	}
	if config.StateStoreFile == "" {
		config.StateStoreFile = "/tmp/lb_state.json"
	}

	return &HACoordinator{
		config:     config,
		pool:       pool,
		stopChan:   make(chan struct{}),
		stateStore: &StateStore{BackendStates: make(map[string]BackendState)},
	}
}

// Start starts the HA coordinator
func (ha *HACoordinator) Start() error {
	if !ha.config.Enabled {
		fmt.Println("[HA] HA is disabled")
		return nil
	}

	fmt.Printf("[HA] Starting HA coordinator in %s mode (node: %s)\n", ha.config.Mode, ha.config.NodeID)

	switch ha.config.Mode {
	case "active_passive":
		go ha.runActivePasive()
	case "active_active":
		go ha.runActiveActive()
	default:
		return fmt.Errorf("unknown HA mode: %s", ha.config.Mode)
	}

	return nil
}

// Stop stops the HA coordinator
func (ha *HACoordinator) Stop() {
	close(ha.stopChan)
}

// runActivePasive implements active/passive failover
func (ha *HACoordinator) runActivePasive() {
	ticker := time.NewTicker(ha.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ha.stopChan:
			return
		case <-ticker.C:
			ha.updateLeaderStatus()
		}
	}
}

// runActiveActive implements active/active redundancy
func (ha *HACoordinator) runActiveActive() {
	ticker := time.NewTicker(ha.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ha.stopChan:
			return
		case <-ticker.C:
			ha.syncState()
		}
	}
}

// updateLeaderStatus updates the leader status in active/passive mode
func (ha *HACoordinator) updateLeaderStatus() {
	// Simplified: just write heartbeat to file
	now := time.Now()
	data := map[string]interface{}{
		"node_id":   ha.config.NodeID,
		"is_leader": ha.isLeader,
		"timestamp": now.Unix(),
	}

	jsonData, _ := json.Marshal(data)
	os.WriteFile(ha.config.HeartbeatFile, jsonData, 0644)
}

// syncState syncs backend states across nodes in active/active mode
func (ha *HACoordinator) syncState() {
	// Create current state
	state := &StateStore{
		BackendStates: make(map[string]BackendState),
		Timestamp:     time.Now(),
	}

	for _, backend := range ha.pool.GetBackends() {
		state.BackendStates[backend.ID] = BackendState{
			ID:          backend.ID,
			Healthy:     backend.IsHealthy(),
			LastUpdated: time.Now(),
		}
	}

	// Write to shared state store
	jsonData, err := json.Marshal(state)
	if err != nil {
		fmt.Printf("[HA] Error marshaling state: %v\n", err)
		return
	}

	if err := os.WriteFile(ha.config.StateStoreFile, jsonData, 0644); err != nil {
		fmt.Printf("[HA] Error writing state: %v\n", err)
		return
	}

	// Read state from other nodes and apply it
	ha.applyRemoteState()
}

// applyRemoteState reads and applies state from other nodes
func (ha *HACoordinator) applyRemoteState() {
	// Read the shared state file
	data, err := os.ReadFile(ha.config.StateStoreFile)
	if err != nil {
		return // File doesn't exist yet, that's fine
	}

	var state StateStore
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	// Only apply if it's not too old
	if time.Since(state.Timestamp) > ha.config.HeartbeatTimeout {
		return
	}

	// Apply backend health states
	for _, backend := range ha.pool.GetBackends() {
		if backendState, exists := state.BackendStates[backend.ID]; exists {
			// If remote state differs and is recent, apply it
			if backendState.Healthy != backend.IsHealthy() &&
				time.Since(backendState.LastUpdated) < 1*time.Minute {
				if backendState.Healthy {
					ha.pool.MarkHealthy(backend)
				} else {
					ha.pool.MarkUnhealthy(backend)
				}
			}
		}
	}
}

// GetLeaderStatus returns whether this node is the leader
func (ha *HACoordinator) GetLeaderStatus() bool {
	ha.mu.RLock()
	defer ha.mu.RUnlock()
	return ha.isLeader
}

// SetLeaderStatus sets whether this node is the leader
func (ha *HACoordinator) SetLeaderStatus(isLeader bool) {
	ha.mu.Lock()
	defer ha.mu.Unlock()
	ha.isLeader = isLeader
	fmt.Printf("[HA] Node %s is now: %s\n", ha.config.NodeID, map[bool]string{true: "LEADER", false: "FOLLOWER"}[isLeader])
}
