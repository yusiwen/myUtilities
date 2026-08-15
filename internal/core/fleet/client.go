package fleet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Client talks to a fleet dispatcher. It is used by the controller (submit and
// query jobs) and by agents (register, poll, upload output, report completion,
// download files).
type Client struct {
	ServerURL string
	Token     string
	HTTP      *http.Client
}

// NewClient creates a dispatcher client.
func NewClient(serverURL, token string) *Client {
	if !bytes.Contains([]byte(serverURL), []byte("://")) {
		serverURL = "http://" + serverURL
	}
	return &Client{
		ServerURL: serverURL,
		Token:     token,
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
}

// do performs a request with the shared auth token and a JSON body.
func (c *Client) do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.ServerURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("X-Auth-Token", c.Token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.HTTP.Do(req)
}

// SubmitJobRequest describes a job to submit.
type SubmitJobRequest struct {
	Targets []string
	Command string
	Recipe  string
	Vars    map[string]string
	Files   []string // local paths uploaded with the job
}

// SubmitJob uploads a job (metadata + files in one multipart request) and
// returns the assigned job ID.
func (c *Client) SubmitJob(req SubmitJobRequest) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	job := Job{Command: req.Command, Recipe: req.Recipe, Vars: req.Vars, Targets: req.Targets}
	meta, err := json.Marshal(job)
	if err != nil {
		return "", err
	}
	if err := writer.WriteField("job", string(meta)); err != nil {
		return "", err
	}
	for _, path := range req.Files {
		f, err := os.Open(path)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", path, err)
		}
		part, err := writer.CreateFormFile("file", filepath.Base(path))
		if err != nil {
			f.Close()
			return "", err
		}
		if _, err := io.Copy(part, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	resp, err := c.do(http.MethodPost, "/api/fleet/jobs", &buf, writer.FormDataContentType())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("submit job: status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// JobStatus fetches the current status of a job.
func (c *Client) JobStatus(id string) (*JobStatusResponse, error) {
	resp, err := c.do(http.MethodGet, "/api/fleet/jobs/"+url.PathEscape(id), nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("job status: status %d: %s", resp.StatusCode, string(body))
	}
	var out JobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListJobs lists recent jobs.
func (c *Client) ListJobs() ([]Job, error) {
	resp, err := c.do(http.MethodGet, "/api/fleet/jobs", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list jobs: status %d", resp.StatusCode)
	}
	var out struct {
		Jobs []Job `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Jobs, nil
}

// ListAgents lists registered agents with online status.
func (c *Client) ListAgents() ([]Agent, error) {
	resp, err := c.do(http.MethodGet, "/api/fleet/agents", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list agents: status %d", resp.StatusCode)
	}
	var out struct {
		Agents []Agent `json:"agents"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Agents, nil
}

// Register registers the agent with the dispatcher.
func (c *Client) Register(hostname string, groups []string) error {
	body, _ := json.Marshal(map[string]interface{}{"hostname": hostname, "groups": groups})
	resp, err := c.do(http.MethodPost, "/api/fleet/register", bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// Poll claims the next pending run for this agent. Returns nil if there is no
// work.
func (c *Client) Poll(hostname string) (*AgentState, error) {
	resp, err := c.do(http.MethodPost, "/api/fleet/agents/"+url.PathEscape(hostname)+"/poll", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("poll: status %d: %s", resp.StatusCode, string(b))
	}
	var state AgentState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

// UploadOutput appends a chunk of output for a run.
func (c *Client) UploadOutput(hostname, jobID, chunk string) error {
	body, _ := json.Marshal(map[string]string{"chunk": chunk})
	path := fmt.Sprintf("/api/fleet/agents/%s/runs/%s/output", url.PathEscape(hostname), url.PathEscape(jobID))
	resp, err := c.do(http.MethodPost, path, bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload output: status %d", resp.StatusCode)
	}
	return nil
}

// ReportCompletion reports the final result of a run.
func (c *Client) ReportCompletion(hostname, jobID string, success bool, errText string) error {
	body, _ := json.Marshal(map[string]interface{}{"success": success, "error": errText})
	path := fmt.Sprintf("/api/fleet/agents/%s/runs/%s/complete", url.PathEscape(hostname), url.PathEscape(jobID))
	resp, err := c.do(http.MethodPost, path, bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("report completion: status %d", resp.StatusCode)
	}
	return nil
}

// DownloadFile downloads a job file into dst.
func (c *Client) DownloadFile(hostname, jobID, name, dst string) error {
	path := fmt.Sprintf("/api/fleet/agents/%s/runs/%s/files/%s",
		url.PathEscape(hostname), url.PathEscape(jobID), url.PathEscape(name))
	resp, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download file %s: status %d", name, resp.StatusCode)
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, cpErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if cpErr != nil {
		return cpErr
	}
	return closeErr
}
