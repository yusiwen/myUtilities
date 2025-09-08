package watcher

import (
	"context"
	"fmt"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// GitWatcher 监控Git仓库变化
type GitWatcher struct {
	repoPath string
	remote   string
	branch   string
	auth     *BasicAuth
	interval time.Duration
	stopChan chan struct{}
	lastHash string
	repo     *git.Repository
}

type BasicAuth struct {
	Username string
	Password string
}

func NewGitWatcher(repoPath, remote, branch string, auth *BasicAuth, interval time.Duration) *GitWatcher {
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

	// 初始化仓库
	if err := w.initRepo(); err != nil {
		return nil, err
	}

	// 获取初始状态
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
	// 获取当前文件列表
	return []interface{}{}, nil
}

func (w *GitWatcher) initRepo() error {
	// 打开仓库，如果不是有效的Git仓库则报错
	repo, err := git.PlainOpen(w.repoPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository at %s: %w", w.repoPath, err)
	}

	// 保存仓库引用
	w.repo = repo
	return nil
}

func (w *GitWatcher) getRemoteHash() (string, error) {
	// 获取远程分支哈希
	if w.repo == nil {
		return "", fmt.Errorf("repository not initialized")
	}

	// 获取远程引用
	remoteName := w.remote
	if remoteName == "" {
		remoteName = "origin"
	}

	// 构建远程分支引用名称
	refName := plumbing.NewRemoteReferenceName(remoteName, w.branch)

	// 获取引用
	ref, err := w.repo.Reference(refName, true)
	if err != nil {
		return "", fmt.Errorf("failed to get reference for %s: %w", refName, err)
	}

	// 返回提交哈希
	return ref.Hash().String(), nil
}

func (w *GitWatcher) checkForUpdate(eventCh chan<- Event) {
	currentHash, err := w.getRemoteHash()
	if err != nil {
		eventCh <- Event{Type: Error, Object: err.Error()}
		return
	}

	if currentHash != w.lastHash {
		// 拉取更新
		if err := w.pullChanges(); err != nil {
			eventCh <- Event{Type: Error, Object: err.Error()}
			return
		}

		// 发送更新事件
		eventCh <- Event{
			Type:   Modified,
			Object: "Git repository updated",
		}

		w.lastHash = currentHash
	}
}

func (w *GitWatcher) pullChanges() error {
	// 实现拉取更新的逻辑
	if w.repo == nil {
		return fmt.Errorf("repository not initialized")
	}

	// 获取工作区
	worktree, err := w.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// 设置拉取选项
	options := &git.PullOptions{
		RemoteName:    w.remote,
		ReferenceName: plumbing.NewBranchReferenceName(w.branch),
	}

	// 如果提供了认证信息，则设置认证
	if w.auth != nil && w.auth.Username != "" {
		options.Auth = &http.BasicAuth{
			Username: w.auth.Username,
			Password: w.auth.Password,
		}
	}

	// 拉取更新
	if err := worktree.Pull(options); err != nil {
		if err == git.NoErrAlreadyUpToDate {
			// 已经是最新的，不是错误
			return nil
		}
		return fmt.Errorf("failed to pull changes: %w", err)
	}

	return nil
}
