package fleet

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/morikuni/aec"
	corefleet "github.com/yusiwen/myUtilities/internal/core/fleet"
)

var successColor = aec.GreenF
var errColor = aec.RedF

// clientFor builds a dispatcher client from the given options + config.
func (o *CommonOptions) clientFor(cfg *Config) *corefleet.Client {
	server := cfg.Server
	if o.Server != "" {
		server = o.Server
	}
	token := cfg.Token
	if o.Token != "" {
		token = o.Token
	}
	return corefleet.NewClient(server, token)
}

// ServeCmd starts the dispatcher server.
func (c *ServeCmd) Run() error {
	cfg, err := LoadConfig(c.Config)
	if err != nil {
		return err
	}
	cfg = cfg.Resolve()

	port := c.Port
	if port == 0 {
		port = cfg.Port
	}
	if port == 0 {
		port = 8890
	}

	store, err := corefleet.OpenStore(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	dc := &corefleet.DispatcherConfig{
		Token:        cfg.Token,
		DataDir:      cfg.DataDir,
		AgentTimeout: 3 * cfg.PollIntervalDuration(),
	}

	mux := http.NewServeMux()
	corefleet.RegisterHandlers(mux, store, dc)
	fmt.Printf("fleet dispatcher listening on :%d (data: %s)\n", port, cfg.DataDir)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}

// AgentCmd runs the agent loop.
func (c *AgentCmd) Run() error {
	cfg, err := LoadConfig(c.Config)
	if err != nil {
		return err
	}

	hostname := c.Hostname
	if hostname == "" {
		hostname = cfg.Hostname
	}
	groups := cfg.Groups
	if c.Groups != "" {
		groups = strings.Split(c.Groups, ",")
	}
	poll := cfg.PollIntervalDuration()
	if c.PollInterval > 0 {
		poll = time.Duration(c.PollInterval) * time.Second
	}

	return corefleet.RunAgent(corefleet.AgentConfig{
		ServerURL:    cfg.Server,
		Token:        cfg.Token,
		Hostname:     hostname,
		Groups:       groups,
		PollInterval: poll,
	})
}

// RunCmd submits a job and optionally watches it.
func (c *RunCmd) Run() error {
	cfg, err := LoadConfig(c.Config)
	if err != nil {
		return err
	}

	if c.File != "" && c.Command != "" {
		return errors.New("--file and --command are mutually exclusive")
	}
	if c.File == "" && c.Command == "" {
		return errors.New("require either --file or --command")
	}

	client := c.clientFor(cfg)

	vars := map[string]string{}
	for _, kv := range c.Var {
		k, v, found := strings.Cut(kv, "=")
		if !found || k == "" {
			return fmt.Errorf("invalid --var %q (expected key=value)", kv)
		}
		vars[k] = v
	}

	req := corefleet.SubmitJobRequest{
		Targets: strings.Split(c.Hosts, ","),
		Vars:    vars,
		Files:   c.Files,
	}
	if c.File != "" {
		data, err := os.ReadFile(c.File)
		if err != nil {
			return fmt.Errorf("read recipe: %w", err)
		}
		req.Recipe = string(data)
	} else {
		req.Command = c.Command
	}

	jobID, err := client.SubmitJob(req)
	if err != nil {
		return err
	}
	fmt.Printf("submitted job %s for hosts: %s\n", jobID, c.Hosts)

	if !c.Watch {
		st, err := client.JobStatus(jobID)
		if err != nil {
			return err
		}
		return printJobStatus(st, false)
	}
	return watchJob(client, jobID)
}

func watchJob(client *corefleet.Client, jobID string) error {
	prev := map[string]int{}
	for {
		st, err := client.JobStatus(jobID)
		if err != nil {
			return err
		}
		for host, out := range st.Outputs {
			if n := len(out); n > prev[host] {
				fmt.Print(out[prev[host]:])
				prev[host] = n
			}
		}

		done := len(st.Runs) > 0
		for _, run := range st.Runs {
			if run.State == corefleet.JobRunPending || run.State == corefleet.JobRunRunning {
				done = false
				break
			}
		}
		if done {
			return printJobStatus(st, true)
		}
		time.Sleep(time.Second)
	}
}

func printJobStatus(st *corefleet.JobStatusResponse, summary bool) error {
	if !summary {
		for _, run := range st.Runs {
			fmt.Printf("  %-16s %s\n", run.Hostname, run.State)
		}
		if st.Status == corefleet.JobRunFailed {
			return errors.New("job failed")
		}
		return nil
	}

	fmt.Println()
	fmt.Println("Fleet summary:")
	failed := false
	for _, run := range st.Runs {
		mark := "✓"
		color := successColor
		if run.State == corefleet.JobRunFailed {
			mark = "✗"
			color = errColor
			failed = true
		}
		fmt.Printf("  %s %-16s %s\n", aec.Apply(mark, color), run.Hostname, run.State)
		if run.Error != "" {
			fmt.Printf("    error: %s\n", run.Error)
		}
	}
	if failed {
		return errors.New("one or more hosts failed")
	}
	return nil
}

// HostsCmd lists agents.
func (c *HostsCmd) Run() error {
	cfg, err := LoadConfig(c.Config)
	if err != nil {
		return err
	}
	agents, err := c.clientFor(cfg).ListAgents()
	if err != nil {
		return err
	}
	if len(agents) == 0 {
		fmt.Println("no agents registered")
		return nil
	}
	for _, a := range agents {
		state := "offline"
		if a.Online {
			state = "online "
		}
		fmt.Printf("  %s %-16s groups=%v last_seen=%d\n", state, a.Hostname, a.Groups, a.LastSeen)
	}
	return nil
}

// StatusCmd shows job status.
func (c *StatusCmd) Run() error {
	cfg, err := LoadConfig(c.Config)
	if err != nil {
		return err
	}
	client := c.clientFor(cfg)

	st, err := client.JobStatus(c.JobID)
	if err != nil {
		return err
	}
	if !c.Watch {
		fmt.Printf("job %s status: %s\n", st.ID, st.Status)
		return printJobStatus(st, false)
	}
	return watchJob(client, c.JobID)
}

// JobsCmd lists recent jobs.
func (c *JobsCmd) Run() error {
	cfg, err := LoadConfig(c.Config)
	if err != nil {
		return err
	}
	jobs, err := c.clientFor(cfg).ListJobs()
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		fmt.Println("no jobs")
		return nil
	}
	for _, j := range jobs {
		what := j.Command
		if what == "" {
			what = "<recipe>"
		}
		fmt.Printf("  %-10s %s  targets=%v  %s\n", j.ID, time.Unix(j.CreatedAt, 0).Format("2006-01-02 15:04:05"), j.Targets, what)
	}
	return nil
}
