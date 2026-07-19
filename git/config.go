package git

import "github.com/yusiwen/myUtilities/core/llm"

func loadConfig() (*llm.Config, error) {
	return llm.LoadConfig("commit")
}
