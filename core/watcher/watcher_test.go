package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resourceKey(name string) ResourceKey {
	return ResourceKey{Group: "test", Version: "v1", Resource: name, Namespace: "default", Name: name}
}

func TestEventStoreAddAndGet(t *testing.T) {
	s := NewEventStore(100)
	key := resourceKey("test-resource")

	e1 := Event{Type: Added, Object: "obj1", Timestamp: time.Now()}
	e2 := Event{Type: Modified, Object: "obj2", Timestamp: time.Now()}
	e3 := Event{Type: Deleted, Object: "obj3", Timestamp: time.Now()}

	v1 := s.AddEvent(key, e1)
	v2 := s.AddEvent(key, e2)
	v3 := s.AddEvent(key, e3)

	// GetEventsAfter v1 should return e2, e3
	events, err := s.GetEventsAfter(key, v1)
	if err != nil {
		t.Fatalf("GetEventsAfter: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Check Objects are wrapped with resourceVersion
	obj2 := extractObject(t, events[0].Object)
	if obj2 != "obj2" {
		t.Errorf("expected obj2, got %v", obj2)
	}
	checkHasVersion(t, events[0].Object, v2)

	obj3 := extractObject(t, events[1].Object)
	if obj3 != "obj3" {
		t.Errorf("expected obj3, got %v", obj3)
	}
	checkHasVersion(t, events[1].Object, v3)

	// GetEventsAfter v3 should return empty
	events, err = s.GetEventsAfter(key, v3)
	if err != nil {
		t.Fatalf("GetEventsAfter: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestEventStoreUnknownVersion(t *testing.T) {
	s := NewEventStore(10)
	key := resourceKey("unknown-version")

	_, err := s.GetEventsAfter(key, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}

	s.AddEvent(key, Event{Type: Added, Object: "x"})
	_, err = s.GetEventsAfter(key, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
}

func TestEventStoreMaxSize(t *testing.T) {
	maxSize := 5
	s := NewEventStore(maxSize)
	key := resourceKey("max-size")

	var versions []string
	for i := 0; i < 10; i++ {
		v := s.AddEvent(key, Event{Type: Added, Object: i})
		versions = append(versions, v)
	}

	// Oldest version should have been evicted
	_, err := s.GetEventsAfter(key, versions[0])
	if err == nil {
		t.Fatal("expected error for evicted version")
	}

	// Most recent version should still work
	events, err := s.GetEventsAfter(key, versions[9])
	if err != nil {
		t.Fatalf("GetEventsAfter for latest: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events after latest, got %d", len(events))
	}

	// Should have exactly maxSize events stored
	events, err = s.GetEventsAfter(key, versions[10-maxSize])
	if err != nil {
		t.Fatalf("GetEventsAfter for oldest remaining: %v", err)
	}
	if len(events) != maxSize-1 {
		t.Fatalf("expected %d events, got %d", maxSize-1, len(events))
	}
}

func TestEventStoreMultipleKeys(t *testing.T) {
	s := NewEventStore(100)
	k1 := resourceKey("key1")
	k2 := resourceKey("key2")

	v1 := s.AddEvent(k1, Event{Type: Added, Object: "a"})
	s.AddEvent(k2, Event{Type: Added, Object: "b"})

	events, err := s.GetEventsAfter(k1, v1)
	if err != nil {
		t.Fatalf("GetEventsAfter: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events for k1, got %d", len(events))
	}
}

func TestFileWatcherWatch(t *testing.T) {
	dir := t.TempDir()

	fw := NewFileWatcher(dir, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventCh, err := fw.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Create a file
	file1 := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file1, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-eventCh:
		if ev.Type != Added {
			t.Errorf("expected Added event, got %s", ev.Type)
		}
		if ev.Object.(string) != file1 {
			t.Errorf("expected %s, got %v", file1, ev.Object)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Added event")
	}

	// Modify the file
	if err := os.WriteFile(file1, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-eventCh:
		if ev.Type != Modified {
			t.Errorf("expected Modified event, got %s", ev.Type)
		}
		if ev.Object.(string) != file1 {
			t.Errorf("expected %s, got %v", file1, ev.Object)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Modified event")
	}

	// Delete the file
	if err := os.Remove(file1); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-eventCh:
		if ev.Type != Deleted {
			t.Errorf("expected Deleted event, got %s", ev.Type)
		}
		if ev.Object.(string) != file1 {
			t.Errorf("expected %s, got %v", file1, ev.Object)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Deleted event")
	}

	fw.Stop()
}

func TestFileWatcherList(t *testing.T) {
	dir := t.TempDir()

	fw := NewFileWatcher(dir, time.Second)

	// Empty dir
	list, err := fw.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}

	// Add files
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("c"), 0644)

	list, err = fw.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 files, got %d: %v", len(list), list)
	}
}

func TestWatchServerLifecycle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0644)

	server := NewWatchServer()
	key := resourceKey("file-watch")
	fw := NewFileWatcher(dir, 100*time.Millisecond)

	if err := server.RegisterWatcher(key, fw); err != nil {
		t.Fatalf("RegisterWatcher: %v", err)
	}

	ch, clientID, err := server.Watch(key, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer server.Unwatch(key, clientID)

	// Should receive initial ADDED events
	gotInitial := 0
	timeout := time.After(2 * time.Second)
	for gotInitial < 1 {
		select {
		case ev := <-ch:
			if ev.Type == Added {
				gotInitial++
			}
		case <-timeout:
			t.Fatalf("timeout waiting for initial ADDED, got %d events", gotInitial)
		}
	}
}

// helpers

func extractObject(t *testing.T, obj interface{}) interface{} {
	t.Helper()
	if v, ok := obj.(struct {
		Object          interface{} `json:"object"`
		ResourceVersion string      `json:"resourceVersion"`
	}); ok {
		return v.Object
	}
	t.Fatalf("Object is not versioned: %T %+v", obj, obj)
	return nil
}

func checkHasVersion(t *testing.T, obj interface{}, expectedVersion string) {
	t.Helper()
	if v, ok := obj.(struct {
		Object          interface{} `json:"object"`
		ResourceVersion string      `json:"resourceVersion"`
	}); ok {
		if v.ResourceVersion != expectedVersion {
			t.Errorf("expected version %s, got %s", expectedVersion, v.ResourceVersion)
		}
	} else {
		t.Fatalf("Object is not versioned: %T", obj)
	}
}
