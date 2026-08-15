package fleet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// maxRunOutput caps the stored output per run; older bytes are dropped.
const maxRunOutput = 1024 * 1024

// Store wraps BoltDB operations for the fleet module.
type Store struct {
	mtx *sync.Mutex
	db  *bolt.DB
}

// OpenStore opens or creates a BoltDB database at the given path.
func OpenStore(dbPath string) (*Store, error) {
	if strings.HasPrefix(dbPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %v", err)
		}
		dbPath = filepath.Join(home, dbPath[2:])
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	db, err := bolt.Open(dbPath, 0660, nil)
	if err != nil {
		return nil, err
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{"agents", "jobs", "runs", "run_output"} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{mtx: &sync.Mutex{}, db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.db.Close()
}

// GetAgent retrieves an agent by hostname, or nil if not found.
func (s *Store) GetAgent(hostname string) (*Agent, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var agent *Agent
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("agents")).Get([]byte(hostname))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &agent)
	})
	return agent, err
}

// PutAgent stores or updates an agent.
func (s *Store) PutAgent(agent *Agent) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Batch(func(tx *bolt.Tx) error {
		data, err := json.Marshal(agent)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("agents")).Put([]byte(agent.Hostname), data)
	})
}

// ListAgents lists all registered agents.
func (s *Store) ListAgents() ([]*Agent, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var agents []*Agent
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("agents")).ForEach(func(k, v []byte) error {
			var agent Agent
			if err := json.Unmarshal(v, &agent); err != nil {
				return err
			}
			agents = append(agents, &agent)
			return nil
		})
	})
	sort.Slice(agents, func(i, j int) bool { return agents[i].Hostname < agents[j].Hostname })
	return agents, err
}

// UpdateAgentLastSeen bumps an agent's heartbeat timestamp.
func (s *Store) UpdateAgentLastSeen(hostname string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Batch(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("agents"))
		data := bucket.Get([]byte(hostname))
		if data == nil {
			return nil // not registered
		}
		var agent Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			return err
		}
		agent.LastSeen = time.Now().Unix()
		newData, err := json.Marshal(agent)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(hostname), newData)
	})
}

// GetJob retrieves a job by ID, or nil if not found.
func (s *Store) GetJob(id string) (*Job, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var job *Job
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("jobs")).Get([]byte(id))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &job)
	})
	return job, err
}

// PutJob stores a job.
func (s *Store) PutJob(job *Job) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Batch(func(tx *bolt.Tx) error {
		data, err := json.Marshal(job)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("jobs")).Put([]byte(job.ID), data)
	})
}

// ListJobs returns the most recent jobs, newest first, up to limit.
func (s *Store) ListJobs(limit int) ([]*Job, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var jobs []*Job
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("jobs")).ForEach(func(k, v []byte) error {
			var job Job
			if err := json.Unmarshal(v, &job); err != nil {
				return err
			}
			jobs = append(jobs, &job)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt > jobs[j].CreatedAt })
	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs, nil
}

func runKey(jobID, hostname string) []byte {
	return []byte(jobID + "/" + hostname)
}

// GetJobRun retrieves a job run, or nil if not found.
func (s *Store) GetJobRun(jobID, hostname string) (*JobRun, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var run *JobRun
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("runs")).Get(runKey(jobID, hostname))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &run)
	})
	return run, err
}

// PutJobRun stores a job run.
func (s *Store) PutJobRun(run *JobRun) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Batch(func(tx *bolt.Tx) error {
		data, err := json.Marshal(run)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("runs")).Put(runKey(run.JobID, run.Hostname), data)
	})
}

// CreatePendingRun creates a pending run for a host if one does not already
// exist.
func (s *Store) CreatePendingRun(jobID, hostname string) error {
	run, err := s.GetJobRun(jobID, hostname)
	if err != nil {
		return err
	}
	if run != nil {
		return nil
	}
	return s.PutJobRun(&JobRun{
		JobID:     jobID,
		Hostname:  hostname,
		State:     JobRunPending,
		StartedAt: time.Now().Unix(),
	})
}

