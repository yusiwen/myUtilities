package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

type GitWatcher struct {
	repoPath string
	remote   string
	branch   string
	auth     *http.BasicAuth
	interval time.Duration
	stopChan chan struct{}
	lastHash string
	repo     *git.Repository
}

func NewGitWatcher(repoPath, remote, branch string, auth *http.BasicAuth, interval time.Duration) *GitWatcher {
	if branch == "" {
		branch = "main"
	}
	return &GitWatcher{
		repoPath: repoPath,
		remote:   remote,
		branch:   branch,
		auth:     auth,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (w *GitWatcher) Watch(ctx context.Context) (<-chan Event, error) {
	eventCh := make(chan Event, 10)

	if err := w.initRepo(); err != nil {
		return nil, err
	}

	hash, err := w.getRemoteHash()
	if err != nil {
		return nil, err
	}
	w.lastHash = hash

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		defer close(eventCh)

		for {
			select {
			case <-ticker.C:
				w.checkForUpdate(eventCh)
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventCh, nil
}

func (w *GitWatcher) Stop() {
	close(w.stopChan)
}

func (w *GitWatcher) List() ([]interface{}, error) {
	return []interface{}{w.repoPath}, nil
}

func (w *GitWatcher) initRepo() error {
	repo, err := git.PlainOpen(w.repoPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository at %s: %w", w.repoPath, err)
	}
	w.repo = repo
	return nil
}

func (w *GitWatcher) getRemoteHash() (string, error) {
	if w.repo == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	remoteName := w.remote
	if remoteName == "" {
		remoteName = "origin"
	}

	remote, err := w.repo.Remote(remoteName)
	if err != nil {
		return "", fmt.Errorf("failed to get remote %s: %w", remoteName, err)
	}

	refs, err := remote.List(&git.ListOptions{Auth: w.auth})
	if err != nil {
		return "", fmt.Errorf("failed to list remote %s refs: %w", remoteName, err)
	}

	target := plumbing.NewBranchReferenceName(w.branch)
	for _, ref := range refs {
		if ref.Name() == target {
			return ref.Hash().String(), nil
		}
	}
	return "", fmt.Errorf("branch %s not found on remote %s", w.branch, remoteName)
}

func (w *GitWatcher) checkForUpdate(eventCh chan<- Event) {
	currentHash, err := w.getRemoteHash()
	if err != nil {
		eventCh <- Event{Type: Error, Object: err.Error(), Timestamp: time.Now()}
		return
	}

	if currentHash != w.lastHash {
		if err := w.pullChanges(); err != nil {
			eventCh <- Event{Type: Error, Object: err.Error(), Timestamp: time.Now()}
			return
		}

		w.lastHash = currentHash
		eventCh <- Event{
			Type:      Modified,
			Object:    "Git repository updated",
			Timestamp: time.Now(),
		}
	}
}

func (w *GitWatcher) pullChanges() error {
	if w.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	worktree, err := w.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	options := &git.PullOptions{
		RemoteName:    w.remote,
		ReferenceName: plumbing.NewBranchReferenceName(w.branch),
		Auth:          w.auth,
	}

	if err := worktree.Pull(options); err != nil {
		if err == git.NoErrAlreadyUpToDate {
			return nil
		}
		return fmt.Errorf("failed to pull changes: %w", err)
	}

	return nil
}
