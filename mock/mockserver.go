package mock

import (
	"encoding/json"
	"fmt"
	"github.com/ryanolee/go-chaff"
	"net/http"
	"os"
)

const schema = `{
	"properties": {
		"id": {"type": "integer"},
		"name": {"type": "string"}
	},
	"required": ["id", "name"]
}`

var data []interface{}

func generateData(size int) {
	for i := 0; i < size; i++ {
		generator, err := chaff.ParseSchemaStringWithDefaults(schema)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		result := generator.GenerateWithDefaults()

		data = append(data, result)
	}
}

type Result struct {
	Data interface{} `json:"Data"`
}

type MockResponse struct {
	Response
	Result Result `json:"Result"`
}

func (o *MockServerOptions) Run() error {
	if o.Size > 10000 {
		return fmt.Errorf("size to large, max 10000")
	}

	generateData(o.Size)

	http.HandleFunc("/api/mock/query", o.queryHandler)

	fmt.Printf("Server listening at :%d\n", o.Port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", o.Port), nil); err != nil {
		return fmt.Errorf("server listen failed: %v", err)
	}
	return nil
}

type queryRequest struct {
	PageNo   int `json:"pageNo"`
	PageSize int `json:"pageSize"`
}

func (o *MockServerOptions) queryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"Status": {"Code": "1", "Message": "POST method only"}}`, http.StatusOK)
		return
	}

	var req queryRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, `{"Status": {"Code": "2", "Message": "JSON parsing error"}}`, http.StatusOK)
		return
	}

	pageNo := req.PageNo
	pageSize := req.PageSize

	maxPageNo := (len(data) + pageSize - 1) / pageSize
	pageNo = min(maxPageNo, pageNo)

	result := data[pageNo-1 : pageNo-1+pageSize]

	resp := MockResponse{
		Response: Response{
			Status: Status{
				Code:    "0",
				Message: "OK",
			},
		},
		Result: Result{
			Data: result,
		},
	}
	res, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, `{"Status": {"Code": "3", "Message": "JSON generating error"}}`, http.StatusOK)
		return
	}

	fmt.Fprintf(w, "%s", res)
	return
}
