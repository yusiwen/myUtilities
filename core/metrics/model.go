package metrics

type DataPoint struct {
	Timestamp int64   `json:"t"`
	Value     float64 `json:"v"`
}

type Metric struct {
	Name   string            `json:"metric"`
	Tags   map[string]string `json:"tags"`
	Points []DataPoint       `json:"points"`
}

type WriteRequest struct {
	Metric    string            `json:"metric"`
	Tags      map[string]string `json:"tags"`
	Timestamp int64             `json:"time,omitempty"`
	Value     float64           `json:"value"`
}
