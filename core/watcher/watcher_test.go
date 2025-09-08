package watcher

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestFileWatcher(t *testing.T) {
	server := NewWatchServer()
	fileKey := ResourceKey{
		Group:     "core",
		Name:      "file",
		Namespace: "default",
		Resource:  "file",
		Version:   "v1",
	}
	fileWatcher := NewFileWatcher("/Users/yusiwen/Downloads/tmp", 5*time.Second)
	err := server.RegisterWatcher(fileKey, fileWatcher)
	if err != nil {
		fmt.Println(err)
		return
	}

	ch, clientId, err := server.Watch(fileKey, "")
	defer server.Unwatch(fileKey, clientId)

	var wg sync.WaitGroup
	wg.Add(1)

	// 处理事件
	go func() {
		defer wg.Done()
		for event := range ch {
			switch event.Type {
			case Added:
				fmt.Println("新增:", event.Object)
			case Modified:
				fmt.Println("更新:", event.Object)
			case Deleted:
				fmt.Println("删除:", event.Object)
			case Error:
				fmt.Println("错误:", event.Object)
			default:
				fmt.Println("未知事件:", event.Object)
			}
		}
	}()

	wg.Wait()
}
