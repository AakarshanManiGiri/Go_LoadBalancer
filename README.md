# Go HTTP Load Balancer

A production-grade, configurable HTTP load balancer built in Go without using Nginx. This load balancer features intelligent routing algorithms, comprehensive health checking, sticky sessions, and high availability support.

## Features

### Core Routing Capabilities
- **Round Robin**: Simple, fair distribution of requests across all healthy backends
- **Weighted Round Robin**: Distribute traffic based on backend capacity/weight
- **Least Connections**: Route requests to backends with the fewest active connections
- **Reverse Proxying**: Built on Go's `net/http/httputil.ReverseProxy` for efficient HTTP proxying

### Health Management
- **Active Health Checking**: Periodic probes to backend health endpoints
- **Passive Health Checking**: Automatic detection of unhealthy backends through error monitoring
- **Configurable Thresholds**: Fine-tune health check sensitivity
- **Quick Failover**: Instantly removes unhealthy backends from rotation

### Session Management
- **Cookie-Based Sticky Sessions**: Session affinity using HTTP cookies
- **IP Hash Sticky Sessions**: Consistent routing based on client IP
- **Automatic Session Cleanup**: Expired sessions are automatically removed
- **Session TTL**: Configurable session lifetime

### Reliability & High Availability
- **Active/Passive Failover**: One primary, automatic failover to secondary
- **Active/Active Redundancy**: Multiple load balancers sharing state
- **Shared State Store**: JSON-based state synchronization between nodes
- **Connection Tracking**: Monitor and limit connections per backend

### Production Ready
- **Comprehensive Logging**: Detailed logs for debugging and monitoring
- **Health Endpoints**: `/health` for status, `/stats` for metrics
- **Graceful Shutdown**: Clean shutdown with connection draining
- **Connection Pooling**: Configurable connection limits and timeouts

## Project Structure

```
Go_LoadBalancer/
├── main.go              # Entry point
├── config.go            # Configuration loading (YAML)
├── balancer.go          # Main load balancer implementation
├── backends.go          # Backend pool management
├── algorithm.go         # Routing algorithms (RR, WRR, LC)
├── health.go            # Active and passive health checking
├── sticky.go            # Sticky session management
├── ha.go                # High availability coordination
├── config.yaml          # Example configuration
├── main_test.go         # Tests and simulation
└── README.md            # This file
```

## Installation

### Prerequisites
- Go 1.21 or higher
- Linux, macOS, or Windows

### Build from Source

```bash
# Clone the repository
git clone https://github.com/example/go-loadbalancer.git
cd Go_LoadBalancer

# Download dependencies
go mod download

# Build the binary
go build -o loadbalancer

# Or run directly
go run *.go -config config.yaml
```

## Configuration

The load balancer is configured via a YAML file. See `config.yaml` for a complete example.

### Main Settings

```yaml
port: 8080                          # Listen port
algorithm: round_robin              # Routing algorithm
request_timeout: 30s                # Request timeout
idle_timeout: 90s                   # Connection idle timeout
max_idle_conns: 100                 # Max idle connections
```

### Backend Configuration

```yaml
backends:
  - id: backend-1                   # Unique ID
    host: localhost                 # Backend host
    port: 8081                      # Backend port
    weight: 1                       # Weight for weighted RR
    max_conns: 100                  # Max connections to backend
```

### Health Check Configuration

```yaml
health_check:
  enabled: true
  interval: 10s                     # How often to check
  timeout: 5s                       # Health check timeout
  unhealthy_threshold: 3            # Failed checks before marking unhealthy
  healthy_threshold: 2              # Successful checks before marking healthy
  path: /health                     # Health check endpoint
  method: GET                       # HTTP method
  expected_status: 200              # Expected HTTP status
```

### Sticky Session Configuration

```yaml
sticky_session:
  enabled: true
  method: cookie                    # "cookie" or "ip_hash"
  cookie_name: LB_SESSION           # Cookie name (for cookie method)
  ttl: 24h                          # Session time-to-live
```

