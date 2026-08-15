package fleet

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	corerunner "github.com/yusiwen/myUtilities/internal/core/runner"
)

const agentMaxRetries = 5

// AgentConfig configures a fleet agent.
type AgentConfig struct {
	ServerURL    string
	Token        string
	Hostname     string
	Groups       []string
	PollInterval time.Duration
}

// RunAgent registers with the dispatcher and then polls for work, executing
// claimed jobs with the core runner and streaming output back.
func RunAgent(cfg AgentConfig) error {
	return RunAgentContext(context.Background(), cfg)
}

// RunAgentContext is RunAgent with a context that stops the polling loop.
func RunAgentContext(ctx context.Context, cfg AgentConfig) error {
	if cfg.Hostname == "" {
		host, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("agent: resolve hostname: %w", err)
		}
		cfg.Hostname = host
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}

	client := NewClient(cfg.ServerURL, cfg.Token)
	if err := registerWithRetry(client, cfg.Hostname, cfg.Groups); err != nil {
		return err
	}
	log.Printf("fleet agent %q registered with %s", cfg.Hostname, cfg.ServerURL)

	for {
		select {
		case <-ctx.Done():
			log.Printf("fleet agent %q stopping", cfg.Hostname)
			return nil
		default:
		}

		state, err := client.Poll(cfg.Hostname)
		if err != nil {
			log.Printf("agent: poll failed: %v", err)
			time.Sleep(cfg.PollInterval)
			continue
		}
		if state == nil {
			time.Sleep(cfg.PollInterval)
			continue
		}

		runErr := executeRun(client, cfg.Hostname, state)
		errText := ""
		if runErr != nil {
			errText = runErr.Error()
		}
		if cerr := client.ReportCompletion(cfg.Hostname, state.JobID, runErr == nil, errText); cerr != nil {
			log.Printf("agent: report completion for job %s failed: %v", state.JobID, cerr)
		}
	}
}

func registerWithRetry(client *Client, hostname string, groups []string) error {
	for i := range agentMaxRetries {
		if err := client.Register(hostname, groups); err == nil {
			return nil
		} else if i < agentMaxRetries-1 {
			wait := time.Duration(i+1) * time.Second
			log.Printf("agent: register attempt %d failed, retrying in %v: %v", i+1, wait, err)
			time.Sleep(wait)
		} else {
			return fmt.Errorf("agent: failed to register after %d retries: %v", agentMaxRetries, err)
		}
	}
	return nil
}

// executeRun runs a claimed job: downloads files into a staging dir, verifies
// checksums, extracts archives, then executes the command or recipe with the
// core runner while streaming output to the dispatcher.
func executeRun(client *Client, hostname string, state *AgentState) error {
	workdir, err := os.MkdirTemp("", "fleet-"+state.JobID+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workdir)

	for _, f := range state.Files {
		dst := filepath.Join(workdir, filepath.Base(f.Name))
		if err := client.DownloadFile(hostname, state.JobID, f.Name, dst); err != nil {
			return fmt.Errorf("download %s: %w", f.Name, err)
		}
		if f.SHA256 != "" {
			sum, err := ComputeSHA256(dst)
			if err != nil {
				return fmt.Errorf("checksum %s: %w", f.Name, err)
			}
			if sum != f.SHA256 {
				return fmt.Errorf("checksum mismatch for %s: expected %s got %s", f.Name, f.SHA256, sum)
			}
		}
		if ArchiveExt(f.Name) != "" {
			if err := ExtractArchive(dst, workdir); err != nil {
				return fmt.Errorf("extract %s: %w", f.Name, err)
			}
		}
	}

	streamer := newOutputStreamer(client, hostname, state.JobID)
	defer streamer.Close()

	if state.Recipe != "" {
		recipe, err := corerunner.ParseRecipe([]byte(state.Recipe))
		if err != nil {
			return err
		}
		r := corerunner.NewCommandRunner(nil)
		r.OutputWriter = streamer
		_, err = r.RunRecipe(recipe, corerunner.RecipeRunOptions{Vars: state.Vars, Workdir: workdir})
		return err
	}

	if state.Command != "" {
		r := corerunner.NewCommandRunner([]corerunner.Command{{
			Name:    state.JobID,
			CmdLine: state.Command,
			Dir:     workdir,
		}})
		r.OutputWriter = streamer
		return r.Run()
	}

	return fmt.Errorf("job %s has no command or recipe", state.JobID)
}

// outputStreamer batches runner output and flushes it to the dispatcher in
// chunks (~100ms), preserving order.
type outputStreamer struct {
	client   *Client
	hostname string
	jobID    string

	mu    sync.Mutex
	buf   bytes.Buffer
	timer *time.Timer
	err   error
}

func newOutputStreamer(client *Client, hostname, jobID string) *outputStreamer {
	return &outputStreamer{client: client, hostname: hostname, jobID: jobID}
}

func (s *outputStreamer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, s.err
	}
	s.buf.Write(p)
	if s.timer == nil {
		s.timer = time.AfterFunc(100*time.Millisecond, s.flush)
	}
	return len(p), nil
}

// flush sends any buffered output to the dispatcher. It is called on a timer
// and on Close; the mutex guarantees chunks are delivered in order.
func (s *outputStreamer) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.buf.Len() == 0 {
		return
	}
	chunk := s.buf.String()
	s.buf.Reset()
	if err := s.client.UploadOutput(s.hostname, s.jobID, chunk); err != nil {
		s.err = err
		log.Printf("agent: upload output for job %s failed: %v", s.jobID, err)
	}
}

// Close flushes any remaining buffered output.
func (s *outputStreamer) Close() {
	s.flush()
}
