package log

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"time"

	"github.com/morikuni/aec"
	"github.com/yusiwen/myUtilities/internal/core/logtail"
)

var noColor bool

func init() {
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
}

func colored(s string, style aec.ANSI) string {
	if noColor {
		return s
	}
	return aec.Apply(s, style)
}

// Options holds the CLI flags for `mu log`.
type Options struct {
	Files   []string `arg:"" name:"file" help:"Log file(s) to tail." required:""`
	Follow  bool     `short:"f" help:"Continually watch and print new log lines."`
	Level   string   `short:"l" name:"level" help:"Only show log entries at or above this level (default shows all)." enum:"debug,info,warn,error,fatal" default:"debug"`
	Since   string   `short:"s" name:"since" help:"Only show entries newer than this duration (e.g. 5m, 1h, 2h30m)."`
	Grep    string   `short:"g" name:"grep" help:"Only show lines matching this regex."`
	Num     int      `short:"n" name:"lines" help:"Number of lines to show initially (0 = all lines)." default:"100"`
	NoColor bool     `short:"C" help:"Disable colored output."`
}

// Run executes the log tailer.
func (o *Options) Run() error {
	if o.NoColor {
		noColor = true
	}

	// Parse duration for --since.
	var sinceDur time.Duration
	if o.Since != "" {
		var err error
		sinceDur, err = time.ParseDuration(o.Since)
		if err != nil {
			return fmt.Errorf("invalid --since value %q: %w", o.Since, err)
		}
	}

	// Parse grep regex.
	var grepRe *regexp.Regexp
	if o.Grep != "" {
		var err error
		grepRe, err = regexp.Compile(o.Grep)
		if err != nil {
			return fmt.Errorf("invalid --grep regex %q: %w", o.Grep, err)
		}
	}

	// Parse minimum level.
	var minLevel logtail.Level
	switch o.Level {
	case "":
		minLevel = logtail.LevelUnknown // no filter
	case "debug":
		minLevel = logtail.LevelDebug
	case "info":
		minLevel = logtail.LevelInfo
	case "warn":
		minLevel = logtail.LevelWarn
	case "error":
		minLevel = logtail.LevelError
	case "fatal":
		minLevel = logtail.LevelFatal
	}

	// Build filter options.
	filterOpts := logtail.FilterOptions{
		MinLevel: minLevel,
		Since:    sinceDur,
		Grep:     grepRe,
	}

	// Create the tailer.
	t := logtail.NewTailer(o.Files, 500*time.Millisecond)

	// Read initial lines.
	lines, err := t.ReadInitial(o.Num)
	if err != nil {
		return fmt.Errorf("reading initial lines: %w", err)
	}

	// Apply filters and print.
	filtered := logtail.FilterLines(lines, filterOpts)
	for _, line := range filtered {
		printLine(line)
	}

	if !o.Follow {
		return nil
	}

	// Follow mode: set up context with signal handling.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Channel for new lines.
	ch := make(chan string, 100)

	// Start the tailer in a goroutine.
	go t.Follow(ctx, ch)

	// Print a separator to indicate follow mode.
	if len(o.Files) > 1 {
		fmt.Fprintln(os.Stderr, colored(fmt.Sprintf("--- watching %d files (Ctrl-C to stop) ---", len(o.Files)), aec.Faint))
	} else {
		fmt.Fprintln(os.Stderr, colored("--- watching (Ctrl-C to stop) ---", aec.Faint))
	}

	// Read from the channel and print new lines.
	for raw := range ch {
		filtered := logtail.FilterLines([]string{raw}, filterOpts)
		for _, line := range filtered {
			printLine(line)
		}
	}

	return nil
}

// printLine colors and prints a single log line based on its detected level.
func printLine(raw string) {
	parsed := logtail.ParseLine(raw)

	// Color the entire line based on detected level.
	switch parsed.Level {
	case logtail.LevelDebug:
		fmt.Println(colored(raw, aec.Faint))
	case logtail.LevelInfo:
		fmt.Println(colored(raw, aec.GreenF))
	case logtail.LevelWarn:
		fmt.Println(colored(raw, aec.YellowF))
	case logtail.LevelError:
		fmt.Println(colored(raw, aec.RedF))
	case logtail.LevelFatal:
		fmt.Println(colored(colored(raw, aec.RedF), aec.Bold))
	default:
		fmt.Println(raw)
	}
}
