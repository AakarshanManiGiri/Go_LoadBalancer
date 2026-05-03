# Quick Start Guide

Get up and running with the Go Load Balancer in 5 minutes.

## Prerequisites

- Go 1.21 or higher
- Three backend servers (or we can simulate them)

## Step 1: Build

```bash
cd Go_LoadBalancer
go build -o loadbalancer
```

## Step 2: Start Mock Backends (Optional)

If you don't have real backends, start simple Go servers:

```bash
# Terminal 1 - Backend 1
go run -
:8081

# Terminal 2 - Backend 2
go run -
:8082

# Terminal 3 - Backend 3
go run -
:8083
```

Or create a simple backend script (backend.go):
```go
package main
import ("fmt"; "net/http"; "os")
func main() {
    port := os.Args[1]
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(200)
        fmt.Fprint(w, "OK")
    })
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Response from backend on port %s\n", port)
    })
    fmt.Printf("Backend listening on :%s\n", port)
    http.ListenAndServe(":"+port, nil)
}
```

Run backends:
```bash
go run backend.go 8081 &
go run backend.go 8082 &
go run backend.go 8083 &
```

## Step 3: Configure Load Balancer

Use the provided `config.yaml`:

```yaml
port: 8080
algorithm: round_robin

backends:
  - id: backend-1
    host: localhost
    port: 8081
    weight: 1
    max_conns: 100

  - id: backend-2
    host: localhost
    port: 8082
    weight: 1
    max_conns: 100

  - id: backend-3
    host: localhost
    port: 8083
    weight: 1
    max_conns: 100

health_check:
  enabled: true
  interval: 10s
  timeout: 5s
  unhealthy_threshold: 3
  healthy_threshold: 2
  path: /health
  method: GET
  expected_status: 200

sticky_session:
  enabled: true
  method: cookie
  cookie_name: LB_SESSION
  ttl: 24h
```

## Step 4: Start Load Balancer

```bash
./loadbalancer -config config.yaml
```

You should see:
```
Starting load balancer on port 8080 with round_robin algorithm
[HEALTH] Backend backend-1 marked HEALTHY
[HEALTH] Backend backend-2 marked HEALTHY
[HEALTH] Backend backend-3 marked HEALTHY
```

## Step 5: Test It Out

### Test basic routing (in another terminal):

```bash
# Make several requests
for i in {1..10}; do
  curl http://localhost:8080/
  echo ""
done
```

You should see responses from different backends.

### Check health status:

```bash
curl http://localhost:8080/health | jq
```

### Check statistics:

```bash
curl http://localhost:8080/stats | jq
```

### Test with sticky sessions:

```bash
# Save cookies
curl -c cookies.txt http://localhost:8080/
curl -b cookies.txt http://localhost:8080/
curl -b cookies.txt http://localhost:8080/

# Note: All three requests should go to the SAME backend
```

## Step 6: Try Different Algorithms

### Least Connections:

Edit config.yaml:
```yaml
algorithm: least_connections
```

Make requests with longer processing time on backends:
```bash
# Modify backend to add delay
http.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
    time.Sleep(5 * time.Second)
    fmt.Fprint(w, "Done")
})
```

### Weighted Round Robin:

Edit config.yaml:
```yaml
algorithm: weighted_round_robin
backends:
  - id: powerful
    host: localhost
    port: 8081
    weight: 2  # Gets more traffic
  - id: normal
    host: localhost
    port: 8082
    weight: 1
```

## Step 7: Run Tests

```bash
go test -v
```

This runs:
- Round-robin routing test
- Least connections test
- Health checking test
- Sticky session test
- Failover test
- Connection limits test
- Full simulation

## Common Commands

```bash
# Build
make build

# Run with config
make run

# Run tests
make test

# Run tests with coverage
make test-coverage

# Code formatting
make fmt

# Run simulation demo
make demo

# Clean build artifacts
make clean
```

## Troubleshooting

### Backend not responding
```bash
# Check if backend is running
curl http://localhost:8081/health

# Check firewall
netstat -an | grep 8081
```

### Load balancer won't start
```bash
# Check if port 8080 is in use
netstat -an | grep 8080

# Try different port
./loadbalancer -config <(sed 's/port: 8080/port: 8888/' config.yaml)
```

### Health checks failing
```bash
# Increase timeout
health_check:
  timeout: 10s  # was 5s

# Disable health checks temporarily
health_check:
  enabled: false
```

## Next Steps

1. **Read IMPLEMENTATION_GUIDE.md** - Deep dive into architecture
2. **Read README.md** - Complete feature documentation
3. **Modify config.yaml** - Try different configurations
4. **Try different algorithms** - Round Robin, Weighted RR, Least Connections
5. **Run load tests** - Test with Apache Bench or similar:
   ```bash
   ab -n 10000 -c 100 http://localhost:8080/
   ```

## Example Configurations

### High-Performance API Server
```yaml
algorithm: least_connections
health_check:
  interval: 5s  # Quick failover
  unhealthy_threshold: 2
sticky_session:
  enabled: true
  method: ip_hash  # No cookies
```

### Traditional Web App
```yaml
algorithm: round_robin
health_check:
  interval: 10s
sticky_session:
  enabled: true
  method: cookie  # Cookie-based sessions
```

### Database Connection Pool
```yaml
algorithm: least_connections
backends:
  - id: db-1
    max_conns: 50
  - id: db-2
    max_conns: 50
health_check:
  enabled: false  # DB usually has own health checks
```

## Getting Help

- Check the [README.md](README.md) for detailed documentation
- See [IMPLEMENTATION_GUIDE.md](IMPLEMENTATION_GUIDE.md) for architecture details
- Run `go test -v` to see all features in action
- Check logs in the console output for errors

## Summary

You now have:
✓ A working load balancer listening on port 8080
✓ Round-robin request distribution
✓ Active health checking
✓ Sticky sessions
✓ Multiple routing algorithms available

Ready to scale! 🚀
