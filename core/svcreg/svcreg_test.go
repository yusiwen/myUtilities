package svcreg

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestServer() (*httptest.Server, *svcHandler) {
	store := newMemStore()
	handler := newHandler(store, ":0")
	mux := http.NewServeMux()
	handler.registerRoutes(mux)
	server := httptest.NewServer(mux)
	return server, handler
}

func TestAPIRegisterAndGetService(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	body := `{"service":{"appId":"test-app","serviceName":"hello","version":"1.0.0"}}`
	resp, err := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST service: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var createResp CreateServiceResponse
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	if createResp.ServiceId == "" {
		t.Fatal("expected non-empty serviceId")
	}

	getResp, err := http.Get(server.URL + "/v4/default/registry/microservices/" + createResp.ServiceId)
	if err != nil {
		t.Fatalf("GET service: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var svcResp GetServiceResponse
	json.NewDecoder(getResp.Body).Decode(&svcResp)
	getResp.Body.Close()

	if svcResp.Service.ServiceId != createResp.ServiceId {
		t.Errorf("expected serviceId %s, got %s", createResp.ServiceId, svcResp.Service.ServiceId)
	}
	if svcResp.Service.AppId != "test-app" {
		t.Errorf("expected appId test-app, got %s", svcResp.Service.AppId)
	}
}

func TestAPIRegisterDuplicateService(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	body := `{"service":{"appId":"test","serviceName":"dup","version":"1.0.0"}}`
	http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(body))

	resp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(body))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate, got %d", resp.StatusCode)
	}

	var errResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()

	if errResp["errorCode"] != float64(400010) {
		t.Errorf("expected errorCode 400010, got %v", errResp["errorCode"])
	}
}

func TestAPIRegisterAndHeartbeatInstance(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"hb-test","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	instBody := `{"instance":{"hostName":"node1","endpoints":["rest://127.0.0.1:8080"]}}`
	instResp, err := http.Post(server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances",
		"application/json", strings.NewReader(instBody))
	if err != nil {
		t.Fatalf("POST instance: %v", err)
	}
	if instResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", instResp.StatusCode)
	}

	var inst RegisterInstanceResponse
	json.NewDecoder(instResp.Body).Decode(&inst)
	instResp.Body.Close()

	if inst.InstanceId == "" {
		t.Fatal("expected non-empty instanceId")
	}

	hbURL := server.URL + "/v4/default/registry/microservices/" + svc.ServiceId + "/instances/" + inst.InstanceId + "/heartbeat"
	req, _ := http.NewRequest("PUT", hbURL, nil)
	hbResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT heartbeat: %v", err)
	}
	if hbResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for heartbeat, got %d", hbResp.StatusCode)
	}
	hbResp.Body.Close()
}

func TestAPIHeartbeatNonExistentInstance(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	req, _ := http.NewRequest("PUT", server.URL+"/v4/default/registry/microservices/svc-1/instances/inst-999/heartbeat", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT heartbeat: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-existent instance, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPIFindInstances(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"find-me","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	instBody := `{"instance":{"hostName":"node1","endpoints":["rest://127.0.0.1:8080"]}}`
	http.Post(server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances",
		"application/json", strings.NewReader(instBody))

	findResp, err := http.Get(server.URL + "/v4/default/registry/instances?appId=test&serviceName=find-me&version=1.0.0")
	if err != nil {
		t.Fatalf("GET find instances: %v", err)
	}
	if findResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", findResp.StatusCode)
	}

	var result FindInstancesResponse
	json.NewDecoder(findResp.Body).Decode(&result)
	findResp.Body.Close()

	if len(result.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(result.Instances))
	}
	if result.Instances[0].HostName != "node1" {
		t.Errorf("expected hostName node1, got %s", result.Instances[0].HostName)
	}
}

