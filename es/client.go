package es

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
)

func newESClient(cfg *ESConfig) (*elasticsearch.Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("ES host not configured")
	}
	esCfg := elasticsearch.Config{
		Addresses: []string{cfg.Host},
	}
	if cfg.Username != "" || cfg.Password != "" {
		esCfg.Username = cfg.Username
		esCfg.Password = cfg.Password
	}
	esCfg.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return elasticsearch.NewClient(esCfg)
}

func esPing(es *elasticsearch.Client) (map[string]interface{}, error) {
	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ES: %v", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("ES error: %s", string(body))
	}
	var info map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse ES info: %v", err)
	}
	return info, nil
}

func esListIndices(es *elasticsearch.Client) ([]string, error) {
	res, err := es.Cat.Indices(es.Cat.Indices.WithFormat("json"), es.Cat.Indices.WithH("index"))
	if err != nil {
		return nil, fmt.Errorf("failed to list indices: %v", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("ES error: %s", string(body))
	}
	var rows []map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("failed to parse indices: %v", err)
	}
	indices := make([]string, 0, len(rows))
	for _, row := range rows {
		if idx, ok := row["index"].(string); ok {
			indices = append(indices, idx)
		}
	}
	return indices, nil
}

func esSearch(es *elasticsearch.Client, index string, body map[string]interface{}) (map[string]interface{}, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("failed to encode search body: %v", err)
	}
	res, err := es.Search(
		es.Search.WithIndex(index),
		es.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, fmt.Errorf("search failed: %v", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		errBody, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("ES error: %s", string(errBody))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse search result: %v", err)
	}
	return result, nil
}
