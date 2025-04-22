package mock

import (
	"encoding/json"
	"fmt"
	"github.com/ryanolee/go-chaff"
	"net/http"
	"os"
	"strconv"
)

const schema = `{"properties": {
		"id": {"type": "integer"},
		"name": {"type": "string"}
	}
}`

var data []interface{}

func init() {
	for i := 0; i < 100; i++ {
		generator, err := chaff.ParseSchemaStringWithDefaults(schema)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		result := generator.GenerateWithDefaults()

		data = append(data, result)
	}
}

type MockResponse struct {
	Response
	PageNo   int         `json:"pageNo"`
	PageSize int         `json:"pageSize"`
	Total    int         `json:"total"`
	Data     interface{} `json:"data"`
}

func (o *MockServerOptions) Run() error {
	http.HandleFunc("/api/mock/query", o.queryHandler)

	fmt.Printf("Server listening at :%d\n", o.Port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", o.Port), nil); err != nil {
		return fmt.Errorf("server listen failed: %v", err)
	}
	return nil
}

func (o *MockServerOptions) queryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"code": "0", "msg": "GET method only"}`, http.StatusOK)
		return
	}
	pageNo := parseInt(r.URL.Query().Get("pageNo"), 1)
	pageSize := min(parseInt(r.URL.Query().Get("pageSize"), 10), len(data))

	maxPageNo := (len(data) + pageSize - 1) / pageSize
	pageNo = min(maxPageNo, pageNo)

	result := data[pageNo-1 : pageNo-1+pageSize]

	resp := MockResponse{
		Response: Response{
			Code: "1",
			Msg:  "OK",
		},
		Data:     result,
		PageNo:   pageNo,
		PageSize: pageSize,
		Total:    len(data),
	}
	res, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, `{"code": "0", "msg": "JSON generating error"}`, http.StatusOK)
		return
	}

	fmt.Fprintf(w, "%s", res)
	return
}

func parseInt(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	if result, err := strconv.Atoi(value); err == nil {
		return result
	}
	return defaultValue
}
