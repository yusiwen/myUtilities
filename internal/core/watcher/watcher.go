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

// subscription is one Watch client.
//
// stop is closed by Unwatch to tell the initial-state and history senders to
// give up; ch itself is deliberately never closed, because those senders run
// without the server lock and a close would turn an ordinary unsubscribe into a
// "send on closed channel" panic. Callers stop reading once they call Unwatch.
type subscription struct {
	ch   chan Event
	stop chan struct{}
}

// WatchServer dispatches events to registered watchers and their subscribers.
type WatchServer struct {
	mu         sync.RWMutex
	watchers   map[ResourceKey]Watcher
	clients    map[ResourceKey]map[uint64]*subscription
	nextClient uint64
	eventStore *EventStore
}

// NewWatchServer creates a new WatchServer with a 1000-event history store.
func NewWatchServer() *WatchServer {
	return &WatchServer{
		watchers:   make(map[ResourceKey]Watcher),
		clients:    make(map[ResourceKey]map[uint64]*subscription),
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
	s.clients[key] = make(map[uint64]*subscription)

	// Start the watching goroutine
	go s.startWatching(key, watcher)

	return nil
}

// Watch allows a client to subscribe to resource changes. The returned channel
// is not closed by Unwatch; the caller stops reading after unsubscribing.
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

	// Create the subscription
	sub := &subscription{
		ch:   make(chan Event, 100),
		stop: make(chan struct{}),
	}
	s.clients[key][clientID] = sub

	// If resourceVersion is provided, send historical events
	if resourceVersion != "" {
		go s.sendHistoryEvents(key, resourceVersion, sub)
	} else {
		// Send current state as ADDED events
		go s.sendInitialState(key, watcher, sub)
	}

	return sub.ch, clientID, nil
}

// Unwatch removes a client subscription and tells its senders to stop. It is
// safe to call more than once.
func (s *WatchServer) Unwatch(key ResourceKey, clientID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if clients, ok := s.clients[key]; ok {
		if sub, exists := clients[clientID]; exists {
			close(sub.stop)
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
		for _, sub := range s.clients[key] {
			select {
			case sub.ch <- event:
			default:
				// Skip event to avoid blocking
			}
		}

		s.mu.RUnlock()
	}
}

func (s *WatchServer) sendInitialState(key ResourceKey, watcher Watcher, sub *subscription) {
	resources, err := watcher.List()
	if err != nil {
		sendEvent(sub, Event{Type: Error, Object: err.Error()})
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

		if !sendEvent(sub, event) {
			return
		}
	}
}

func (s *WatchServer) sendHistoryEvents(key ResourceKey, resourceVersion string, sub *subscription) {
	events, err := s.eventStore.GetEventsAfter(key, resourceVersion)
	if err != nil {
		sendEvent(sub, Event{Type: Error, Object: err.Error()})
		return
	}

	for _, event := range events {
		if !sendEvent(sub, event) {
			return
		}
	}
}

// sendEvent delivers one event unless the subscription was cancelled or the
// subscriber is too slow; it reports whether the sender should continue.
func sendEvent(sub *subscription, event Event) bool {
	select {
	case sub.ch <- event:
		return true
	case <-sub.stop:
		return false
	case <-time.After(100 * time.Millisecond):
		// Timeout, skip this event.
		return true
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
