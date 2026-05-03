package main

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// BackendConfig represents configuration for a single backend server
type BackendConfig struct {
	ID       string `yaml:"id"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Weight   int    `yaml:"weight"`    // For weighted round-robin
	MaxConns int    `yaml:"max_conns"` // For least connections
}

// HealthCheckConfig represents health check settings
type HealthCheckConfig struct {
	Enabled            bool          `yaml:"enabled"`
	Interval           time.Duration `yaml:"interval"`            // How often to check
	Timeout            time.Duration `yaml:"timeout"`             // How long to wait for response
	UnhealthyThreshold int           `yaml:"unhealthy_threshold"` // Consecutive failures to mark unhealthy
	HealthyThreshold   int           `yaml:"healthy_threshold"`   // Consecutive successes to mark healthy
	Path               string        `yaml:"path"`                // Health check endpoint
	Method             string        `yaml:"method"`              // HTTP method (GET, HEAD)
	ExpectedStatus     int           `yaml:"expected_status"`     // Expected HTTP status
}

// StickySessionConfig represents sticky session settings
type StickySessionConfig struct {
	Enabled    bool          `yaml:"enabled"`
	Method     string        `yaml:"method"` // "cookie" or "ip_hash"
	CookieName string        `yaml:"cookie_name"`
	TTL        time.Duration `yaml:"ttl"`
}

// LoadBalancerConfig represents the main configuration
type LoadBalancerConfig struct {
	Port           int                 `yaml:"port"`
	Algorithm      string              `yaml:"algorithm"` // "round_robin" or "least_connections"
	Backends       []BackendConfig     `yaml:"backends"`
	HealthCheck    HealthCheckConfig   `yaml:"health_check"`
	StickySession  StickySessionConfig `yaml:"sticky_session"`
	RequestTimeout time.Duration       `yaml:"request_timeout"`
	IdleTimeout    time.Duration       `yaml:"idle_timeout"`
	MaxIdleConns   int                 `yaml:"max_idle_conns"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filepath string) (*LoadBalancerConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &LoadBalancerConfig{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Port == 0 {
		config.Port = 8080
	}
	if config.Algorithm == "" {
		config.Algorithm = "round_robin"
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 90 * time.Second
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 100
	}
	if config.HealthCheck.Interval == 0 {
		config.HealthCheck.Interval = 10 * time.Second
	}
	if config.HealthCheck.Timeout == 0 {
		config.HealthCheck.Timeout = 5 * time.Second
	}
	if config.HealthCheck.UnhealthyThreshold == 0 {
		config.HealthCheck.UnhealthyThreshold = 3
	}
	if config.HealthCheck.HealthyThreshold == 0 {
		config.HealthCheck.HealthyThreshold = 2
	}
	if config.HealthCheck.ExpectedStatus == 0 {
		config.HealthCheck.ExpectedStatus = 200
	}
	if config.HealthCheck.Method == "" {
		config.HealthCheck.Method = "GET"
	}
	if config.StickySession.CookieName == "" {
		config.StickySession.CookieName = "LB_SESSION"
	}
	if config.StickySession.TTL == 0 {
		config.StickySession.TTL = 24 * time.Hour
	}

	return config, nil
}
