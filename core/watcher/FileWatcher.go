package watcher

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileWatcher 监控本地文件变化
type FileWatcher struct {
	path      string
	interval  time.Duration
	stopChan  chan struct{}
	lastState map[string]FileState // 文件路径 -> 状态
}

type FileState struct {
	Size     int64
	ModTime  time.Time
	Checksum string // 简化的校验和
}

func NewFileWatcher(path string, interval time.Duration) *FileWatcher {
	return &FileWatcher{
		path:     path,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

func (w *FileWatcher) Watch(ctx context.Context) (<-chan Event, error) {
	eventCh := make(chan Event, 10)

	// 初始化状态
	if err := w.scanFiles(); err != nil {
		return nil, err
	}

	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		defer close(eventCh)

		for {
			select {
			case <-ticker.C:
				w.detectChanges(eventCh)
			case <-w.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return eventCh, nil
}

func (w *FileWatcher) Stop() {
	close(w.stopChan)
}

func (w *FileWatcher) List() ([]interface{}, error) {
	// 实际实现需要扫描文件系统
	return []interface{}{}, nil
}

// getFileState 获取单个文件的状态
func getFileState(filePath string, fileInfo os.FileInfo) (FileState, error) {
	checksum, err := calculateChecksum(filePath)
	if err != nil {
		return FileState{}, fmt.Errorf("failed to calculate checksum for %s: %w", filePath, err)
	}

	return FileState{
		Size:     fileInfo.Size(),
		ModTime:  fileInfo.ModTime(),
		Checksum: checksum,
	}, nil
}

// scanPath 扫描路径（文件或目录）并返回文件状态映射
func scanPath(path string) (map[string]FileState, error) {
	stateMap := make(map[string]FileState)

	// 检查路径是否存在
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access path %s: %w", path, err)
	}

	// 如果是单个文件，直接处理
	if !fileInfo.IsDir() {
		state, err := getFileState(path, fileInfo)
		if err != nil {
			return nil, err
		}
		stateMap[path] = state
		return stateMap, nil
	}

	// 遍历目录
	err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if info.IsDir() {
			return nil
		}

		state, err := getFileState(filePath, info)
		if err != nil {
			return err
		}

		stateMap[filePath] = state
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory %s: %w", path, err)
	}

	return stateMap, nil
}

// handleError 处理错误，可选择发送到事件通道
func handleError(err error, message string, eventCh chan<- Event) error {
	formattedErr := fmt.Errorf("%s: %w", message, err)

	// 如果提供了事件通道，发送错误事件
	if eventCh != nil {
		eventCh <- Event{
			Type:      Error,
			Object:    formattedErr.Error(),
			Timestamp: time.Now(),
		}
		return nil
	}

	// 否则返回错误
	return formattedErr
}

func (w *FileWatcher) scanFiles() error {
	stateMap, err := scanPath(w.path)
	if err != nil {
		return err
	}

	w.lastState = stateMap
	return nil
}

// calculateChecksum 计算文件的MD5校验和
// 为了效率，只读取文件的前8KB来计算校验和，这在大多数情况下足够检测文件变化
// 返回十六进制编码的MD5哈希值字符串
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 只读取前8KB来计算校验和
	buffer := make([]byte, 8*1024)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	hash := md5.Sum(buffer[:n])
	return hex.EncodeToString(hash[:]), nil
}

// compareStates 比较两个状态映射并发送相应的事件
func compareStates(currentState, lastState map[string]FileState, eventCh chan<- Event) {
	// 检测新增和修改的文件
	for path, state := range currentState {
		oldState, exists := lastState[path]
		if !exists {
			// 新增文件
			eventCh <- Event{
				Type:      Added,
				Object:    path,
				Timestamp: time.Now(),
			}
		} else if oldState.Size != state.Size ||
			oldState.ModTime != state.ModTime ||
			oldState.Checksum != state.Checksum {
			// 修改文件 - 明确比较各个字段
			eventCh <- Event{
				Type:      Modified,
				Object:    path,
				Timestamp: time.Now(),
			}
		}
	}

	// 检测删除的文件
	for path := range lastState {
		if _, exists := currentState[path]; !exists {
			// 删除文件
			eventCh <- Event{
				Type:      Deleted,
				Object:    path,
				Timestamp: time.Now(),
			}
		}
	}
}

// detectChanges 扫描文件系统并检测变化，将变化事件发送到eventCh
func (w *FileWatcher) detectChanges(eventCh chan<- Event) {
	// 扫描文件系统，获取当前状态
	currentState, err := scanPath(w.path)
	if err != nil {
		handleError(err, fmt.Sprintf("Failed to scan path %s", w.path), eventCh)
		return
	}

	// 比较状态并发送事件
	compareStates(currentState, w.lastState, eventCh)

	// 更新状态
	w.lastState = currentState
}
