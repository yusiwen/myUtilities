package watch

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/yusiwen/myUtilities/core/watcher"
)

func (o *FileOptions) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	absDir, err := filepath.Abs(o.Dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	fw := watcher.NewFileWatcher(absDir, o.Interval)
	eventCh, err := fw.Watch(ctx)
	if err != nil {
		return fmt.Errorf("start watching: %w", err)
	}

	fmt.Printf("Watching %s for changes (interval: %s)...\n", absDir, o.Interval)

	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				return nil
			}
			if ev.Type == watcher.Error {
				fmt.Fprintf(os.Stderr, "error: %v\n", ev.Object)
				continue
			}
			if !o.matchFilter(ev) {
				continue
			}
			fmt.Printf("[%s] %-8s %v\n",
				ev.Timestamp.Format("2006-01-02 15:04:05"), ev.Type, ev.Object)

		case <-ctx.Done():
			fw.Stop()
			fmt.Println()
			return nil
		}
	}
}

func (o *FileOptions) matchFilter(ev watcher.Event) bool {
	path, ok := ev.Object.(string)
	if !ok {
		return true
	}

	if len(o.Include) > 0 {
		base := filepath.Base(path)
		matched := false
		for _, pattern := range o.Include {
			if m, _ := filepath.Match(pattern, base); m {
				matched = true
				break
			}
			if m, _ := filepath.Match(pattern, path); m {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, pattern := range o.Exclude {
		if m, _ := filepath.Match(pattern, path); m {
			return false
		}
		if m, _ := filepath.Match(pattern, filepath.Base(path)); m {
			return false
		}
	}

	return true
}

func (o *GitOptions) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	absDir, err := filepath.Abs(o.Dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	auth := resolveGitAuth()
	gw := watcher.NewGitWatcher(absDir, o.Remote, o.Branch, auth, o.Interval)
	eventCh, err := gw.Watch(ctx)
	if err != nil {
		return fmt.Errorf("start watching: %w", err)
	}

	branchInfo := o.Branch
	if branchInfo == "" {
		branchInfo = "main"
	}
	fmt.Printf("Watching git remote %s/%s for updates (interval: %s)...\n",
		o.Remote, branchInfo, o.Interval)

	for {
		select {
		case ev, ok := <-eventCh:
			if !ok {
				return nil
			}
			if ev.Type == watcher.Error {
				fmt.Fprintf(os.Stderr, "error: %v\n", ev.Object)
				continue
			}
			fmt.Printf("[%s] %-8s %v\n",
				ev.Timestamp.Format("2006-01-02 15:04:05"), ev.Type, ev.Object)

		case <-ctx.Done():
			gw.Stop()
			fmt.Println()
			return nil
		}
	}
}
