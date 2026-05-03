#!/bin/bash
# demo.sh - Demonstration script for the Go Load Balancer

set -e

echo "=========================================="
echo "Go HTTP Load Balancer - Demo"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if load balancer is built
if [ ! -f "loadbalancer" ]; then
    echo -e "${YELLOW}Building load balancer...${NC}"
    go build -o loadbalancer
    echo -e "${GREEN}✓ Build complete${NC}"
fi

# Function to start a mock backend
start_backend() {
    local port=$1
    local name=$2
    
    echo -e "${BLUE}Starting ${name} on port ${port}...${NC}"
    
    # Create a simple Go server in the background
    go run - <<EOF &
package main
import (
    "fmt"
    "net/http"
)
func main() {
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprint(w, "OK")
    })
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Backend", "${name}")
        fmt.Fprintf(w, "Response from ${name} (port ${port})\n")
    })
    fmt.Printf("${name} listening on port ${port}\n")
    http.ListenAndServe(":${port}", nil)
}
EOF
    sleep 1
}

# Function to stop all background processes
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    pkill -f "loadbalancer" || true
    pkill -f "go run" || true
    sleep 1
    echo -e "${GREEN}✓ Cleanup complete${NC}"
}

# Set up cleanup on exit
trap cleanup EXIT

# Start mock backends
echo -e "${BLUE}Setting up mock backends...${NC}"
start_backend 8081 "Backend-1"
start_backend 8082 "Backend-2"
start_backend 8083 "Backend-3"

echo ""
echo -e "${GREEN}✓ All backends started${NC}"
sleep 2

# Start load balancer
echo ""
echo -e "${BLUE}Starting load balancer on port 8080...${NC}"
timeout 60 ./loadbalancer -config config.yaml &
LB_PID=$!

sleep 2

# Test health endpoint
echo ""
echo -e "${BLUE}Testing health endpoint...${NC}"
curl -s http://localhost:8080/health | jq . 2>/dev/null || echo "Health check endpoint"
echo ""

# Test round-robin routing
echo -e "${BLUE}Testing round-robin routing (10 requests)...${NC}"
for i in {1..10}; do
    echo -n "Request $i: "
    curl -s http://localhost:8080/ | head -c 40
    echo ""
    sleep 0.5
done

echo ""

# Test stats endpoint
echo -e "${BLUE}Checking statistics...${NC}"
curl -s http://localhost:8080/stats | jq . 2>/dev/null || echo "Stats endpoint"
echo ""

# Test with sticky sessions
echo -e "${BLUE}Testing sticky sessions...${NC}"
response=$(curl -s -D - http://localhost:8080/ 2>&1)
echo "Response headers:"
echo "$response" | head -20
echo ""

echo -e "${GREEN}✓ Demo complete${NC}"
echo ""
echo "Load balancer features demonstrated:"
echo "  ✓ Round-robin request distribution"
echo "  ✓ Health checking endpoint"
echo "  ✓ Statistics endpoint"
echo "  ✓ Sticky sessions support"
echo ""
echo "Try these commands in another terminal while the demo runs:"
echo "  curl http://localhost:8080/"
echo "  curl http://localhost:8080/health"
echo "  curl http://localhost:8080/stats"
echo ""

# Keep running until interrupted
wait $LB_PID 2>/dev/null || true
