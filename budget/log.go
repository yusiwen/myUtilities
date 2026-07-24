package budget

import (
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	debugLogger   *log.Logger
	loggerInit    sync.Once
	loggerInitErr error
)

func ensureLogger() {
	loggerInit.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Printf("budget: cannot find home dir: %v", err)
			loggerInitErr = err
			return
		}
		dir := filepath.Join(home, ".config", "mu")
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Printf("budget: cannot create config dir: %v", err)
			loggerInitErr = err
			return
		}
		f, err := os.OpenFile(filepath.Join(dir, "budget.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("budget: cannot open log file: %v", err)
			loggerInitErr = err
			return
		}
		debugLogger = log.New(f, "", log.LstdFlags)
	})
}

func debugLog(format string, args ...interface{}) {
	ensureLogger()
	if debugLogger != nil {
		debugLogger.Printf("budget: "+format, args...)
	} else {
		log.Printf("budget: "+format, args...)
	}
}
