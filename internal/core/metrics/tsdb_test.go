package metrics

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestWriteAndQuery(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	tags := map[string]string{"host": "testbox", "cpu": "0"}

	if err := db.Write("cpu.used.percent", tags, now, 45.2); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := db.Write("cpu.used.percent", tags, now.Add(10*time.Second), 46.1); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if err := db.Write("cpu.used.percent", tags, now.Add(20*time.Second), 47.0); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	metrics, err := db.Query("cpu.used.percent", tags, now.Add(-1*time.Hour), now.Add(1*time.Hour), 0)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	m := metrics[0]
	if m.Name != "cpu.used.percent" {
		t.Errorf("expected name cpu.used.percent, got %s", m.Name)
	}

	if len(m.Points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(m.Points))
	}

	if m.Points[0].Value != 45.2 {
		t.Errorf("expected first value 45.2, got %f", m.Points[0].Value)
	}
}

func TestWriteBatch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	batch := []Metric{
		{Name: "cpu.used.percent", Tags: map[string]string{"host": "a"}, Points: []DataPoint{{Timestamp: now.UnixNano(), Value: 50.0}}},
		{Name: "memory.used.bytes", Tags: map[string]string{"host": "a"}, Points: []DataPoint{{Timestamp: now.UnixNano(), Value: 8192}}},
	}

	if err := db.WriteBatch(batch); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	names, err := db.ListMetrics()
	if err != nil {
		t.Fatalf("ListMetrics failed: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 metric names, got %d: %v", len(names), names)
	}
}

func TestListMetrics(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	names, err := db.ListMetrics()
	if err != nil {
		t.Fatalf("ListMetrics failed on empty db: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected 0 names on empty db, got %d", len(names))
	}

	now := time.Now()
	db.Write("cpu.test", nil, now, 1.0)
	db.Write("mem.test", nil, now, 2.0)

	names, err = db.ListMetrics()
	if err != nil {
		t.Fatalf("ListMetrics failed: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}
}

func TestCompact(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	tags := map[string]string{"host": "test"}

	// Write old point (1 hour ago) and new point (now)
	db.Write("cpu.test", tags, now.Add(-1*time.Hour), 10.0)
	db.Write("cpu.test", tags, now, 20.0)

	// Compact with 30 minute retention → old point should be removed
	if err := db.Compact(30 * time.Minute); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	metrics, err := db.Query("cpu.test", tags, now.Add(-2*time.Hour), now.Add(1*time.Hour), 0)
	if err != nil {
		t.Fatalf("Query after compact failed: %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric after compact, got %d", len(metrics))
	}
	if len(metrics[0].Points) != 1 {
		t.Fatalf("expected 1 point after compact, got %d", len(metrics[0].Points))
	}
	if metrics[0].Points[0].Value != 20.0 {
		t.Errorf("expected remaining value 20.0, got %f", metrics[0].Points[0].Value)
	}
}

func TestCompactZeroRetention(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	db.Write("cpu.test", nil, now, 1.0)

	// Compact with 0 retention should do nothing
	if err := db.Compact(0); err != nil {
		t.Fatalf("Compact(0) failed: %v", err)
	}

	metrics, err := db.Query("cpu.test", nil, now.Add(-1*time.Hour), now.Add(1*time.Hour), 0)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(metrics) != 1 || len(metrics[0].Points) != 1 {
		t.Fatal("expected data to remain after Compact(0)")
	}
}

func TestQueryLimit(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	for i := 0; i < 100; i++ {
		db.Write("limit.test", nil, now.Add(time.Duration(i)*time.Second), float64(i))
	}

	metrics, err := db.Query("limit.test", nil, now, now.Add(200*time.Second), 10)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
	if len(metrics[0].Points) > 10 {
		t.Fatalf("expected at most 10 points with limit=10, got %d", len(metrics[0].Points))
	}
}

func TestTagsHashDifferentHosts(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	db.Write("cpu.test", map[string]string{"host": "hostA"}, now, 10.0)
	db.Write("cpu.test", map[string]string{"host": "hostB"}, now, 20.0)

	metricsA, _ := db.Query("cpu.test", map[string]string{"host": "hostA"}, now.Add(-1*time.Hour), now.Add(1*time.Hour), 0)
	metricsB, _ := db.Query("cpu.test", map[string]string{"host": "hostB"}, now.Add(-1*time.Hour), now.Add(1*time.Hour), 0)

	if len(metricsA) != 1 || len(metricsA[0].Points) != 1 {
		t.Fatal("expected 1 point for hostA")
	}
	if len(metricsB) != 1 || len(metricsB[0].Points) != 1 {
		t.Fatal("expected 1 point for hostB")
	}
	if metricsA[0].Points[0].Value != 10.0 {
		t.Errorf("expected hostA value 10.0, got %f", metricsA[0].Points[0].Value)
	}
	if metricsB[0].Points[0].Value != 20.0 {
		t.Errorf("expected hostB value 20.0, got %f", metricsB[0].Points[0].Value)
	}
}

func TestParseRetention(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"30d", 30 * 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"24h", 24 * time.Hour},
		{"1h", 1 * time.Hour},
	}
	for _, tt := range tests {
		got := ParseRetention(tt.input)
		if got != tt.want {
			t.Errorf("ParseRetention(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
