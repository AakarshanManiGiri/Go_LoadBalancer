package main

import (
	"crypto/md5"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// StickySessionManager manages sticky sessions
type StickySessionManager struct {
	config   StickySessionConfig
	sessions map[string]string // session ID -> backend ID
	mu       sync.RWMutex
	expiry   map[string]time.Time // session ID -> expiry time
}

// NewStickySessionManager creates a new sticky session manager
func NewStickySessionManager(config StickySessionConfig) *StickySessionManager {
	ssm := &StickySessionManager{
		config:   config,
		sessions: make(map[string]string),
		expiry:   make(map[string]time.Time),
	}

	if config.Enabled {
		// Start cleanup goroutine
		go ssm.cleanupLoop()
	}

	return ssm
}

// GetBackendFromRequest retrieves the sticky backend for a request
func (ssm *StickySessionManager) GetBackendFromRequest(r *http.Request, pool *BackendPool) (*Backend, string) {
	if !ssm.config.Enabled {
		return nil, ""
	}

	var sessionID string

	// Determine session ID based on method
	switch ssm.config.Method {
	case "cookie":
		cookie, err := r.Cookie(ssm.config.CookieName)
		if err == nil {
			sessionID = cookie.Value
		}
	case "ip_hash":
		sessionID = ssm.hashIP(r.RemoteAddr)
	default:
		return nil, ""
	}

	if sessionID == "" {
		return nil, ""
	}

	// Check if session is still valid
	ssm.mu.RLock()
	backendID, exists := ssm.sessions[sessionID]
	expiry, hasExpiry := ssm.expiry[sessionID]
	ssm.mu.RUnlock()

	if !exists || (hasExpiry && expiry.Before(time.Now())) {
		return nil, sessionID
	}

	// Find the backend
	for _, backend := range pool.GetHealthyBackends() {
		if backend.ID == backendID {
			return backend, sessionID
		}
	}

	return nil, sessionID
}

// SetBackendForSession stores the backend for a session
func (ssm *StickySessionManager) SetBackendForSession(sessionID string, backend *Backend) {
	if !ssm.config.Enabled || sessionID == "" {
		return
	}

	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	ssm.sessions[sessionID] = backend.ID
	ssm.expiry[sessionID] = time.Now().Add(ssm.config.TTL)
}

// SetCookie sets a cookie on the response for session tracking
func (ssm *StickySessionManager) SetCookie(w http.ResponseWriter, sessionID string) {
	if !ssm.config.Enabled || ssm.config.Method != "cookie" || sessionID == "" {
		return
	}

	cookie := &http.Cookie{
		Name:     ssm.config.CookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(ssm.config.TTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

// GenerateSessionID generates a new session ID
func (ssm *StickySessionManager) GenerateSessionID(r *http.Request) string {
	var data string

	if ssm.config.Method == "ip_hash" {
		data = ssm.hashIP(r.RemoteAddr)
	} else {
		data = fmt.Sprintf("%d-%s", time.Now().UnixNano(), r.RemoteAddr)
	}

	return data
}

// hashIP hashes the IP address to create a stable session ID
func (ssm *StickySessionManager) hashIP(remoteAddr string) string {
	ip, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		ip = remoteAddr
	}

	hash := md5.Sum([]byte(ip))
	return fmt.Sprintf("%x", hash)
}

// cleanupLoop periodically removes expired sessions
func (ssm *StickySessionManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ssm.cleanup()
	}
}

// cleanup removes expired sessions
func (ssm *StickySessionManager) cleanup() {
	ssm.mu.Lock()
	defer ssm.mu.Unlock()

	now := time.Now()
	for sessionID, expiry := range ssm.expiry {
		if expiry.Before(now) {
			delete(ssm.sessions, sessionID)
			delete(ssm.expiry, sessionID)
		}
	}
}