// ClaimNextRun finds the next pending run for a hostname, marks it running,
// and returns the job it belongs to (or nil if there is no pending work).
func (s *Store) ClaimNextRun(hostname string) (*Job, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var job *Job
	err := s.db.Update(func(tx *bolt.Tx) error {
		runs := tx.Bucket([]byte("runs"))
		return runs.ForEach(func(k, v []byte) error {
			if job != nil {
				return nil // already claimed
			}
			var run JobRun
			if err := json.Unmarshal(v, &run); err != nil {
				return err
			}
			if run.Hostname != hostname || run.State != JobRunPending {
				return nil
			}
			run.State = JobRunRunning
			run.StartedAt = time.Now().Unix()
			newData, err := json.Marshal(run)
			if err != nil {
				return err
			}
			if err := runs.Put(k, newData); err != nil {
				return err
			}
			// Load the owning job.
			data := tx.Bucket([]byte("jobs")).Get([]byte(run.JobID))
			if data == nil {
				return nil
			}
			var j Job
			if err := json.Unmarshal(data, &j); err != nil {
				return err
			}
			job = &j
			return nil
		})
	})
	return job, err
}

// ListJobRuns lists all runs for a job.
func (s *Store) ListJobRuns(jobID string) ([]*JobRun, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var runs []*JobRun
	err := s.db.View(func(tx *bolt.Tx) error {
		prefix := []byte(jobID + "/")
		return tx.Bucket([]byte("runs")).ForEach(func(k, v []byte) error {
			if !strings.HasPrefix(string(k), string(prefix)) {
				return nil
			}
			var run JobRun
			if err := json.Unmarshal(v, &run); err != nil {
				return err
			}
			runs = append(runs, &run)
			return nil
		})
	})
	sort.Slice(runs, func(i, j int) bool { return runs[i].Hostname < runs[j].Hostname })
	return runs, err
}

// GetJobRunOutput retrieves the accumulated output for a run.
func (s *Store) GetJobRunOutput(jobID, hostname string) (string, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var output string
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("run_output")).Get(runKey(jobID, hostname))
		if data != nil {
			output = string(data)
		}
		return nil
	})
	return output, err
}

// AppendRunOutput appends a chunk of output, keeping only the trailing
// maxRunOutput bytes.
func (s *Store) AppendRunOutput(jobID, hostname, chunk string) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.db.Batch(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("run_output"))
		key := runKey(jobID, hostname)
		combined := string(bucket.Get(key)) + chunk
		if len(combined) > maxRunOutput {
			combined = "[truncated]\n" + combined[len(combined)-maxRunOutput:]
		}
		return bucket.Put(key, []byte(combined))
	})
}

// GetJobStatus derives the job status from its runs.
func (s *Store) GetJobStatus(jobID string) (JobRunState, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	runs, err := s.listJobRunsLocked(jobID)
	if err != nil {
		return JobRunPending, err
	}
	if len(runs) == 0 {
		return JobRunPending, nil
	}
	allSucceeded := true
	for _, run := range runs {
		if run.State == JobRunFailed {
			return JobRunFailed, nil
		}
		if run.State != JobRunSucceeded {
			allSucceeded = false
		}
	}
	if allSucceeded {
		return JobRunSucceeded, nil
	}
	for _, run := range runs {
		if run.State == JobRunRunning || run.State == JobRunPending {
			return JobRunRunning, nil
		}
	}
	return JobRunPending, nil
}

// GetJobOutput returns per-host outputs for a job.
func (s *Store) GetJobOutput(jobID string) (map[string]string, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	outputs := make(map[string]string)
	err := s.db.View(func(tx *bolt.Tx) error {
		prefix := []byte(jobID + "/")
		return tx.Bucket([]byte("run_output")).ForEach(func(k, v []byte) error {
			if !strings.HasPrefix(string(k), string(prefix)) {
				return nil
			}
			host := string(k[len(jobID)+1:])
			outputs[host] = string(v)
			return nil
		})
	})
	return outputs, err
}

func (s *Store) listJobRunsLocked(jobID string) ([]*JobRun, error) {
	var runs []*JobRun
	prefix := []byte(jobID + "/")
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("runs")).ForEach(func(k, v []byte) error {
			if !strings.HasPrefix(string(k), string(prefix)) {
				return nil
			}
			var run JobRun
			if err := json.Unmarshal(v, &run); err != nil {
				return err
			}
			runs = append(runs, &run)
			return nil
		})
	})
	return runs, err
}
