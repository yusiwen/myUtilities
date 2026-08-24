package watcher

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileWatcher monitors local file system changes.
type FileWatcher struct {
	path      string
	interval  time.Duration
	stopChan  chan struct{}
	lastState map[string]FileState // file path -> state
}

type FileState struct {
	Size     int64
	ModTime  time.Time
	Checksum string // Simplified checksum (MD5 of first 8KB)
}

func NewFileWatcher(path string, interval time.Duration) *FileWatcher {
	return &FileWatcher{
		path:     path,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (w *FileWatcher) Watch(ctx context.Context) (<-chan Event, error) {
	eventCh := make(chan Event, 10)

	// Initialize initial state
	if err := w.scanFiles(); err != nil {
		return nil, err
	}

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		defer close(eventCh)

		for {
			select {
			case <-ticker.C:
				w.detectChanges(eventCh)
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventCh, nil
}

func (w *FileWatcher) Stop() {
	close(w.stopChan)
}

func (w *FileWatcher) List() ([]interface{}, error) {
	stateMap, err := scanPath(w.path)
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, 0, len(stateMap))
	for path := range stateMap {
		result = append(result, path)
	}
	return result, nil
}

// getFileState retrieves the state of a single file.
func getFileState(filePath string, fileInfo os.FileInfo) (FileState, error) {
	checksum, err := calculateChecksum(filePath)
	if err != nil {
		return FileState{}, fmt.Errorf("failed to calculate checksum for %s: %w", filePath, err)
	}

	return FileState{
		Size:     fileInfo.Size(),
		ModTime:  fileInfo.ModTime(),
		Checksum: checksum,
	}, nil
}

// scanPath scans a path (file or directory) and returns a map of file states.
func scanPath(path string) (map[string]FileState, error) {
	stateMap := make(map[string]FileState)

	// Check if the path exists
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access path %s: %w", path, err)
	}

	// Handle single file directly
	if !fileInfo.IsDir() {
		state, err := getFileState(path, fileInfo)
		if err != nil {
			return nil, err
		}
		stateMap[path] = state
		return stateMap, nil
	}

	// Walk the directory tree
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		state, err := getFileState(filePath, info)
		if err != nil {
			return err
		}

		stateMap[filePath] = state
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory %s: %w", path, err)
	}

	return stateMap, nil
}

// handleError processes an error, optionally sending it to the event channel.
func handleError(err error, message string, eventCh chan<- Event) error {
	formattedErr := fmt.Errorf("%s: %w", message, err)

	// Send error event if channel provided
	if eventCh != nil {
		eventCh <- Event{
			Type:      Error,
			Object:    formattedErr.Error(),
			Timestamp: time.Now(),
		}
		return nil
	}

	// Otherwise return the error
	return formattedErr
}

func (w *FileWatcher) scanFiles() error {
	stateMap, err := scanPath(w.path)
	if err != nil {
		return err
	}

	w.lastState = stateMap
	return nil
}

// calculateChecksum computes the MD5 checksum of a file.
// For efficiency, only the first 8KB are read, which is sufficient to detect most file changes.
// Returns the hex-encoded MD5 hash string.
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Read only the first 8KB for the checksum
	buffer := make([]byte, 8*1024)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	hash := md5.Sum(buffer[:n])
	return hex.EncodeToString(hash[:]), nil
}

// compareStates compares current and previous file states, emitting change events.
func compareStates(currentState, lastState map[string]FileState, eventCh chan<- Event) {
	// Detect new and modified files
	for path, state := range currentState {
		oldState, exists := lastState[path]
		if !exists {
			// New file
			eventCh <- Event{
				Type:      Added,
				Object:    path,
				Timestamp: time.Now(),
			}
		} else if oldState.Size != state.Size ||
			oldState.ModTime != state.ModTime ||
			oldState.Checksum != state.Checksum {
			// Modified file — compare individual fields
			eventCh <- Event{
				Type:      Modified,
				Object:    path,
				Timestamp: time.Now(),
			}
		}
	}

	// Detect deleted files
	for path := range lastState {
		if _, exists := currentState[path]; !exists {
			// Deleted file
			eventCh <- Event{
				Type:      Deleted,
				Object:    path,
				Timestamp: time.Now(),
			}
		}
	}
}

// detectChanges scans the file system for changes and sends change events to eventCh.
func (w *FileWatcher) detectChanges(eventCh chan<- Event) {
	// Scan file system and get current state
	currentState, err := scanPath(w.path)
	if err != nil {
		handleError(err, fmt.Sprintf("Failed to scan path %s", w.path), eventCh)
		return
	}

	// Compare states and emit events
	compareStates(currentState, w.lastState, eventCh)

	// Update tracked state
	w.lastState = currentState
}
