package fleet

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, token string) (*httptest.Server, *Store, *Client, string) {
	t.Helper()
	store := newTestStore(t)
	dataDir := filepath.Join(t.TempDir(), "data")
	cfg := &DispatcherConfig{Token: token, DataDir: dataDir, AgentTimeout: time.Minute}
	mux := http.NewServeMux()
	RegisterHandlers(mux, store, cfg)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, store, NewClient(srv.URL, token), dataDir
}

func submitMultipart(t *testing.T, srvURL, token string, jobJSON string, files map[string]string) (*http.Response, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.WriteField("job", jobJSON)
	for name, content := range files {
		part, err := w.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write([]byte(content))
	}
	w.Close()

	req, _ := http.NewRequest(http.MethodPost, srvURL+"/api/fleet/jobs", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("X-Auth-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	body := new(bytes.Buffer)
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	return resp, body.String()
}

func TestAuthRequired(t *testing.T) {
	srv, _, _, _ := newTestServer(t, "secret")
	resp, err := http.Get(srv.URL + "/api/fleet/jobs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestCreateJobAndFlow(t *testing.T) {
	srv, _, client, dataDir := newTestServer(t, "")

	job := `{"command":"echo hi","targets":["h1"]}`
	resp, body := submitMultipart(t, srv.URL, "", job, map[string]string{"bundle.tar.gz": "payload"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create job: %d %s", resp.StatusCode, body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.ID == "" {
		t.Fatalf("create response: %v %s", err, body)
	}

	// Uploaded file stored with checksum.
	files, err := filepath.Glob(filepath.Join(dataDir, "jobs", created.ID, "files", "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected 1 stored file, got %v (%v)", files, err)
	}
	stored, _ := os.ReadFile(files[0])
	if string(stored) != "payload" {
		t.Fatalf("stored content mismatch: %q", stored)
	}

	// Agent polls and claims the run.
	state, err := client.Poll("h1")
	if err != nil || state == nil || state.JobID != created.ID {
		t.Fatalf("poll: %+v %v", state, err)
	}
	if len(state.Files) != 1 || state.Files[0].Name != "bundle.tar.gz" {
		t.Fatalf("expected bundle.tar.gz in files, got %+v", state.Files)
	}

	// No more work for h1.
	if again, _ := client.Poll("h1"); again != nil {
		t.Fatalf("expected no more work, got %s", again.JobID)
	}

	// Stream output + complete.
	if err := client.UploadOutput("h1", created.ID, "hello\n"); err != nil {
		t.Fatalf("UploadOutput: %v", err)
	}
	if err := client.ReportCompletion("h1", created.ID, true, ""); err != nil {
		t.Fatalf("ReportCompletion: %v", err)
	}

	st, err := client.JobStatus(created.ID)
	if err != nil {
		t.Fatalf("JobStatus: %v", err)
	}
	if st.Status != JobRunSucceeded {
		t.Fatalf("expected succeeded, got %s", st.Status)
	}
	if st.Outputs["h1"] != "hello\n" {
		t.Fatalf("expected output, got %q", st.Outputs["h1"])
	}

	// Download the uploaded file back.
	dl := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := client.DownloadFile("h1", created.ID, "bundle.tar.gz", dl); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if data, _ := os.ReadFile(dl); string(data) != "payload" {
		t.Fatalf("downloaded content mismatch: %q", data)
	}
}

func TestCreateJobValidation(t *testing.T) {
	srv, _, _, _ := newTestServer(t, "")
	for _, job := range []string{
		`{"targets":["h1"]}`,    // no command/recipe
		`{"command":"echo hi"}`, // no targets
		`not json`,              // bad json
	} {
		resp, body := submitMultipart(t, srv.URL, "", job, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("job %q: expected 400, got %d: %s", job, resp.StatusCode, body)
		}
	}
}

func TestRegisterAndListAgents(t *testing.T) {
	_, _, client, _ := newTestServer(t, "")
	if err := client.Register("h1", []string{"prod"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	agents, err := client.ListAgents()
	if err != nil || len(agents) != 1 {
		t.Fatalf("ListAgents: %v %v", agents, err)
	}
	if agents[0].Hostname != "h1" || !agents[0].Online {
		t.Fatalf("unexpected agent: %+v", agents[0])
	}
	if !strings.Contains(agents[0].Groups[0], "prod") {
		t.Fatalf("groups not stored: %v", agents[0].Groups)
	}
}