func TestAPIExistenceCheck(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"exist-test","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	existResp, err := http.Get(server.URL + "/v4/default/registry/existence?type=microservice&appId=test&serviceName=exist-test&version=1.0.0")
	if err != nil {
		t.Fatalf("GET existence: %v", err)
	}
	if existResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", existResp.StatusCode)
	}

	var exist GetExistenceResponse
	json.NewDecoder(existResp.Body).Decode(&exist)
	existResp.Body.Close()

	if exist.ServiceId != svc.ServiceId {
		t.Errorf("expected serviceId %s, got %s", svc.ServiceId, exist.ServiceId)
	}

	notFoundResp, _ := http.Get(server.URL + "/v4/default/registry/existence?type=microservice&appId=test&serviceName=nonexistent&version=1.0.0")
	if notFoundResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-existent, got %d", notFoundResp.StatusCode)
	}
	notFoundResp.Body.Close()
}

func TestAPIListService(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	body := `{"service":{"appId":"test","serviceName":"svc1","version":"1.0.0"}}`
	http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(body))
	body = `{"service":{"appId":"test","serviceName":"svc2","version":"1.0.0"}}`
	http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(body))

	resp, _ := http.Get(server.URL + "/v4/default/registry/microservices")
	var list GetServicesResponse
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()

	if len(list.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(list.Services))
	}
}

func TestAPITags(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"tag-test","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	tagBody := `{"tags":{"env":"prod","group":"api"}}`
	tagResp, err := http.Post(server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/tags",
		"application/json", strings.NewReader(tagBody))
	if err != nil {
		t.Fatalf("POST tags: %v", err)
	}
	if tagResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", tagResp.StatusCode)
	}
	tagResp.Body.Close()

	getResp, _ := http.Get(server.URL + "/v4/default/registry/microservices/" + svc.ServiceId + "/tags")
	var tagsResp GetServiceTagsResponse
	json.NewDecoder(getResp.Body).Decode(&tagsResp)
	getResp.Body.Close()

	if tagsResp.Tags["env"] != "prod" {
		t.Errorf("expected env=prod, got %s", tagsResp.Tags["env"])
	}
}

func TestAPIVersion(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	resp, _ := http.Get(server.URL + "/v4/default/registry/version")
	var version map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&version)
	resp.Body.Close()

	if version["apiVersion"] != "4.0.0" {
		t.Errorf("expected apiVersion 4.0.0, got %v", version["apiVersion"])
	}
}

func TestAPIDeleteServiceThenGetFails(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"del-test","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	req, _ := http.NewRequest("DELETE", server.URL+"/v4/default/registry/microservices/"+svc.ServiceId, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	getResp, _ := http.Get(server.URL + "/v4/default/registry/microservices/" + svc.ServiceId)
	if getResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 after delete, got %d", getResp.StatusCode)
	}
	getResp.Body.Close()
}

func TestAPIDeleteInstance(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"inst-del","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	instBody := `{"instance":{"hostName":"node1"}}`
	instResp, _ := http.Post(server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances",
		"application/json", strings.NewReader(instBody))
	var inst RegisterInstanceResponse
	json.NewDecoder(instResp.Body).Decode(&inst)
	instResp.Body.Close()

	req, _ := http.NewRequest("DELETE", server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances/"+inst.InstanceId, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	getResp, _ := http.Get(server.URL + "/v4/default/registry/microservices/" + svc.ServiceId + "/instances/" + inst.InstanceId)
	if getResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 after instance delete, got %d", getResp.StatusCode)
	}
	getResp.Body.Close()
}

