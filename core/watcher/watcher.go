package watcher

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Watcher 通用监控接口
type Watcher interface {
	// Watch 开始监控资源变化
	Watch(ctx context.Context) (<-chan Event, error)

	// Stop 停止监控
	Stop()

	// List 获取当前资源列表
	List() ([]interface{}, error)
}

// ResourceKey 资源标识符
type ResourceKey struct {
	Group     string
	Version   string
	Resource  string
	Namespace string
	Name      string
}

// EventHandler 处理事件的函数类型
type EventHandler func(event Event)

// ================== 事件分发系统 ==================

// WatchServer 事件分发服务器
type WatchServer struct {
	mu         sync.RWMutex
	watchers   map[ResourceKey]Watcher
	clients    map[ResourceKey]map[uint64]chan Event
	nextClient uint64
	eventStore *EventStore
}

// NewWatchServer 创建新的Watch服务器
func NewWatchServer() *WatchServer {
	return &WatchServer{
		watchers:   make(map[ResourceKey]Watcher),
		clients:    make(map[ResourceKey]map[uint64]chan Event),
		eventStore: NewEventStore(1000), // 存储最近的1000个事件
	}
}

// RegisterWatcher 注册资源监控器
func (s *WatchServer) RegisterWatcher(key ResourceKey, watcher Watcher) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.watchers[key]; exists {
		return fmt.Errorf("watcher already registered for %v", key)
	}

	s.watchers[key] = watcher
	s.clients[key] = make(map[uint64]chan Event)

	// 启动监控协程
	go s.startWatching(key, watcher)

	return nil
}

// Watch 客户端订阅资源变更
func (s *WatchServer) Watch(key ResourceKey, resourceVersion string) (<-chan Event, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	watcher, exists := s.watchers[key]
	if !exists {
		return nil, 0, fmt.Errorf("no watcher registered for %v", key)
	}

	// 分配客户端ID
	s.nextClient++
	clientID := s.nextClient

	// 创建事件通道
	eventCh := make(chan Event, 100)
	s.clients[key][clientID] = eventCh

	// 如果提供了resourceVersion，发送历史事件
	if resourceVersion != "" {
		go s.sendHistoryEvents(key, resourceVersion, eventCh)
	} else {
		// 发送当前状态作为ADDED事件
		go s.sendInitialState(key, watcher, eventCh)
	}

	return eventCh, clientID, nil
}

// Unwatch 取消订阅
func (s *WatchServer) Unwatch(key ResourceKey, clientID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if clients, ok := s.clients[key]; ok {
		if ch, exists := clients[clientID]; exists {
			close(ch)
			delete(clients, clientID)
		}
	}
}

func (s *WatchServer) startWatching(key ResourceKey, watcher Watcher) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, err := watcher.Watch(ctx)
	if err != nil {
		// 处理错误
		return
	}

	for event := range eventCh {
		s.mu.RLock()

		// 存储事件
		resourceVersion := s.eventStore.AddEvent(key, event)
		event.Object = s.addResourceVersion(event.Object, resourceVersion)

		// 分发事件给所有订阅者
		for _, clientCh := range s.clients[key] {
			select {
			case clientCh <- event:
			default:
				// 避免阻塞，跳过事件
			}
		}

		s.mu.RUnlock()
	}
}

func (s *WatchServer) sendInitialState(key ResourceKey, watcher Watcher, ch chan<- Event) {
	resources, err := watcher.List()
	if err != nil {
		ch <- Event{Type: Error, Object: err.Error()}
		return
	}

	for _, obj := range resources {
		// 创建ADDED事件
		event := Event{
			Type:      Added,
			Object:    obj,
			Timestamp: time.Now(),
		}

		// 存储并添加资源版本
		resourceVersion := s.eventStore.AddEvent(key, event)
		event.Object = s.addResourceVersion(obj, resourceVersion)

		select {
		case ch <- event:
		case <-time.After(100 * time.Millisecond):
			// 超时跳过
		}
	}
}

func (s *WatchServer) sendHistoryEvents(key ResourceKey, resourceVersion string, ch chan<- Event) {
	events, err := s.eventStore.GetEventsAfter(key, resourceVersion)
	if err != nil {
		ch <- Event{Type: Error, Object: err.Error()}
		return
	}

	for _, event := range events {
		select {
		case ch <- event:
		case <-time.After(100 * time.Millisecond):
			// 超时跳过
		}
	}
}

func (s *WatchServer) addResourceVersion(obj interface{}, version string) interface{} {
	// 在实际应用中，这里会根据对象类型添加resourceVersion字段
	// 简化实现：返回包含版本信息的包装对象
	return struct {
		Object          interface{} `json:"object"`
		ResourceVersion string      `json:"resourceVersion"`
	}{
		Object:          obj,
		ResourceVersion: version,
	}
}
