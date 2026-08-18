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

func TestListHosts(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	hosts, err := db.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts on empty db failed: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected 0 hosts on empty db, got %d", len(hosts))
	}

	now := time.Now()
	db.Write("cpu.test", map[string]string{"host": "hostB"}, now, 1.0)
	db.Write("cpu.test", map[string]string{"host": "hostA"}, now, 2.0)
	db.Write("mem.test", map[string]string{"host": "hostA"}, now, 3.0)
	db.Write("net.test", nil, now, 4.0)

	hosts, err = db.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts failed: %v", err)
	}
	want := []string{"hostA", "hostB"}
	if len(hosts) != len(want) {
		t.Fatalf("expected %v, got %v", want, hosts)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("expected %v, got %v", want, hosts)
		}
	}
}

func TestQueryAllHostsGroupsPerSeries(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	db.Write("cpu.used.percent", map[string]string{"host": "hostA"}, now, 10.0)
	db.Write("cpu.used.percent", map[string]string{"host": "hostA"}, now.Add(10*time.Second), 11.0)
	db.Write("cpu.used.percent", map[string]string{"host": "hostB"}, now, 20.0)

	metrics, err := db.Query("cpu.used.percent", nil, now.Add(-1*time.Hour), now.Add(1*time.Hour), 0)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 series, got %d", len(metrics))
	}

	// Two series: hostA (2 points) and hostB (1 point).
	var gotA, gotB *Metric
	for i := range metrics {
		if metrics[i].Tags["host"] == "hostA" {
			gotA = &metrics[i]
		}
		if metrics[i].Tags["host"] == "hostB" {
			gotB = &metrics[i]
		}
	}
	if gotA == nil || gotB == nil {
		t.Fatalf("expected series for hostA and hostB, got %v", metrics)
	}
	if len(gotA.Points) != 2 {
		t.Errorf("expected 2 points for hostA, got %d", len(gotA.Points))
	}
	if len(gotB.Points) != 1 {
		t.Errorf("expected 1 point for hostB, got %d", len(gotB.Points))
	}
	if gotA.Points[0].Value != 10.0 || gotB.Points[0].Value != 20.0 {
		t.Errorf("unexpected values: hostA=%v hostB=%v", gotA.Points, gotB.Points)
	}
}

func TestCompactPrunesMeta(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	db.Write("cpu.test", map[string]string{"host": "gone"}, now.Add(-2*time.Hour), 1.0)

	if err := db.Compact(30 * time.Minute); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}

	hosts, err := db.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts failed: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected meta pruned for expired series, got hosts %v", hosts)
	}
}

func TestStatsAndListMetricNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	now := time.Now()
	db.Write("cpu.used.percent", map[string]string{"host": "a"}, now, 1.0)
	db.Write("cpu.used.percent", map[string]string{"host": "a"}, now.Add(time.Second), 2.0)
	db.Write("memory.used.bytes", map[string]string{"host": "b"}, now, 3.0)

	stats, err := db.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.Points != 3 {
		t.Fatalf("expected 3 points, got %d", stats.Points)
	}
	if stats.Series != 2 {
		t.Fatalf("expected 2 series, got %d", stats.Series)
	}

	names, err := db.ListMetricNames()
	if err != nil {
		t.Fatalf("ListMetricNames failed: %v", err)
	}
	if len(names) != 2 || names[0] != "cpu.used.percent" || names[1] != "memory.used.bytes" {
		t.Fatalf("unexpected metric names: %v", names)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Read-only open sees the same counts and works while the file is closed.
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly failed: %v", err)
	}
	defer ro.Close()
	roStats, err := ro.Stats()
	if err != nil {
		t.Fatalf("read-only Stats failed: %v", err)
	}
	if roStats.Points != 3 || roStats.Series != 2 {
		t.Fatalf("read-only stats mismatch: %+v", roStats)
	}
}

func TestOpenReadOnlyMissing(t *testing.T) {
	if _, err := OpenReadOnly(filepath.Join(t.TempDir(), "nope.db")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestOpenReadOnlyLocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if _, err := OpenReadOnly(path); err == nil {
		t.Fatal("expected read-only open to fail while writer holds the lock")
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER") == "1" {
		helperServe()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
