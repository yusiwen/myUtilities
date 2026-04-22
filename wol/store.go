package wol

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	bolt "github.com/coreos/bbolt"
)

const (
	bucketName = "Aliases"
)

// MacIface holds a MAC Address to wake up, along with an optionally specified
// default interface to use when typically waking up said interface.
type MacIface struct {
	Mac   string
	Iface string
}

// DecodeToMacIface takes a byte buffer and converts decodes it using the gob
// package to a MacIface entry.
func DecodeToMacIface(buf *bytes.Buffer) (MacIface, error) {
	var entry MacIface
	decoder := gob.NewDecoder(buf)
	err := decoder.Decode(&entry)
	return entry, err
}

// EncodeFromMacIface takes a MAC and an Iface and encodes a gob with a MacIface
// entry.
func EncodeFromMacIface(mac, iface string) (*bytes.Buffer, error) {
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

	// Create bucket if not exists
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
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
		entry, err = DecodeToMacIface(bytes.NewBuffer(value))
		return err
	})
	return entry, err
}

// Set adds or updates a hostname mapping.
func (s *Store) Set(hostname, mac, iface string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	buf, err := EncodeFromMacIface(mac, iface)
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
			entry, err := DecodeToMacIface(bytes.NewBuffer(v))
			if err != nil {
				return err
			}
			result[string(k)] = entry
		}
		return nil
	})
	return result, err
}
