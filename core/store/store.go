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

const (
	bucketName   = "Aliases"
	bootBucket   = "Boot"
	statusBucket = "Status"
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
		if _, err := tx.CreateBucketIfNotExists([]byte(bootBucket)); err != nil {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte(statusBucket))
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

// RecordBoot stores a boot timestamp for a given hostname.
func (s *Store) RecordBoot(hostname string, t time.Time) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(t.Unix()); err != nil {
		return err
	}

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bootBucket))
		return bucket.Put([]byte(hostname), buf.Bytes())
	})
}

// GetBootTime retrieves the last boot time for a hostname.
func (s *Store) GetBootTime(hostname string) (time.Time, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var ts int64
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(bootBucket))
		value := bucket.Get([]byte(hostname))
		if value == nil {
			return fmt.Errorf("hostname %q not found in boot bucket", hostname)
		}
		return gob.NewDecoder(bytes.NewBuffer(value)).Decode(&ts)
	})
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(ts, 0), nil
}

// SetStatus stores the current status for a hostname (e.g., "boot", "shutdown").
func (s *Store) SetStatus(hostname, status string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(statusBucket))
		return bucket.Put([]byte(hostname), []byte(status))
	})
}

// GetStatus retrieves the current status for a hostname.
// Returns an empty string if no status has been recorded.
func (s *Store) GetStatus(hostname string) (string, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var status string
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(statusBucket))
		value := bucket.Get([]byte(hostname))
		if value != nil {
			status = string(value)
		}
		return nil
	})
	return status, err
}
