package fleet

// Agent represents a registered agent node. Online is derived from LastSeen
// at read time and only meaningful on API responses.
type Agent struct {
	Hostname string   `json:"hostname"`
	Groups   []string `json:"groups"`
	LastSeen int64    `json:"last_seen"` // Unix timestamp
	Online   bool     `json:"online"`
}

// Job is a submitted task targeting one or more hosts. Exactly one of
// Command or Recipe is set.
type Job struct {
	ID        string            `json:"id"`
	Command   string            `json:"command,omitempty"`
	Recipe    string            `json:"recipe,omitempty"`
	Vars      map[string]string `json:"vars,omitempty"`
	Targets   []string          `json:"targets"`
	Files     []JobFile         `json:"files,omitempty"`
	CreatedAt int64             `json:"created_at"`
}

// JobFile is a file uploaded with a job.
type JobFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// JobRun is a single execution of a job on one host.
type JobRun struct {
	JobID      string      `json:"job_id"`
	Hostname   string      `json:"hostname"`
	State      JobRunState `json:"state"`
	StartedAt  int64       `json:"started_at"`
	FinishedAt int64       `json:"finished_at"`
	Error      string      `json:"error,omitempty"`
}

// JobRunState represents the state of a job run.
type JobRunState string

const (
	JobRunPending   JobRunState = "pending"
	JobRunRunning   JobRunState = "running"
	JobRunSucceeded JobRunState = "succeeded"
	JobRunFailed    JobRunState = "failed"
)

// AgentState is the run payload returned to an agent by poll.
type AgentState struct {
	JobID   string            `json:"job_id"`
	Command string            `json:"command,omitempty"`
	Recipe  string            `json:"recipe,omitempty"`
	Vars    map[string]string `json:"vars,omitempty"`
	Files   []JobFile         `json:"files,omitempty"`
}

// JobStatusResponse is the shape returned by GET /api/fleet/jobs/{id}.
type JobStatusResponse struct {
	ID        string            `json:"id"`
	Status    JobRunState       `json:"status"`
	Targets   []string          `json:"targets"`
	Runs      []RunStatus       `json:"runs"`
	Outputs   map[string]string `json:"outputs"`
	CreatedAt int64             `json:"created_at"`
}

// RunStatus is the per-host status within a job.
type RunStatus struct {
	Hostname   string      `json:"hostname"`
	State      JobRunState `json:"state"`
	StartedAt  int64       `json:"started_at,omitempty"`
	FinishedAt int64       `json:"finished_at,omitempty"`
	Error      string      `json:"error,omitempty"`
}
