package watcher

import (
	"fmt"
	"sync"
	"time"
)

// EventType 定义资源变更类型
type EventType string

const (
	Added    EventType = "ADDED"
	Modified EventType = "MODIFIED"
	Deleted  EventType = "DELETED"
	Error    EventType = "ERROR"
)

// Event 表示资源变更事件
type Event struct {
	Type      EventType   `json:"type"`
	Object    interface{} `json:"object"`
	Timestamp time.Time   `json:"timestamp"`
}

// EventStore 存储事件历史
type EventStore struct {
	mu       sync.RWMutex
	events   map[ResourceKey][]Event
	versions map[ResourceKey]map[string]int // resourceVersion -> index
	maxSize  int
}

func NewEventStore(maxSize int) *EventStore {
	return &EventStore{
		events:   make(map[ResourceKey][]Event),
		versions: make(map[ResourceKey]map[string]int),
		maxSize:  maxSize,
	}
}

func (s *EventStore) AddEvent(key ResourceKey, event Event) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 生成资源版本 (使用时间戳+随机数)
	resourceVersion := fmt.Sprintf("%d-%d", time.Now().UnixNano(), len(s.events[key]))

	// 存储事件
	if _, exists := s.events[key]; !exists {
		s.events[key] = make([]Event, 0, s.maxSize)
		s.versions[key] = make(map[string]int)
	}

	events := s.events[key]
	if len(events) >= s.maxSize {
		// 移除最旧的事件
		oldest := events[0]
		if rv, ok := oldest.Object.(struct {
			Object          interface{}
			ResourceVersion string
		}); ok {
			delete(s.versions[key], rv.ResourceVersion)
		}
		events = events[1:]
	}

	// 添加新事件
	event.Object = struct {
		Object          interface{}
		ResourceVersion string
	}{
		Object:          event.Object,
		ResourceVersion: resourceVersion,
	}

	events = append(events, event)
	s.events[key] = events
	s.versions[key][resourceVersion] = len(events) - 1

	return resourceVersion
}

func (s *EventStore) GetEventsAfter(key ResourceKey, resourceVersion string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, exists := s.events[key]
	if !exists {
		return nil, fmt.Errorf("no events for resource %v", key)
	}

	versions, exists := s.versions[key]
	if !exists {
		return nil, fmt.Errorf("no versions for resource %v", key)
	}

	startIndex, exists := versions[resourceVersion]
	if !exists {
		return nil, fmt.Errorf("resourceVersion %s not found", resourceVersion)
	}

	if startIndex >= len(events)-1 {
		return []Event{}, nil
	}

	return events[startIndex+1:], nil
}
