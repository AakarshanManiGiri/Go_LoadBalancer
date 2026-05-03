# Go Load Balancer - Implementation Guide

## Overview

This is a production-grade HTTP load balancer implemented in pure Go, without any external dependencies except for YAML parsing. It demonstrates advanced load balancing concepts including routing algorithms, health checking, session affinity, and high availability.

## Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│                   HTTP Listener (Port 8080)                 │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │  Load Balancer   │
                    │   HTTP Handler   │
                    └──────────────────┘
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
         ▼                    ▼                    ▼
    ┌─────────────┐   ┌──────────────┐   ┌─────────────────┐
    │   Sticky    │   │  Routing     │   │    Backend      │
    │  Sessions   │   │  Algorithm   │   │     Pool        │
    │  Manager    │   │              │   │                 │
    └─────────────┘   └──────────────┘   └─────────────────┘
         │                    │                    │
         └────────────────────┼────────────────────┘
                              │
                    ┌─────────▼──────────┐
                    │   Reverse Proxy    │
                    │  (httputil.Proxy)  │
                    └─────────────────────┘
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
         ▼                    ▼                    ▼
    ┌─────────────┐   ┌──────────────┐   ┌─────────────────┐
    │  Backend 1  │   │  Backend 2   │   │   Backend 3     │
    │ :8081       │   │ :8082        │   │   :8083         │
    └─────────────┘   └──────────────┘   └─────────────────┘
```

### Key Files

- **main.go**: Entry point and signal handling
- **config.go**: Configuration loading from YAML
- **balancer.go**: Main load balancer logic with HTTP handler
- **backends.go**: Backend pool management and state tracking
- **algorithm.go**: Routing algorithms (Round Robin, Weighted RR, Least Connections)
- **health.go**: Active and passive health checking
- **sticky.go**: Sticky session management (cookie and IP hash)
- **ha.go**: High availability coordination (active/passive, active/active)

## Request Flow

### 1. Incoming Request
```go
// HTTP request comes in on port 8080
GET /api/users
```

### 2. Handler Processing
```go
func (lb *LoadBalancer) handleRequest(w http.ResponseWriter, r *http.Request) {
    // Step 1: Check sticky session
    backend, sessionID := lb.stickySessionMgr.GetBackendFromRequest(r, lb.pool)
    
    // Step 2: If no sticky session, select via algorithm
    if backend == nil {
        backend, _ = lb.algorithm.SelectBackend(lb.pool)
        sessionID = lb.stickySessionMgr.GenerateSessionID(r)
        lb.stickySessionMgr.SetBackendForSession(sessionID, backend)
    }
    
    // Step 3: Check connection limits
    backend.IncrementConnections()
    defer backend.DecrementConnections()
    
    // Step 4: Proxy the request
    lb.proxyRequest(w, r, backend)
}
```

### 3. Reverse Proxying
```go
// httputil.ReverseProxy forwards the request
proxy := httputil.NewSingleHostReverseProxy(backend.URL)
proxy.ServeHTTP(w, r)
```

### 4. Response Return
```go
// Response is sent back to client with session cookie (if enabled)
HTTP/1.1 200 OK
Set-Cookie: LB_SESSION=<session-id>
Content-Type: application/json
```

## Routing Algorithms

### Round Robin
**Algorithm**: Rotates through backends sequentially

```go
// Current index: 0
// Backends: [b1, b2, b3]

Request 1: index = (0 + 1) % 3 = 1 → b2
Request 2: index = (1 + 1) % 3 = 2 → b3
Request 3: index = (2 + 1) % 3 = 0 → b1
Request 4: index = (1 + 1) % 3 = 1 → b2
```

**Pros**: Simple, fair distribution
**Cons**: Ignores backend load

### Weighted Round Robin
**Algorithm**: Distributes based on weights

```go
Backends: [b1 (weight=2), b2 (weight=1), b3 (weight=1)]

Distribution: b1, b1, b2, b3, b1, b1, b2, b3...
(2x traffic to b1)
```

**Pros**: Accounts for backend capacity
**Cons**: More complex

### Least Connections
**Algorithm**: Routes to backend with fewest active connections

```go
Active connections:
- b1: 5 connections
- b2: 2 connections  ← Selected (least)
- b3: 8 connections
```

**Pros**: Good for long-lived connections
**Cons**: Requires connection tracking

## Health Checking

### Active Health Checking

Periodic probes to `/health` endpoint:

```go
Interval: 10 seconds
Every 10s:
  GET http://backend:8081/health
  
If status == 200:
  success_count++
  failure_count = 0
  
If failures >= unhealthy_threshold (3):
  Mark backend UNHEALTHY
  Remove from rotation
  
If successes >= healthy_threshold (2):
  Mark backend HEALTHY
  Add back to rotation
```

### Passive Health Checking

Monitors errors during proxying:

```go
If proxy fails (connection timeout, error):
  healthChecker.OnProxyError(backend)
  failure_count++
  
If failures >= threshold:
  Mark backend UNHEALTHY
```

## Session Affinity

### Cookie Method

```
First Request:
GET /api/users
↓
Generate session ID
↓
Response includes: Set-Cookie: LB_SESSION=<id>