## Usage

### Basic Usage

```bash
# Start with default config.yaml
./loadbalancer

# Or specify a custom config
./loadbalancer -config custom.yaml
```

### With High Availability

```bash
# Start primary load balancer
./loadbalancer -config config.yaml -ha ha.yaml

# Start secondary load balancer (on different machine)
./loadbalancer -config config.yaml -ha ha.yaml
```

## Endpoints

### Status Endpoints

```bash
# Check load balancer and backend health
curl http://localhost:8080/health

# Get load balancer statistics
curl http://localhost:8080/stats

# Proxy request to a backend
curl http://localhost:8080/api/users
```

## Routing Algorithms

### Round Robin
The simplest algorithm, distributes requests evenly across all healthy backends.

```yaml
algorithm: round_robin
```

**Use Case**: When all backends have similar capacity and load characteristics.

### Weighted Round Robin
Distributes requests based on backend weights.

```yaml
algorithm: weighted_round_robin
backends:
  - id: backend-1
    host: localhost
    port: 8081
    weight: 2           # Gets 2x more traffic than weight-1 backends
  - id: backend-2
    host: localhost
    port: 8082
    weight: 1
```

**Use Case**: When backends have different capacities or resources.

### Least Connections
Routes new requests to the backend with the fewest active connections.

```yaml
algorithm: least_connections
backends:
  - id: backend-1
    host: localhost
    port: 8081
    max_conns: 100      # Optional: limit max connections
```

**Use Case**: Long-lived connections or when backends have different processing times.

## Health Checking

### Active Health Checking

The load balancer periodically probes backend health endpoints:

```yaml
health_check:
  enabled: true
  interval: 10s              # Check every 10 seconds
  timeout: 5s                # Wait 5 seconds for response
  unhealthy_threshold: 3     # Mark unhealthy after 3 failures
  healthy_threshold: 2       # Mark healthy after 2 successes
  path: /health              # Health endpoint
  expected_status: 200
```

### Passive Health Checking

The load balancer tracks proxy errors and can automatically mark backends unhealthy:

```go
// Automatic when errors occur during proxying
healthChecker.OnProxyError(backend)
```

## Session Affinity (Sticky Sessions)

### Cookie-Based Method

Clients receive a cookie that ties them to a specific backend:

```yaml
sticky_session:
  enabled: true
  method: cookie
  cookie_name: LB_SESSION
  ttl: 24h
```

```bash
# First request - no session
curl http://localhost:8080/api

# Returns with Set-Cookie: LB_SESSION=<session-id>
# Subsequent requests with this cookie go to the same backend
curl -b "LB_SESSION=abc123" http://localhost:8080/api
```

### IP Hash Method

Routes based on client IP address (no cookie needed):

```yaml
sticky_session:
  enabled: true
  method: ip_hash
  ttl: 24h
```

**Use Case**: 
- Cookie method: Traditional web applications needing session persistence
- IP hash: When cookies can't be used or for protocol transparency

## High Availability

### Active/Passive Failover

One primary load balancer handles traffic; if it fails, traffic switches to secondary:

```yaml
backends:
  - id: backend-1
    host: primary-lb.example.com
    port: 8080
  - id: backend-2
    host: secondary-lb.example.com
    port: 8080
```

**Deployment**:
1. Set up two load balancer instances
2. Use DNS or a virtual IP to point to the primary
3. Monitor the primary; if it fails, failover DNS to secondary

### Active/Active Redundancy

Multiple load balancers share state and handle traffic simultaneously:

```yaml
health_check:
  enabled: true
  interval: 5s

# Shared state store (must be on shared storage)
state_store_file: /shared/lb_state.json
```

**Benefits**:
- Higher throughput (both LBs active)
- No single point of failure
- Automatic backend state synchronization
- Zero-downtime updates

