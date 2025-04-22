package mock

import (
	"encoding/json"
	"fmt"
	"github.com/ryanolee/go-chaff"
	"net/http"
)

type MockServerOptions struct {
	Port int `help:"Port to listen on." default:"8081"`
}

const schema = `{"properties": {
		"id": {"type": "integer"},
		"name": {"type": "string"}
	}
}`

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

	generator, err := chaff.ParseSchemaStringWithDefaults(schema)
	if err != nil {
		http.Error(w, `{"code": "0", "msg": "Schema loading error"}`, http.StatusOK)
		return
	}

	fmt.Println(generator.Metadata.Errors)
	result := generator.GenerateWithDefaults()

	res, err := json.Marshal(result)
	if err != nil {
		http.Error(w, `{"code": "0", "msg": "JSON generating error"}`, http.StatusOK)
		return
	}

	fmt.Fprintf(w, `{"code": "1", "msg": "OK", "data": %s}`, res)
	return
}
