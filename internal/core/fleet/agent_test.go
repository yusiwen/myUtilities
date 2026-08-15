package fleet

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAgentEndToEnd runs the full agent loop against a real dispatcher: the
// agent registers, polls, claims a job, executes it, streams output, and
// reports completion.
func TestAgentEndToEnd(t *testing.T) {
	srv, _, client, _ := newTestServer(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = RunAgentContext(ctx, AgentConfig{
			ServerURL:    srv.URL,
			Hostname:     "h1",
			PollInterval: 50 * time.Millisecond,
		})
	}()

	// Wait for registration.
	deadline := time.Now().Add(5 * time.Second)
	for {
		agents, _ := client.ListAgents()
		if len(agents) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent did not register in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	jobID, err := client.SubmitJob(SubmitJobRequest{
		Targets: []string{"h1"},
		Command: "echo agent-ran",
	})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	// Poll until terminal.
	var st *JobStatusResponse
	for {
		st, err = client.JobStatus(jobID)
		if err != nil {
			t.Fatalf("JobStatus: %v", err)
		}
		done := len(st.Runs) > 0
		for _, r := range st.Runs {
			if r.State == JobRunPending || r.State == JobRunRunning {
				done = false
			}
		}
		if done || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if st.Status != JobRunSucceeded {
		t.Fatalf("expected succeeded, got %s (runs: %+v)", st.Status, st.Runs)
	}
	if !strings.Contains(st.Outputs["h1"], "agent-ran") {
		t.Fatalf("output missing 'agent-ran': %q", st.Outputs["h1"])
	}
}

// TestAgentFailingJob verifies failures are reported with an error.
func TestAgentFailingJob(t *testing.T) {
	srv, _, client, _ := newTestServer(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = RunAgentContext(ctx, AgentConfig{
			ServerURL:    srv.URL,
			Hostname:     "h1",
			PollInterval: 50 * time.Millisecond,
		})
	}()

	jobID, err := client.SubmitJob(SubmitJobRequest{
		Targets: []string{"h1"},
		Command: "exit 7",
	})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var st *JobStatusResponse
	for {
		st, err = client.JobStatus(jobID)
		if err != nil {
			t.Fatalf("JobStatus: %v", err)
		}
		if len(st.Runs) > 0 && st.Runs[0].State == JobRunFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not fail in time: %+v", st.Runs)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(st.Runs[0].Error, "exit code 7") {
		t.Fatalf("expected exit code 7 in error, got %q", st.Runs[0].Error)
	}
}

// TestAgentRecipeWithFiles runs a recipe that consumes an uploaded archive.
func TestAgentRecipeWithFiles(t *testing.T) {
	srv, _, client, dataDir := newTestServer(t, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = RunAgentContext(ctx, AgentConfig{
			ServerURL:    srv.URL,
			Hostname:     "h1",
			PollInterval: 50 * time.Millisecond,
		})
	}()

	// Build a local archive to upload.
	archive := filepath.Join(t.TempDir(), "bundle.tar.gz")
	writeTarGz(t, archive, map[string]string{"payload.txt": "payload-content"})

	recipe := `
tasks:
  verify:
    command: cat payload.txt
`
	jobID, err := client.SubmitJob(SubmitJobRequest{
		Targets: []string{"h1"},
		Recipe:  recipe,
		Files:   []string{archive},
	})
	if err != nil {
		t.Fatalf("SubmitJob: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var st *JobStatusResponse
	for {
		st, err = client.JobStatus(jobID)
		if err != nil {
			t.Fatalf("JobStatus: %v", err)
		}
		if len(st.Runs) > 0 && st.Runs[0].State != JobRunPending && st.Runs[0].State != JobRunRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish in time: %+v", st.Runs)
		}
		time.Sleep(50 * time.Millisecond)
	}

	if st.Status != JobRunSucceeded {
		t.Fatalf("expected succeeded, got %s (error: %q)", st.Status, st.Runs[0].Error)
	}
	if !strings.Contains(st.Outputs["h1"], "payload-content") {
		t.Fatalf("output missing extracted payload: %q", st.Outputs["h1"])
	}
	if _, err := filepath.Glob(filepath.Join(dataDir, "jobs", jobID, "files", "*")); err != nil {
		t.Fatal(err)
	}
}
