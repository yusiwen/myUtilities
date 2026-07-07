package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
)

func setupGitOrigin(t *testing.T, dir string) *git.Repository {
	t.Helper()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("init origin: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	wt.Add("README.md")
	if _, err := wt.Commit("initial", &git.CommitOptions{}); err != nil {
		t.Fatalf("initial commit: %v", err)
	}

	return repo
}

func TestGitWatcherRemoteHash(t *testing.T) {
	originDir := t.TempDir()
	localDir := t.TempDir()

	setupGitOrigin(t, originDir)

	_, err := git.PlainClone(localDir, false, &git.CloneOptions{
		URL: originDir,
	})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	localRepo, err := git.PlainOpen(localDir)
	if err != nil {
		t.Fatalf("open local: %v", err)
	}

	gw := NewGitWatcher(localDir, "origin", "master", nil, time.Second)
	gw.repo = localRepo

	// Make a new commit in origin
	originRepo, err := git.PlainOpen(originDir)
	if err != nil {
		t.Fatalf("open origin: %v", err)
	}
	wt, err := originRepo.Worktree()
	if err != nil {
		t.Fatalf("origin worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originDir, "README.md"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	wt.Add("README.md")
	newCommit, err := wt.Commit("update", &git.CommitOptions{})
	if err != nil {
		t.Fatalf("update commit: %v", err)
	}

	// getRemoteHash should now return the new hash
	hash, err := gw.getRemoteHash()
	if err != nil {
		t.Fatalf("getRemoteHash after update: %v", err)
	}
	if hash != newCommit.String() {
		t.Errorf("expected %s, got %s", newCommit.String(), hash)
	}
}

func TestGitWatcherCheckForUpdate(t *testing.T) {
	originDir := t.TempDir()
	localDir := t.TempDir()

	originRepo := setupGitOrigin(t, originDir)

	_, err := git.PlainClone(localDir, false, &git.CloneOptions{
		URL: originDir,
	})
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	localRepo, err := git.PlainOpen(localDir)
	if err != nil {
		t.Fatalf("open local: %v", err)
	}

	gw := NewGitWatcher(localDir, "origin", "master", nil, time.Second)
	gw.repo = localRepo

	// Initialize lastHash to match remote
	hash, err := gw.getRemoteHash()
	if err != nil {
		t.Fatalf("getRemoteHash: %v", err)
	}
	gw.lastHash = hash

	eventCh := make(chan Event, 10)

	// No remote change yet - should not emit event
	gw.checkForUpdate(eventCh)
	select {
	case ev := <-eventCh:
		t.Fatalf("unexpected event: %v", ev)
	default:
	}

	// Make a new commit in origin
	wt, err := originRepo.Worktree()
	if err != nil {
		t.Fatalf("origin worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(originDir, "README.md"), []byte("v3"), 0644); err != nil {
		t.Fatal(err)
	}
	wt.Add("README.md")
	if _, err := wt.Commit("another update", &git.CommitOptions{}); err != nil {
		t.Fatalf("update commit: %v", err)
	}

	// checkForUpdate should detect change
	gw.checkForUpdate(eventCh)
	select {
	case ev := <-eventCh:
		if ev.Type != Modified {
			t.Errorf("expected Modified, got %s", ev.Type)
		}
		if ev.Timestamp.IsZero() {
			t.Errorf("expected non-zero Timestamp")
		}
		if gw.lastHash == hash {
			t.Errorf("lastHash should have been updated")
		}
	case <-time.After(time.Second):
		t.Fatal("expected Modified event")
	}
}

func TestGitWatcherList(t *testing.T) {
	dir := t.TempDir()
	setupGitOrigin(t, dir)

	gw := NewGitWatcher(dir, "", "", nil, time.Second)

	list, err := gw.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}
	if list[0].(string) != dir {
		t.Errorf("expected %s, got %v", dir, list[0])
	}
}
