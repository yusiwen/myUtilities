package downloader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// stateVersion is the resume-state file format version.
const stateVersion = 1

// State is the resume bookkeeping for one in-flight download. It lives next to
// the partial file so that an interrupted transfer can be continued later.
type State struct {
	Version      int       `json:"version"`
	URL          string    `json:"url"`
	FinalURL     string    `json:"final_url,omitempty"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	BlockSize    int64     `json:"block_size"`
	TotalBlocks  int64     `json:"total_blocks"`
	DoneBlocks   []int64   `json:"done_blocks"`
	Output       string    `json:"output"`
	StartedAt    time.Time `json:"started_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// PartPath is the partial file written during the transfer.
func PartPath(output string) string { return output + ".part" }

// StatePath is the sidecar file holding resume bookkeeping.
func StatePath(output string) string { return output + ".part.mu-dl.json" }

// LoadState reads the resume state for output. It returns (nil, nil) when no
// state file exists, and an error when the file exists but cannot be parsed
// (callers treat that as "start over").
func LoadState(output string) (*State, error) {
	data, err := os.ReadFile(StatePath(output))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("corrupt resume state %s: %w", StatePath(output), err)
	}
	if st.Version != stateVersion {
		return nil, fmt.Errorf("unsupported resume state version %d (want %d)", st.Version, stateVersion)
	}
	return &st, nil
}

// Save writes the state atomically (temporary file + rename) so a crash never
// leaves a half-written JSON document behind.
func (s *State) Save() error {
	path := StatePath(s.Output)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".mu-dl-state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// RemoveState deletes the resume state, ignoring "not found".
func RemoveState(output string) error {
	err := os.Remove(StatePath(output))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
