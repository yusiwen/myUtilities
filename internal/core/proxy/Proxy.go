package proxy

import (
	"context"
	"sync"
	"time"
)

type Proxy interface {
	Start() error
	Close()
}

// BackendConfig holds configuration for a backend database.
type BackendConfig struct {
	Name     string // Backend name (used for logging)
	Host     string
	Port     int
	Priority int // Priority (lower number = higher priority)
}

// BackendStatus tracks the runtime status of a backend database.
type BackendStatus struct {
	IsAvailable bool
	LastCheck   time.Time
	LastError   error
	Context     context.Context
	Cancel      context.CancelFunc
	Mutex       sync.RWMutex
}

type DefaultProxy struct {
	ListenAddr  string
	CurrentIdx  int
	Mutex       sync.RWMutex
	HealthCheck struct {
		Query      string
		Expected   string
		Timeout    time.Duration
		Interval   time.Duration
		CancelFunc context.CancelFunc
	}
}
