package watcher

import (
	"fmt"
	"sync"
	"time"
)

type EventType string

const (
	Added    EventType = "ADDED"
	Modified EventType = "MODIFIED"
	Deleted  EventType = "DELETED"
	Error    EventType = "ERROR"
)

type Event struct {
	Type      EventType   `json:"type"`
	Object    interface{} `json:"object"`
	Timestamp time.Time   `json:"timestamp"`
}

type EventStore struct {
	mu       sync.RWMutex
	events   map[ResourceKey][]Event
	versions map[ResourceKey][]string
	maxSize  int
	nextSeq  uint64
}

func NewEventStore(maxSize int) *EventStore {
	return &EventStore{
		events:   make(map[ResourceKey][]Event),
		versions: make(map[ResourceKey][]string),
		maxSize:  maxSize,
	}
}

func (s *EventStore) AddEvent(key ResourceKey, event Event) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextSeq++
	resourceVersion := fmt.Sprintf("%d-%d", time.Now().UnixNano(), s.nextSeq)

	if _, exists := s.events[key]; !exists {
		s.events[key] = make([]Event, 0, s.maxSize)
		s.versions[key] = make([]string, 0, s.maxSize)
	}

	events := s.events[key]
	vers := s.versions[key]

	if len(events) >= s.maxSize {
		events = events[1:]
		vers = vers[1:]
	}

	events = append(events, event)
	vers = append(vers, resourceVersion)
	s.events[key] = events
	s.versions[key] = vers

	return resourceVersion
}

func (s *EventStore) GetEventsAfter(key ResourceKey, resourceVersion string) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, exists := s.events[key]
	if !exists {
		return nil, fmt.Errorf("no events for resource %v", key)
	}

	vers, exists := s.versions[key]
	if !exists {
		return nil, fmt.Errorf("no versions for resource %v", key)
	}

	startIndex := -1
	for i, v := range vers {
		if v == resourceVersion {
			startIndex = i
			break
		}
	}
	if startIndex < 0 {
		return nil, fmt.Errorf("resourceVersion %s not found", resourceVersion)
	}

	if startIndex >= len(events)-1 {
		return []Event{}, nil
	}

	result := make([]Event, len(events)-startIndex-1)
	for i, ev := range events[startIndex+1:] {
		ev.Object = versionedObject(ev.Object, vers[startIndex+1+i])
		result[i] = ev
	}
	return result, nil
}

func versionedObject(obj interface{}, version string) interface{} {
	return struct {
		Object          interface{} `json:"object"`
		ResourceVersion string      `json:"resourceVersion"`
	}{
		Object:          obj,
		ResourceVersion: version,
	}
}
