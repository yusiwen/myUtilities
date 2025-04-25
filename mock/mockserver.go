package mock

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/ryanolee/go-chaff"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const schema = `{
	"properties": {
		"id": {"type": "integer"},
		"name": {"type": "string"}
	},
	"required": ["id", "name"]
}`

var data map[string][]interface{}

func loadFile(fileName string) error {
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}
	header := records[0]
	rs := make([]map[string]string, len(records)-1)
	for i := 1; i < len(records); i++ {
		rs[i-1] = make(map[string]string)
		for j := 0; j < len(header); j++ {
			rs[i-1][header[j]] = records[i][j]
		}
	}
	fileNameWithoutExt := fileNameWithoutExtension(fileName)
	data[fileNameWithoutExt] = make([]interface{}, len(rs))
	d := data[fileNameWithoutExt]
	for i := 0; i < len(rs); i++ {
		d[i] = rs[i]
	}
	fmt.Printf("loaded %d records from %s\n", len(d), fileName)
	return nil
}

func loadRandomData(size int) error {
	data["default"] = make([]interface{}, size)
	d := data["default"]
	for i := 0; i < size; i++ {
		generator, err := chaff.ParseSchemaStringWithDefaults(schema)
		if err != nil {
			return err
		}

		result := generator.GenerateWithDefaults()

		d = append(d, result)
	}
	return nil
}

func (o *MockServerOptions) generateData() error {
	data = make(map[string][]interface{})

	if o.CsvFiles != "" {
		files := strings.Split(o.CsvFiles, ";")
		for _, file := range files {
			err := loadFile(file)
			if err != nil {
				return err
			}
		}
	} else {
		err := loadRandomData(o.Size)
		if err != nil {
			return err
		}
	}
	return nil
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

	err := o.generateData()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/mock/query/{rs}", o.queryHandler)

	fmt.Printf("Server listening at :%d\n", o.Port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux); err != nil {
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

	pageNo := max(req.PageNo, 1)
	pageSize := req.PageSize

	rsName := r.PathValue("rs")
	if len(rsName) == 0 {
		rsName = "default"
	}
	d := data[rsName]

	maxPageNo := (len(d) + pageSize - 1) / pageSize
	fmt.Println("len(d): ", len(d))
	fmt.Printf("pageNo: %d, pageSize: %d, maxPageNo: %d\n", pageNo, pageSize, maxPageNo)
	var result interface{}
	if pageNo > maxPageNo {
		result = []interface{}{}
	} else {
		result = d[(pageNo-1)*pageSize : min(len(d), pageNo*pageSize)]
	}

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

func fileNameWithoutExtension(fileName string) string {
	return strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
}