func TestAPIBatchHeartbeat(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"batch-hb","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	instBody := `{"instance":{"hostName":"node1"}}`
	instResp, _ := http.Post(server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances",
		"application/json", strings.NewReader(instBody))
	var inst RegisterInstanceResponse
	json.NewDecoder(instResp.Body).Decode(&inst)
	instResp.Body.Close()

	hbBody := `{"instances":[{"serviceId":"` + svc.ServiceId + `","instanceId":"` + inst.InstanceId + `"}]}`
	req, _ := http.NewRequest("PUT", server.URL+"/v4/default/registry/heartbeats", strings.NewReader(hbBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for batch heartbeat, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAPIUpdateInstanceStatus(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"test","serviceName":"status-test","version":"1.0.0"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	instBody := `{"instance":{"hostName":"node1"}}`
	instResp, _ := http.Post(server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances",
		"application/json", strings.NewReader(instBody))
	var inst RegisterInstanceResponse
	json.NewDecoder(instResp.Body).Decode(&inst)
	instResp.Body.Close()

	req, _ := http.NewRequest("PUT", server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances/"+inst.InstanceId+"/status?value=DOWN", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	getResp, _ := http.Get(server.URL + "/v4/default/registry/microservices/" + svc.ServiceId + "/instances/" + inst.InstanceId)
	var getInst GetOneInstanceResponse
	json.NewDecoder(getResp.Body).Decode(&getInst)
	getResp.Body.Close()

	if getInst.Instance.Status != "DOWN" {
		t.Errorf("expected status DOWN, got %s", getInst.Instance.Status)
	}
}

func TestAPIExistenceWithEnv(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	body := `{"service":{"appId":"myapp","serviceName":"env-svc","version":"1.0.0","environment":"development"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(body))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	existResp, _ := http.Get(server.URL + "/v4/default/registry/existence?type=microservice&appId=myapp&serviceName=env-svc&version=1.0.0&env=development")
	if existResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with env match, got %d", existResp.StatusCode)
	}
	existResp.Body.Close()

	existResp2, _ := http.Get(server.URL + "/v4/default/registry/existence?type=microservice&appId=myapp&serviceName=env-svc&version=1.0.0")
	if existResp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 without env match, got %d", existResp2.StatusCode)
	}
	existResp2.Body.Close()
}

func TestAPIBatchFindInstances(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	svcBody := `{"service":{"appId":"myapp","serviceName":"batch-svc","version":"1.0.0","environment":"development"}}`
	svcResp, _ := http.Post(server.URL+"/v4/default/registry/microservices", "application/json", strings.NewReader(svcBody))
	var svc CreateServiceResponse
	json.NewDecoder(svcResp.Body).Decode(&svc)
	svcResp.Body.Close()

	body := `{"instance":{"hostName":"node1","endpoints":["rest://127.0.0.1:8080"]}}`
	http.Post(server.URL+"/v4/default/registry/microservices/"+svc.ServiceId+"/instances",
		"application/json", strings.NewReader(body))

	queryBody := `{"services":[{"service":{"appId":"myapp","serviceName":"batch-svc","version":"1.0.0","environment":"development"}}]}`
	resp, _ := http.Post(server.URL+"/v4/default/registry/instances/action?type=query",
		"application/json", strings.NewReader(queryBody))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result BatchFindInstancesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result.Services == nil || len(result.Services.Updated) != 1 {
		t.Fatalf("expected 1 updated result, got %v", result.Services)
	}
	if len(result.Services.Updated[0].Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(result.Services.Updated[0].Instances))
	}
	if result.Services.Updated[0].Instances[0].HostName != "node1" {
		t.Errorf("expected hostName node1, got %s", result.Services.Updated[0].Instances[0].HostName)
	}
}

func TestAPIBatchFindNoInstance(t *testing.T) {
	server, _ := setupTestServer()
	defer server.Close()

	queryBody := `{"services":[{"service":{"appId":"myapp","serviceName":"nonexistent","version":"1.0.0"}}]}`
	resp, _ := http.Post(server.URL+"/v4/default/registry/instances/action?type=query",
		"application/json", strings.NewReader(queryBody))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result BatchFindInstancesResponse
	json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()

	if result.Services != nil && len(result.Services.Updated) > 0 {
		t.Fatalf("expected no results for non-existent service, got %d", len(result.Services.Updated))
	}
}
