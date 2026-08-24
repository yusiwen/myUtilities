package watcher

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Watcher is the generic interface for resource watchers.
type Watcher interface {
	// Watch starts watching for resource changes and returns an event channel.
	Watch(ctx context.Context) (<-chan Event, error)

	// Stop shuts down the watcher.
	Stop()

	// List returns the current list of resources.
	List() ([]interface{}, error)
}

// ResourceKey uniquely identifies a Kubernetes-style resource.
type ResourceKey struct {
	Group     string
	Version   string
	Resource  string
	Namespace string
	Name      string
}

// EventHandler is the function type for handling events.
type EventHandler func(event Event)

// ================== Event Dispatch System ==================

// WatchServer dispatches events to registered watchers and their subscribers.
type WatchServer struct {
	mu         sync.RWMutex
	watchers   map[ResourceKey]Watcher
	clients    map[ResourceKey]map[uint64]chan Event
	nextClient uint64
	eventStore *EventStore
}

// NewWatchServer creates a new WatchServer with a 1000-event history store.
func NewWatchServer() *WatchServer {
	return &WatchServer{
		watchers:   make(map[ResourceKey]Watcher),
		clients:    make(map[ResourceKey]map[uint64]chan Event),
		eventStore: NewEventStore(1000), // Store the most recent 1000 events
	}
}

// RegisterWatcher registers a resource watcher for the given key.
func (s *WatchServer) RegisterWatcher(key ResourceKey, watcher Watcher) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.watchers[key]; exists {
		return fmt.Errorf("watcher already registered for %v", key)
	}

	s.watchers[key] = watcher
	s.clients[key] = make(map[uint64]chan Event)

	// Start the watching goroutine
	go s.startWatching(key, watcher)

	return nil
}

// Watch allows a client to subscribe to resource changes.
func (s *WatchServer) Watch(key ResourceKey, resourceVersion string) (<-chan Event, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	watcher, exists := s.watchers[key]
	if !exists {
		return nil, 0, fmt.Errorf("no watcher registered for %v", key)
	}

	// Assign a client ID
	s.nextClient++
	clientID := s.nextClient

	// Create the event channel
	eventCh := make(chan Event, 100)
	s.clients[key][clientID] = eventCh

	// If resourceVersion is provided, send historical events
	if resourceVersion != "" {
		go s.sendHistoryEvents(key, resourceVersion, eventCh)
	} else {
		// Send current state as ADDED events
		go s.sendInitialState(key, watcher, eventCh)
	}

	return eventCh, clientID, nil
}

// Unwatch removes a client subscription.
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
		// Handle watch error
		return
	}

	for event := range eventCh {
		s.mu.RLock()

		// Store the event and assign a resource version
		resourceVersion := s.eventStore.AddEvent(key, event)
		event.Object = s.addResourceVersion(event.Object, resourceVersion)

		// Dispatch the event to all subscribers
		for _, clientCh := range s.clients[key] {
			select {
			case clientCh <- event:
			default:
				// Skip event to avoid blocking
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
		// Create an ADDED event
		event := Event{
			Type:      Added,
			Object:    obj,
			Timestamp: time.Now(),
		}

		// Store the event and attach the resource version
		resourceVersion := s.eventStore.AddEvent(key, event)
		event.Object = s.addResourceVersion(obj, resourceVersion)

		select {
		case ch <- event:
		case <-time.After(100 * time.Millisecond):
			// Timeout, skip (sendInitialState)
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
			// Timeout, skip (sendHistoryEvents)
		}
	}
}

func (s *WatchServer) addResourceVersion(obj interface{}, version string) interface{} {
	// In a real implementation, this would add a resourceVersion field
	// based on the object type. Simplified: returns a wrapper with version info.
	return struct {
		Object          interface{} `json:"object"`
		ResourceVersion string      `json:"resourceVersion"`
	}{
		Object:          obj,
		ResourceVersion: version,
	}
}
