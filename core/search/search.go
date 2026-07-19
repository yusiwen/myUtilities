package search

import "context"

type Result struct {
	Title   string
	URL     string
	Snippet string
}

type Searcher interface {
	Search(ctx context.Context, query string, count int) ([]Result, error)
}
