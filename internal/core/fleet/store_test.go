package fleet

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "fleet.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreAgentCRUD(t *testing.T) {
	s := newTestStore(t)

	if a, _ := s.GetAgent("h1"); a != nil {
		t.Fatal("expected no agent before creation")
	}
	if err := s.PutAgent(&Agent{Hostname: "h1", Groups: []string{"prod"}, LastSeen: 100}); err != nil {
		t.Fatalf("PutAgent: %v", err)
	}
	a, err := s.GetAgent("h1")
	if err != nil || a == nil {
		t.Fatalf("GetAgent: %v %v", a, err)
	}
	if a.Hostname != "h1" || len(a.Groups) != 1 || a.Groups[0] != "prod" {
		t.Fatalf("agent not round-tripped: %+v", a)
	}

	if err := s.UpdateAgentLastSeen("h1"); err != nil {
		t.Fatalf("UpdateAgentLastSeen: %v", err)
	}
	a, _ = s.GetAgent("h1")
	if a.LastSeen <= 100 {
		t.Fatalf("last_seen not updated: %d", a.LastSeen)
	}

	agents, err := s.ListAgents()
	if err != nil || len(agents) != 1 {
		t.Fatalf("ListAgents: %v %v", agents, err)
	}
}

func TestStoreJobAndRuns(t *testing.T) {
	s := newTestStore(t)

	job := &Job{ID: "abc", Command: "echo hi", Targets: []string{"h1", "h2"}}
	if err := s.PutJob(job); err != nil {
		t.Fatalf("PutJob: %v", err)
	}
	got, _ := s.GetJob("abc")
	if got == nil || got.Command != "echo hi" {
		t.Fatalf("GetJob: %+v", got)
	}

	if err := s.CreatePendingRun("abc", "h1"); err != nil {
		t.Fatalf("CreatePendingRun: %v", err)
	}
	if err := s.CreatePendingRun("abc", "h2"); err != nil {
		t.Fatalf("CreatePendingRun: %v", err)
	}

	// Claim h1's run.
	claimed, err := s.ClaimNextRun("h1")
	if err != nil || claimed == nil || claimed.ID != "abc" {
		t.Fatalf("ClaimNextRun(h1): %+v %v", claimed, err)
	}
	run, _ := s.GetJobRun("abc", "h1")
	if run.State != JobRunRunning {
		t.Fatalf("expected running, got %s", run.State)
	}

	// h1 has no more pending work.
	if again, _ := s.ClaimNextRun("h1"); again != nil {
		t.Fatalf("expected no more work for h1, got job %s", again.ID)
	}

	// h2 still pending.
	run2, _ := s.GetJobRun("abc", "h2")
	if run2.State != JobRunPending {
		t.Fatalf("expected h2 pending, got %s", run2.State)
	}

	// Complete h1 as failed; job status should be failed.
	run.State = JobRunFailed
	run.FinishedAt = time.Now().Unix()
	run.Error = "boom"
	if err := s.PutJobRun(run); err != nil {
		t.Fatalf("PutJobRun: %v", err)
	}
	status, _ := s.GetJobStatus("abc")
	if status != JobRunFailed {
		t.Fatalf("expected failed, got %s", status)
	}

	// Complete h2 as succeeded; job status still failed (one failed run).
	run2.State = JobRunSucceeded
	run2.FinishedAt = time.Now().Unix()
	if err := s.PutJobRun(run2); err != nil {
		t.Fatalf("PutJobRun: %v", err)
	}
	status, _ = s.GetJobStatus("abc")
	if status != JobRunFailed {
		t.Fatalf("expected failed, got %s", status)
	}
}

func TestStoreOutputAppendAndTruncate(t *testing.T) {
	s := newTestStore(t)

	if err := s.AppendRunOutput("abc", "h1", "line1\n"); err != nil {
		t.Fatalf("AppendRunOutput: %v", err)
	}
	if err := s.AppendRunOutput("abc", "h1", "line2\n"); err != nil {
		t.Fatalf("AppendRunOutput: %v", err)
	}
	out, _ := s.GetJobRunOutput("abc", "h1")
	if out != "line1\nline2\n" {
		t.Fatalf("expected appended output, got %q", out)
	}

	// Force truncation with a chunk larger than the cap.
	big := strings.Repeat("x", maxRunOutput+100)
	if err := s.AppendRunOutput("abc", "h1", big); err != nil {
		t.Fatalf("AppendRunOutput big: %v", err)
	}
	out, _ = s.GetJobRunOutput("abc", "h1")
	if !strings.HasPrefix(out, "[truncated]") {
		t.Fatalf("expected truncation marker, got prefix %q", out[:20])
	}
	if len(out) > maxRunOutput+len("[truncated]\n") {
		t.Fatalf("output exceeds cap: %d", len(out))
	}

	outputs, _ := s.GetJobOutput("abc")
	if outputs["h1"] == "" {
		t.Fatal("GetJobOutput missing h1")
	}
}

func TestStoreListJobsOrder(t *testing.T) {
	s := newTestStore(t)
	older := &Job{ID: "a", CreatedAt: 100}
	newer := &Job{ID: "b", CreatedAt: 200}
	s.PutJob(older)
	s.PutJob(newer)
	jobs, _ := s.ListJobs(0)
	if len(jobs) != 2 || jobs[0].ID != "b" {
		t.Fatalf("expected newest first, got %+v", jobs)
	}
	jobs, _ = s.ListJobs(1)
	if len(jobs) != 1 || jobs[0].ID != "b" {
		t.Fatalf("limit not applied: %+v", jobs)
	}
}