Second Request:
GET /api/data
Cookie: LB_SESSION=<id>
↓
Lookup: sessions[id] = "backend-1"
↓
Route to backend-1 (same as first request)
```

### IP Hash Method

```
Client IP: 192.168.1.100
↓
hash(192.168.1.100) = "a1b2c3d4..."
↓
Consistently routes to same backend
(No cookie, IP-based tracking)
```

## High Availability

### Active/Passive Failover

```
Normal Operation:
Client → VIP → Load Balancer 1 (ACTIVE) → Backends

Load Balancer 1 Fails:
Client → VIP → DNS failover → Load Balancer 2 (ACTIVE) → Backends

Heartbeat Check:
LB1 writes heartbeat to /tmp/lb_heartbeat.json every 5s
If LB1 heartbeat stops, DNS points to LB2
```

### Active/Active Redundancy

```
Normal Operation:
Clients → Load Balancer 1 ↘
         Load Balancer 2 ↗ → Shared State Store → Backends

State Synchronization:
LB1 health checks backends, writes state to /tmp/lb_state.json
LB2 reads state from /tmp/lb_state.json
Backend health state is shared between both LBs

Benefits:
- Both LBs handling traffic (higher throughput)
- No single point of failure
- Automatic state synchronization
```

## Configuration

### Example: Production Config

```yaml
port: 8080
algorithm: least_connections  # For API servers

backends:
  - id: api-1
    host: api-1.internal
    port: 8080
    max_conns: 500

health_check:
  enabled: true
  interval: 5s               # Quick detection
  timeout: 3s
  unhealthy_threshold: 2     # Aggressive
  healthy_threshold: 1
  path: /health

sticky_session:
  enabled: true
  method: cookie
  ttl: 1h
```

## Performance Tuning

### Connection Pool
```go
max_idle_conns: 100  // Per transport
// Increase for more concurrent connections
```

### Health Check Frequency
```go
interval: 10s  // Balance between detection speed and load
```

### Backend Capacity
```yaml
backends:
  - id: powerful
    weight: 3      // 3x traffic
  - id: weak
    weight: 1      // 1x traffic
```

## Testing

### Unit Tests
```bash
go test -v -run TestRoundRobinRouting
go test -v -run TestHealthChecking
```

### Full Simulation
```bash
go test -v -run TestMain
# Runs complete simulation showing all features
```

### Load Testing
```bash
# Install Apache Bench
brew install httpd  # macOS
apt-get install apache2-utils  # Linux

# Run 10,000 requests with 100 concurrent clients
ab -n 10000 -c 100 http://localhost:8080/
```

## Common Issues

### Backend Marked Unhealthy Immediately

**Cause**: Health check endpoint not responding

**Fix**:
1. Verify backend has `/health` endpoint
2. Check `expected_status` in config
3. Increase `timeout` if backend is slow

```yaml
health_check:
  timeout: 10s  # Increase from 5s
  unhealthy_threshold: 5  # More tolerant
```

### Sticky Sessions Not Working

**Cause**: Cookie not being sent/received

**Fix**:
1. Verify `sticky_session.enabled: true`
2. Check cookie name in config
3. Ensure client accepts cookies

```bash
# Test with curl
curl -c cookies.txt http://localhost:8080/
curl -b cookies.txt http://localhost:8080/
```

### All Backends Show Unhealthy

**Cause**: Incorrect backend addresses or port

**Fix**:
1. Test backend directly: `curl http://backend:port/health`
2. Verify firewall allows connections
3. Check health check path exists

## Monitoring

### Health Status
```bash
curl http://localhost:8080/health | jq
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

### Statistics
```bash
curl http://localhost:8080/stats | jq
```

Output:
```json
{
  "total_connections": 42,
  "healthy_backends": 3,
  "total_backends": 3,
  "algorithm": "round_robin"
}
```

## Extension Points

### Custom Routing Algorithm

```go
type MyAlgorithm struct {}

func (m *MyAlgorithm) SelectBackend(pool *BackendPool) (*Backend, error) {
    // Your custom logic here
}

func (m *MyAlgorithm) Name() string {
    return "my_algorithm"
}

// Register in algorithm.go
case "my_algorithm":
    return &MyAlgorithm{}, nil
```

### Custom Health Check

```go
func customHealthCheck(backend *Backend) bool {
    // Custom logic
    return true
}

// Call from healthChecker.checkBackend()
```

## Production Deployment

### Docker Deployment
```bash
docker build -t go-loadbalancer .
docker run -p 8080:8080 -v config.yaml:/app/config.yaml go-loadbalancer
```

### Kubernetes Deployment
```yaml
apiVersion: v1
kind: Service
metadata:
  name: load-balancer
spec:
  ports:
  - port: 8080
    targetPort: 8080
  selector:
    app: load-balancer
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: load-balancer
spec:
  replicas: 2  # HA setup
  selector:
    matchLabels:
      app: load-balancer
  template:
    metadata:
      labels:
        app: load-balancer
    spec:
      containers:
      - name: load-balancer
        image: go-loadbalancer:latest
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
```

## Troubleshooting

### Enable Debug Logging

Edit relevant files to add logging:

```go
fmt.Printf("[DEBUG] Selecting backend for request from %s\n", r.RemoteAddr)
```

### Check Backend Connectivity

```bash
# Test if load balancer can reach backend
curl -v http://localhost:8080/
# Check actual backend
curl -v http://localhost:8081/
```

### Monitor in Real-Time

```bash
# Watch health endpoint changes
watch -n 1 'curl -s http://localhost:8080/health | jq .'
```

## License

See LICENSE file for details.
