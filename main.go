package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	configFile := flag.String("config", "config.yaml", "Path to the configuration file")
	haConfig := flag.String("ha", "", "Path to HA configuration file (optional)")
	flag.Parse()

	// Load main configuration
	config, err := LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Create load balancer
	lb, err := NewLoadBalancer(config)
	if err != nil {
		fmt.Printf("Error creating load balancer: %v\n", err)
		os.Exit(1)
	}

	// Create and start HA coordinator if configured
	var haCoordinator *HACoordinator
	if *haConfig != "" {
		_, err := LoadConfig(*haConfig)
		if err != nil {
			fmt.Printf("Error loading HA config: %v\n", err)
		} else {
			// In a real implementation, we'd have separate config types
			// For now, we'll create a basic HA config from file
			haCoordinator = &HACoordinator{
				config: HAConfig{
					Enabled:           true,
					Mode:              "active_passive",
					NodeID:            "lb-node-1",
					HeartbeatFile:     "/tmp/lb_heartbeat.json",
					StateStoreFile:    "/tmp/lb_state.json",
					HeartbeatInterval: config.HealthCheck.Interval,
					HeartbeatTimeout:  config.HealthCheck.Timeout,
				},
				pool:       lb.pool,
				stopChan:   make(chan struct{}),
				stateStore: &StateStore{BackendStates: make(map[string]BackendState)},
			}
			if err := haCoordinator.Start(); err != nil {
				fmt.Printf("Error starting HA coordinator: %v\n", err)
			}
		}
	}

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start load balancer in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- lb.Start()
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal: %v\n", sig)
		fmt.Println("Shutting down load balancer...")
		lb.Stop()
		if haCoordinator != nil {
			haCoordinator.Stop()
		}
	case err := <-errChan:
		fmt.Printf("Load balancer error: %v\n", err)
	}
}