**Deployment**:
1. Set up load balancers on different machines
2. Configure both to use the same state store (NFS, shared storage, etc.)
3. Use DNS round-robin or a VIP to distribute traffic

## Testing and Simulation

Run the included tests and simulation:

```bash
# Run all tests
go test -v

# Run specific test
go test -v -run TestRoundRobinRouting

# Run simulation only
go test -v -run TestMain
```

The simulation demonstrates:
1. Round-robin distribution
2. Backend failure handling
3. Algorithm switching
4. Sticky session routing
5. Connection limiting

## Performance Considerations

- **Connection Pooling**: Configure `max_idle_conns` based on your expected load
- **Health Check Interval**: Balance between quick detection and load (shorter = more load)
- **Sticky Sessions**: Use IP hash for better performance than cookies
- **Backend Pool Size**: Test with your typical number of backends (tested up to 100+)

## Monitoring and Debugging

### Check Health Status

```bash
curl -s http://localhost:8080/health | jq
```

Output:
```json
{
  "load_balancer": "OK",
  "algorithm": "round_robin",
  "healthy_backends": 3,
  "total_backends": 3,
  "backends": [
    {
      "id": "backend-1",
      "url": "http://localhost:8081",
      "healthy": true,
      "connections": 5
    }
  ]
}
```

### Check Statistics

```bash
curl -s http://localhost:8080/stats | jq
```

### Enable Verbose Logging

Edit `main.go` to add logging:

```go
fmt.Printf("[DEBUG] Selected backend: %s\n", backend.ID)
```

## Common Issues and Solutions

### Issue: "no healthy backends available"
**Solution**: 
1. Verify backends are running and accessible
2. Check health check path and expected status
3. Check firewall and network connectivity

### Issue: Sticky sessions not working
**Solution**:
1. Verify `sticky_session.enabled: true` in config
2. For cookies: check cookie_name is correct
3. For IP hash: ensure client IP is stable

### Issue: Backends frequently marked unhealthy
**Solution**:
1. Increase `unhealthy_threshold` if backends are flaky
2. Increase `timeout` if backends respond slowly
3. Verify the `/health` endpoint is responsive

## Architecture Details

### Request Flow

```
Client Request
    ↓
Load Balancer HTTP Handler
    ↓
Check Sticky Session → [Found] → Use Sticky Backend
    ↓                     [Not Found]
Select Backend via Algorithm
    ↓
Check Backend Health → [Unhealthy] → Re-select
    ↓                    [Healthy]
Check Connection Limit → [At Max] → HTTP 503
    ↓                    [OK]
Increment Connection Counter
    ↓
ReverseProxy → Backend Server
    ↓
Handle Response / Errors
    ↓
Decrement Connection Counter
    ↓
Return Response to Client
```

### Backend Health States

```
             Probe Success     Probe Failure
Initial      ─────────────────→ HEALTHY
HEALTHY                              ↓
             ← 2 successes      3 failures
             ─────────────────→ UNHEALTHY
UNHEALTHY         ↓
             ← 2 successes
             ────────────────→ HEALTHY
```

## Future Enhancements

- [ ] Metrics export (Prometheus, StatsD)
- [ ] Advanced rate limiting and circuit breaking
- [ ] WebSocket support
- [ ] gRPC load balancing
- [ ] TLS/SSL termination
- [ ] Request rewriting/modification
- [ ] Load-based auto-scaling
- [ ] Admin control API for runtime configuration

## Contributing

Contributions are welcome! Please follow these guidelines:

1. Write clear, idiomatic Go code
2. Add tests for new features
3. Update documentation
4. Follow the existing code style

## License

See LICENSE file for details.

## Support

For issues, questions, or suggestions, please open an issue on GitHub.

## Acknowledgments

Built using Go's standard library, particularly:
- `net/http` for HTTP handling
- `net/http/httputil` for reverse proxying
- `sync` for concurrency primitives

---

**Last Updated**: 2024
A Load Balancer written in Go.
