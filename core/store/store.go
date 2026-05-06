package store

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "github.com/coreos/bbolt"
)

// Event represents a boot or shutdown notification event.
type Event struct {
	Type      string    `json:"type"`     // "boot" or "shutdown"
	Timestamp time.Time `json:"timestamp"`
}

const (
	bucketName   = "Aliases"
	eventsBucket = "Events"
)

// MacIface holds a MAC Address to wake up, along with an optionally specified
// default interface to use when typically waking up said interface.
type MacIface struct {
	Mac   string
	Iface string
}

// decodeToMacIface takes a byte buffer and converts decodes it using the gob
// package to a MacIface entry.
func decodeToMacIface(buf *bytes.Buffer) (MacIface, error) {
	var entry MacIface
	decoder := gob.NewDecoder(buf)
	err := decoder.Decode(&entry)
	return entry, err
}

// encodeFromMacIface takes a MAC and an Iface and encodes a gob with a MacIface
// entry.
func encodeFromMacIface(mac, iface string) (*bytes.Buffer, error) {
	buf := bytes.NewBuffer(nil)
	entry := MacIface{mac, iface}
	err := gob.NewEncoder(buf).Encode(entry)
	return buf, err
}

// Store holds a pointer to a mutex which will be acquired and released as
// transactions are carried out on the `db`.
type Store struct {
	mtx *sync.Mutex
	db  *bolt.DB
}

// OpenStore opens or creates a BoltDB database at the given path.
// If the path starts with "~", it will be expanded to the user's home directory.
func OpenStore(dbPath string) (*Store, error) {
	// Expand tilde to home directory
	if strings.HasPrefix(dbPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %v", err)
		}
		dbPath = filepath.Join(home, dbPath[2:])
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	db, err := bolt.Open(dbPath, 0660, nil)
	if err != nil {
		return nil, err
	}

	// Create buckets if not exists
	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(bucketName)); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte(eventsBucket))
		return err
	}); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{
		mtx: &sync.Mutex{},
		db:  db,
	}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.db.Close()
}

// Get retrieves a MacIface from the store based on a hostname.
func (s *Store) Get(hostname string) (MacIface, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var entry MacIface
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		value := bucket.Get([]byte(hostname))
		if value == nil {
			return fmt.Errorf("hostname %q not found", hostname)
		}
		var err error
		entry, err = decodeToMacIface(bytes.NewBuffer(value))
		return err
	})
	return entry, err
}

// Set adds or updates a hostname mapping.
func (s *Store) Set(hostname, mac, iface string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	buf, err := encodeFromMacIface(mac, iface)
	if err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		return bucket.Put([]byte(hostname), buf.Bytes())
	})
}

// Delete removes a hostname mapping.
func (s *Store) Delete(hostname string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		return bucket.Delete([]byte(hostname))
	})
}

// List returns all hostname mappings.
func (s *Store) List() (map[string]MacIface, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	result := make(map[string]MacIface)
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bucketName))
		cursor := bucket.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			entry, err := decodeToMacIface(bytes.NewBuffer(v))
			if err != nil {
				return err
			}
			result[string(k)] = entry
		}
		return nil
	})
	return result, err
}

// maxEvents is the maximum number of events to keep per hostname.
const maxEvents = 10

// RecordEvent appends a boot/shutdown event for a hostname and prunes to maxEvents.
func (s *Store) RecordEvent(hostname string, eventType string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(eventsBucket))
		events := loadEvents(bucket, hostname)

		// Prepend new event
		events = append([]Event{{Type: eventType, Timestamp: time.Now()}}, events...)

		// Prune to maxEvents
		if len(events) > maxEvents {
			events = events[:maxEvents]
		}

		return saveEvents(bucket, hostname, events)
	})
}

// GetEvents returns the latest n events for a hostname (up to maxEvents).
func (s *Store) GetEvents(hostname string, limit int) ([]Event, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if limit <= 0 || limit > maxEvents {
		limit = maxEvents
	}

	var events []Event
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(eventsBucket))
		events = loadEvents(bucket, hostname)
		return nil
	})
	if len(events) > limit {
		events = events[:limit]
	}
	return events, err
}

func loadEvents(bucket *bolt.Bucket, hostname string) []Event {
	value := bucket.Get([]byte(hostname))
	if value == nil {
		return nil
	}
	var events []Event
	if err := gob.NewDecoder(bytes.NewBuffer(value)).Decode(&events); err != nil {
		return nil
	}
	return events
}

func saveEvents(bucket *bolt.Bucket, hostname string, events []Event) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(events); err != nil {
		return err
	}
	return bucket.Put([]byte(hostname), buf.Bytes())
}
