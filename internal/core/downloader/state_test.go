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

// stateUsableMatrix builds a downloader whose remote info and resume state can
// be tweaked per case.
func stateUsableMatrix() (*downloader, *State) {
	d := &downloader{
		opts: Options{URL: "https://example.com/file.bin", BlockSize: 1 << 20},
		info: &RemoteInfo{
			URL:          "https://example.com/file.bin",
			FinalURL:     "https://example.com/file.bin",
			Size:         4 << 20,
			Ranged:       true,
			ETag:         `"v1"`,
			LastModified: "lm-1",
		},
	}
	st := &State{
		Version:      stateVersion,
		URL:          d.opts.URL,
		Size:         4 << 20,
		ETag:         `"v1"`,
		LastModified: "lm-1",
		BlockSize:    1 << 20,
		DoneBlocks:   []int64{0},
	}
	return d, st
}

func TestStateUsable(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*downloader, *State)
		part   int64
		wantOK bool
	}{
		{name: "matching state", mutate: func(*downloader, *State) {}, part: 4 << 20, wantOK: true},
		{name: "missing partial", mutate: func(*downloader, *State) {}, part: -1, wantOK: false},
		{name: "size changed", mutate: func(_ *downloader, s *State) { s.Size = 8 << 20 }, part: 4 << 20, wantOK: false},
		{name: "url changed", mutate: func(_ *downloader, s *State) { s.URL = "https://example.com/other.bin" }, part: 4 << 20, wantOK: false},
		{name: "etag changed", mutate: func(_ *downloader, s *State) { s.ETag = `"v2"` }, part: 4 << 20, wantOK: false},
		{name: "last-modified changed", mutate: func(_ *downloader, s *State) { s.ETag = ""; s.LastModified = "lm-2" }, part: 4 << 20, wantOK: false},
		{name: "block size changed", mutate: func(_ *downloader, s *State) { s.BlockSize = 2 << 20 }, part: 4 << 20, wantOK: false},
		{
			name: "no validator on either side",
			mutate: func(d *downloader, s *State) {
				d.info.ETag = ""
				d.info.LastModified = ""
				s.ETag = ""
				s.LastModified = ""
			},
			part:   4 << 20,
			wantOK: true,
		},
		{
			name:   "state has no validator but remote does",
			mutate: func(_ *downloader, s *State) { s.ETag = ""; s.LastModified = "" },
			part:   4 << 20,
			wantOK: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, st := stateUsableMatrix()
			c.mutate(d, st)
			reason, ok := d.stateUsable(st, c.part)
			if ok != c.wantOK {
				t.Fatalf("stateUsable() ok = %v (%s), want %v", ok, reason, c.wantOK)
			}
			if !ok && reason == "" {
				t.Error("a refused resume must explain why")
			}
		})
	}
}

func TestStateUsableIgnoresBlockSizeWhenNotExplicit(t *testing.T) {
	d, st := stateUsableMatrix()
	d.opts.BlockSize = 0 // auto: the state's layout wins, see blockSizeFor
	st.BlockSize = 2 << 20
	if reason, ok := d.stateUsable(st, 4<<20); !ok {
		t.Fatalf("stateUsable() = %v, want usable when no explicit --block-size", reason)
	}
	d.state = st
	if got := d.blockSizeFor(); got != 2<<20 {
		t.Errorf("blockSizeFor() = %d, want the state's %d", got, 2<<20)
	}
}
