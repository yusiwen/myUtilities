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

// 后端数据库配置
type BackendConfig struct {
	Name     string // 后端名称（用于日志）
	Host     string
	Port     int
	Priority int // 优先级 (数字越小优先级越高)
}

// 后端数据库状态
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
