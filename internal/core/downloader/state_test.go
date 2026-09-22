package downloader

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	out := filepath.Join(t.TempDir(), "file.bin")
	st := &State{
		Version:      stateVersion,
		URL:          "https://example.com/file.bin",
		FinalURL:     "https://cdn.example.com/file.bin",
		Size:         1 << 20,
		ETag:         `"abc"`,
		LastModified: "Wed, 21 Oct 2026 07:28:00 GMT",
		BlockSize:    1 << 19,
		TotalBlocks:  2,
		DoneBlocks:   []int64{0},
		Output:       out,
		StartedAt:    time.Now().Add(-time.Minute).Truncate(time.Second),
		UpdatedAt:    time.Now().Truncate(time.Second),
	}
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadState(out)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadState returned nil for an existing state file")
	}
	if loaded.Size != st.Size || loaded.ETag != st.ETag || loaded.BlockSize != st.BlockSize {
		t.Errorf("loaded state mismatch: %+v", loaded)
	}
	if len(loaded.DoneBlocks) != 1 || loaded.DoneBlocks[0] != 0 {
		t.Errorf("DoneBlocks = %v, want [0]", loaded.DoneBlocks)
	}

	if err := RemoveState(out); err != nil {
		t.Fatalf("RemoveState: %v", err)
	}
	if loaded, err := LoadState(out); err != nil || loaded != nil {
		t.Errorf("LoadState after remove = (%v, %v), want (nil, nil)", loaded, err)
	}
	if err := RemoveState(out); err != nil {
		t.Errorf("RemoveState on a missing file should not fail: %v", err)
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	out := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(StatePath(out), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(out); err == nil {
		t.Fatal("expected an error for a corrupt state file")
	}
}

func TestLoadStateUnsupportedVersion(t *testing.T) {
	out := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(StatePath(out), []byte(`{"version":99}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(out); err == nil {
		t.Fatal("expected an error for an unsupported state version")
	}
}
