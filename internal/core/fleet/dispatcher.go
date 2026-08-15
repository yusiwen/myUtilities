package fleet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// DispatcherConfig configures the fleet dispatcher server.
type DispatcherConfig struct {
	Token        string        // shared auth token; empty disables auth
	DataDir      string        // job file storage root (e.g. ~/.cache/mu/fleet)
	AgentTimeout time.Duration // heartbeat timeout before an agent is offline
}

// jobID returns a short, readable job identifier.
func jobID() string {
	return uuid.NewString()[:8]
}

// RegisterHandlers registers the fleet API routes on the given mux, each
// guarded by the shared token.
func RegisterHandlers(mux *http.ServeMux, store *Store, cfg *DispatcherConfig) {
	auth := func(h http.HandlerFunc) http.HandlerFunc { return requireToken(cfg.Token, h) }

	mux.HandleFunc("/api/fleet/jobs", auth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleCreateJob(w, r, store, cfg)
		case http.MethodGet:
			handleListJobs(w, r, store)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}))
	mux.HandleFunc("/api/fleet/jobs/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
			return
		}
		handleGetJob(w, r, store)
	}))
	mux.HandleFunc("/api/fleet/register", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		handleRegister(w, r, store)
	}))
	mux.HandleFunc("/api/fleet/agents/{name}/poll", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		handlePoll(w, r, store)
	}))
	mux.HandleFunc("/api/fleet/agents/{name}/runs/{id}/output", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		handleOutput(w, r, store)
	}))
	mux.HandleFunc("/api/fleet/agents/{name}/runs/{id}/complete", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		handleComplete(w, r, store)
	}))
	mux.HandleFunc("/api/fleet/agents/{name}/runs/{id}/files/{file}", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
			return
		}
		handleDownloadFile(w, r, cfg)
	}))
	mux.HandleFunc("/api/fleet/agents", auth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
			return
		}
		handleListAgents(w, r, store, cfg)
	}))
}

func handleCreateJob(w http.ResponseWriter, r *http.Request, store *Store, cfg *DispatcherConfig) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	raw := r.FormValue("job")
	if raw == "" {
		http.Error(w, `{"error":"missing job field"}`, http.StatusBadRequest)
		return
	}
	var job Job
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	if job.Command == "" && job.Recipe == "" {
		http.Error(w, `{"error":"job requires command or recipe"}`, http.StatusBadRequest)
		return
	}
	if len(job.Targets) == 0 {
		http.Error(w, `{"error":"job requires targets"}`, http.StatusBadRequest)
		return
	}
	job.ID = jobID()
	job.CreatedAt = time.Now().Unix()
	job.Files = nil

	// Store uploaded files and record their sizes and checksums.
	if files := r.MultipartForm.File["file"]; len(files) > 0 {
		filesDir := filepath.Join(cfg.DataDir, "jobs", job.ID, "files")
		if err := os.MkdirAll(filesDir, 0755); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		for _, fh := range files {
			src, err := fh.Open()
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
				return
			}
			dstPath := filepath.Join(filesDir, filepath.Base(fh.Filename))
			dst, err := os.Create(dstPath)
			if err != nil {
				src.Close()
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
				return
			}
			size, cpErr := io.Copy(dst, src)
			src.Close()
			closeErr := dst.Close()
			if cpErr != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, cpErr), http.StatusInternalServerError)
				return
			}
			if closeErr != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, closeErr), http.StatusInternalServerError)
				return
			}
			sum, err := ComputeSHA256(dstPath)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
				return
			}
			job.Files = append(job.Files, JobFile{Name: fh.Filename, Size: size, SHA256: sum})
		}
	}

	if err := store.PutJob(&job); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	for _, target := range job.Targets {
		if err := store.CreatePendingRun(job.ID, target); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": job.ID})
}

func handleListJobs(w http.ResponseWriter, r *http.Request, store *Store) {
	jobs, err := store.ListJobs(0)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"jobs": jobs})
}

func handleGetJob(w http.ResponseWriter, r *http.Request, store *Store) {
	id := r.PathValue("id")
	job, err := store.GetJob(id)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	if job == nil {
		http.Error(w, `{"error":"job not found"}`, http.StatusNotFound)
		return
	}
	status, err := store.GetJobStatus(id)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	runs, err := store.ListJobRuns(id)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	outputs, err := store.GetJobOutput(id)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}

	var runStatuses []RunStatus
	for _, run := range runs {
		runStatuses = append(runStatuses, RunStatus{
			Hostname:   run.Hostname,
			State:      run.State,
			StartedAt:  run.StartedAt,
			FinishedAt: run.FinishedAt,
			Error:      run.Error,
		})
	}

	resp := JobStatusResponse{
		ID:        job.ID,
		Status:    status,
		Targets:   job.Targets,
		Runs:      runStatuses,
		Outputs:   outputs,
		CreatedAt: job.CreatedAt,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleRegister(w http.ResponseWriter, r *http.Request, store *Store) {
	var req struct {
		Hostname string   `json:"hostname"`
		Groups   []string `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	if req.Hostname == "" {
		http.Error(w, `{"error":"hostname required"}`, http.StatusBadRequest)
		return
	}
	agent := &Agent{Hostname: req.Hostname, Groups: req.Groups, LastSeen: time.Now().Unix()}
	if err := store.PutAgent(agent); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"hostname": req.Hostname})
}

func handlePoll(w http.ResponseWriter, r *http.Request, store *Store) {
	hostname := r.PathValue("name")
	_ = store.UpdateAgentLastSeen(hostname)

	job, err := store.ClaimNextRun(hostname)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	state := AgentState{
		JobID:   job.ID,
		Command: job.Command,
		Recipe:  job.Recipe,
		Vars:    job.Vars,
		Files:   job.Files,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func handleOutput(w http.ResponseWriter, r *http.Request, store *Store) {
	var req struct {
		Chunk string `json:"chunk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	jobID := r.PathValue("id")
	hostname := r.PathValue("name")
	if err := store.AppendRunOutput(jobID, hostname, req.Chunk); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleComplete(w http.ResponseWriter, r *http.Request, store *Store) {
	var req struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	jobID := r.PathValue("id")
	hostname := r.PathValue("name")
	run, err := store.GetJobRun(jobID, hostname)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	if run == nil {
		http.Error(w, `{"error":"run not found"}`, http.StatusNotFound)
		return
	}
	run.FinishedAt = time.Now().Unix()
	if req.Success {
		run.State = JobRunSucceeded
	} else {
		run.State = JobRunFailed
		run.Error = req.Error
	}
	if err := store.PutJobRun(run); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleDownloadFile(w http.ResponseWriter, r *http.Request, cfg *DispatcherConfig) {
	jobID := r.PathValue("id")
	file := r.PathValue("file")
	path := filepath.Join(cfg.DataDir, "jobs", jobID, "files", filepath.Base(file))
	if _, err := os.Stat(path); err != nil {
		http.Error(w, `{"error":"file not found"}`, http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, path)
}

func handleListAgents(w http.ResponseWriter, r *http.Request, store *Store, cfg *DispatcherConfig) {
	agents, err := store.ListAgents()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
		return
	}
	now := time.Now()
	type agentView struct {
		Agent
		Online bool `json:"online"`
	}
	views := make([]agentView, 0, len(agents))
	for _, a := range agents {
		views = append(views, agentView{
			Agent:  *a,
			Online: now.Sub(time.Unix(a.LastSeen, 0)) <= cfg.AgentTimeout,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"agents": views})
}
