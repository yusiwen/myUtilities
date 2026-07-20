package svcreg

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type svcHandler struct {
	store     Store
	listenAddr string

	mu       sync.Mutex
	eventBus map[string][]*watchSubscriber
	revision int64
}

type watchSubscriber struct {
	serviceId string
	ch        chan *WatchInstanceResponse
	closeCh   chan struct{}
	domain    string
}

func newHandler(store Store, listenAddr string) *svcHandler {
	h := &svcHandler{
		store:      store,
		listenAddr: listenAddr,
		eventBus:   make(map[string][]*watchSubscriber),
	}
	go h.leaseLoop()
	return h
}

func (h *svcHandler) leaseLoop() {
	ticker := time.NewTicker(LeaseScanInterval)
	defer ticker.Stop()
	for range ticker.C {
		expired, err := h.store.GetExpiredInstances(time.Now().Unix())
		if err != nil {
			log.Printf("lease scan error: %v", err)
			continue
		}
		for _, inst := range expired {
			svc, err := h.store.GetService(inst.ServiceId)
			if err != nil {
				continue
			}
			key := &MicroServiceKey{
				AppId: svc.AppId, ServiceName: svc.ServiceName,
				Version: svc.Version, Environment: svc.Environment,
			}
			h.publish(inst.ServiceId, &WatchInstanceResponse{
				Action: EVT_EXPIRE, Key: key, Instance: inst,
			})
		}
	}
}

func (h *svcHandler) nextRevision() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.revision++
	return h.revision
}

func (h *svcHandler) publish(serviceId string, evt *WatchInstanceResponse) {
	h.mu.Lock()
	subs := h.eventBus[serviceId]
	h.mu.Unlock()
	for _, sub := range subs {
		select {
		case sub.ch <- evt:
		case <-sub.closeCh:
		default:
		}
	}
}

func (h *svcHandler) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v4/{project}/registry/version", h.GetVersion)
	mux.HandleFunc("GET /v4/{project}/registry/health", h.ClusterHealth)
	mux.HandleFunc("GET /v4/{project}/registry/existence", h.ResourceExist)
	mux.HandleFunc("GET /v4/{project}/registry/microservices", h.ListService)
	mux.HandleFunc("POST /v4/{project}/registry/microservices", h.RegisterService)
	mux.HandleFunc("GET /v4/{project}/registry/microservices/{serviceId}", h.GetService)
	mux.HandleFunc("PUT /v4/{project}/registry/microservices/{serviceId}/properties", h.PutServiceProperties)
	mux.HandleFunc("DELETE /v4/{project}/registry/microservices/{serviceId}", h.UnregisterService)
	mux.HandleFunc("POST /v4/{project}/registry/microservices/{serviceId}/instances", h.RegisterInstance)
	mux.HandleFunc("GET /v4/{project}/registry/microservices/{serviceId}/instances", h.ListInstance)
	mux.HandleFunc("GET /v4/{project}/registry/microservices/{serviceId}/instances/{instanceId}", h.GetInstance)
	mux.HandleFunc("DELETE /v4/{project}/registry/microservices/{serviceId}/instances/{instanceId}", h.UnregisterInstance)
	mux.HandleFunc("PUT /v4/{project}/registry/microservices/{serviceId}/instances/{instanceId}/properties", h.PutInstanceProperties)
	mux.HandleFunc("PUT /v4/{project}/registry/microservices/{serviceId}/instances/{instanceId}/status", h.PutInstanceStatus)
	mux.HandleFunc("PUT /v4/{project}/registry/microservices/{serviceId}/instances/{instanceId}/heartbeat", h.SendHeartbeat)
	mux.HandleFunc("GET /v4/{project}/registry/instances", h.FindInstances)
	mux.HandleFunc("POST /v4/{project}/registry/instances/action", h.InstancesAction)
	mux.HandleFunc("PUT /v4/{project}/registry/heartbeats", h.SendManyHeartbeat)
	mux.HandleFunc("GET /v4/{project}/registry/microservices/{serviceId}/watcher", h.WatchInstance)
	mux.HandleFunc("POST /v4/{project}/registry/microservices/{serviceId}/tags", h.AddTags)
	mux.HandleFunc("GET /v4/{project}/registry/microservices/{serviceId}/tags", h.ListTags)
	mux.HandleFunc("PUT /v4/{project}/registry/microservices/{serviceId}/tags/{key}", h.UpdateTag)
	mux.HandleFunc("DELETE /v4/{project}/registry/microservices/{serviceId}/tags/{key}", h.DeleteTags)
	mux.HandleFunc("GET /v4/{project}/registry/microservices/{serviceId}/schemas/{schemaId}", h.GetSchema)
	mux.HandleFunc("PUT /v4/{project}/registry/microservices/{serviceId}/schemas/{schemaId}", h.PutSchema)
	mux.HandleFunc("POST /v4/{project}/registry/microservices/{serviceId}/schemas", h.PutSchemas)
	mux.HandleFunc("DELETE /v4/{project}/registry/microservices/{serviceId}/schemas/{schemaId}", h.DeleteSchema)
	mux.HandleFunc("GET /v4/{project}/registry/microservices/{serviceId}/schemas", h.ListSchema)
}

func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		writeError(w, ErrInternal.Code, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func writeError(w http.ResponseWriter, code int32, msg string) {
	w.Header().Set("Content-Type", "application/json")
	httpStatus := http.StatusBadRequest
	if code >= 500000 {
		httpStatus = http.StatusInternalServerError
	} else if code == 403001 {
		httpStatus = http.StatusForbidden
	}
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"errorCode":    code,
		"errorMessage": msg,
	})
}

func writeSvcError(w http.ResponseWriter, err error) {
	if e, ok := err.(*Error); ok {
		writeError(w, e.Code, e.Message)
		return
	}
	writeError(w, ErrInternal.Code, err.Error())
}

func writeSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}

func (h *svcHandler) GetVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"apiVersion": "4.0.0",
		"version":    "1.0.0",
		"listen":     h.listenAddr,
	})
}

func (h *svcHandler) ClusterHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"services": map[string]interface{}{
			"count": 0, "onlineCount": 0,
		},
		"instances": map[string]interface{}{
			"count": 0,
		},
	})
}

func (h *svcHandler) ResourceExist(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch q.Get("type") {
	case ExistTypeMicroservice:
		appId := q.Get("appId")
		serviceName := q.Get("serviceName")
		version := q.Get("version")
		env := q.Get("env")
		if appId == "" || serviceName == "" || version == "" {
			writeError(w, ErrInvalidParams.Code, "appId, serviceName and version are required")
			return
		}
		serviceId, err := h.store.FindServiceId(appId, serviceName, version, env)
		if err != nil {
			writeSvcError(w, err)
			return
		}
		writeJSON(w, &GetExistenceResponse{ServiceId: serviceId})
	case ExistTypeSchema:
		serviceId := q.Get("serviceId")
		schemaId := q.Get("schemaId")
		if serviceId == "" || schemaId == "" {
			writeError(w, ErrInvalidParams.Code, "serviceId and schemaId are required")
			return
		}
		_, err := h.store.GetSchema(serviceId, schemaId)
		if err != nil {
			writeSvcError(w, err)
			return
		}
		writeJSON(w, &GetExistenceResponse{ServiceId: serviceId, SchemaId: schemaId})
	default:
		writeError(w, ErrInvalidParams.Code, "Only microservice and schema can be used as type")
	}
}

func (h *svcHandler) RegisterService(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req CreateServiceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	if req.Service == nil {
		writeError(w, ErrInvalidParams.Code, "service is required")
		return
	}
	if req.Service.AppId == "" {
		req.Service.AppId = AppID
	}
	if req.Service.Version == "" {
		req.Service.Version = Version
	}
	if _, err := h.store.FindServiceId(req.Service.AppId, req.Service.ServiceName, req.Service.Version, req.Service.Environment); err == nil {
		writeSvcError(w, ErrServiceAlreadyExists)
		return
	}
	req.Service.ServiceId = NewUUID()
	if req.Service.Status == "" {
		req.Service.Status = MS_UP
	}
	req.Service.Timestamp = now()
	if err := h.store.CreateService(req.Service); err != nil {
		writeSvcError(w, err)
		return
	}
	writeJSON(w, &CreateServiceResponse{ServiceId: req.Service.ServiceId})
}

func (h *svcHandler) GetService(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	svc, err := h.store.GetService(serviceId)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	writeJSON(w, &GetServiceResponse{Service: svc})
}

func (h *svcHandler) ListService(w http.ResponseWriter, r *http.Request) {
	services, err := h.store.ListService()
	if err != nil {
		writeSvcError(w, err)
		return
	}
	writeJSON(w, &GetServicesResponse{Services: services})
}

func (h *svcHandler) PutServiceProperties(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req UpdateServicePropsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	if err := h.store.UpdateServiceProperties(serviceId, req.Properties); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) UnregisterService(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	if err := h.store.DeleteService(serviceId); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) RegisterInstance(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req RegisterInstanceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	if req.Instance == nil {
		writeError(w, ErrInvalidParams.Code, "instance is required")
		return
	}
	req.Instance.ServiceId = serviceId
	req.Instance.InstanceId = NewUUID()
	if req.Instance.Status == "" {
		req.Instance.Status = MSI_UP
	}
	req.Instance.Timestamp = now()
	if req.Instance.HealthCheck == nil {
		req.Instance.HealthCheck = &HealthCheck{
			Mode: CHECK_BY_HEARTBEAT, Interval: DefaultLeaseInterval, Times: DefaultLeaseTimes,
		}
	}
	leaseDeadline := time.Now().Unix() + int64(req.Instance.HealthCheck.Interval*req.Instance.HealthCheck.Times)
	if _, err := h.store.GetService(serviceId); err != nil {
		writeSvcError(w, err)
		return
	}
	if err := h.store.CreateInstance(req.Instance, leaseDeadline); err != nil {
		writeSvcError(w, err)
		return
	}
	h.publish(serviceId, &WatchInstanceResponse{
		Action: EVT_CREATE, Instance: req.Instance,
		Key: &MicroServiceKey{ServiceName: serviceId},
	})
	writeJSON(w, &RegisterInstanceResponse{InstanceId: req.Instance.InstanceId})
}

func (h *svcHandler) GetInstance(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	instanceId := r.PathValue("instanceId")
	inst, err := h.store.GetInstance(serviceId, instanceId)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	writeJSON(w, &GetOneInstanceResponse{Instance: inst})
}

func (h *svcHandler) ListInstance(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	instances, err := h.store.ListInstance(serviceId)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	writeJSON(w, &GetInstancesResponse{Instances: instances})
}

func (h *svcHandler) UnregisterInstance(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	instanceId := r.PathValue("instanceId")
	if err := h.store.DeleteInstance(serviceId, instanceId); err != nil {
		writeSvcError(w, err)
		return
	}
	h.publish(serviceId, &WatchInstanceResponse{
		Action: EVT_DELETE,
		Instance: &MicroServiceInstance{
			ServiceId: serviceId, InstanceId: instanceId,
		},
	})
	writeSuccess(w)
}

func (h *svcHandler) PutInstanceProperties(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	instanceId := r.PathValue("instanceId")
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req UpdateInstancePropsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	if err := h.store.UpdateInstanceProperties(serviceId, instanceId, req.Properties); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) PutInstanceStatus(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	instanceId := r.PathValue("instanceId")
	status := r.URL.Query().Get("value")
	if status == "" {
		writeError(w, ErrInvalidParams.Code, "status value is required")
		return
	}
	if err := h.store.UpdateInstanceStatus(serviceId, instanceId, status); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) SendHeartbeat(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	instanceId := r.PathValue("instanceId")
	inst, err := h.store.GetInstance(serviceId, instanceId)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	interval := DefaultLeaseInterval
	times := DefaultLeaseTimes
	if inst.HealthCheck != nil {
		if inst.HealthCheck.Interval > 0 {
			interval = int(inst.HealthCheck.Interval)
		}
		if inst.HealthCheck.Times > 0 {
			times = int(inst.HealthCheck.Times)
		}
	}
	leaseDeadline := time.Now().Unix() + int64(interval*times)
	if err := h.store.UpdateInstanceHeartbeat(serviceId, instanceId, leaseDeadline); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) SendManyHeartbeat(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req HeartbeatSetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var results []*InstanceHbRst
	for _, elem := range req.Instances {
		if elem == nil {
			continue
		}
		inst, err := h.store.GetInstance(elem.ServiceId, elem.InstanceId)
		if err != nil {
			results = append(results, &InstanceHbRst{
				ServiceId: elem.ServiceId, InstanceId: elem.InstanceId,
				ErrMessage: err.Error(),
			})
			continue
		}
		interval := DefaultLeaseInterval
		times := DefaultLeaseTimes
		if inst.HealthCheck != nil {
			if inst.HealthCheck.Interval > 0 {
				interval = int(inst.HealthCheck.Interval)
			}
			if inst.HealthCheck.Times > 0 {
				times = int(inst.HealthCheck.Times)
			}
		}
		leaseDeadline := time.Now().Unix() + int64(interval*times)
		if err := h.store.UpdateInstanceHeartbeat(elem.ServiceId, elem.InstanceId, leaseDeadline); err != nil {
			results = append(results, &InstanceHbRst{
				ServiceId: elem.ServiceId, InstanceId: elem.InstanceId,
				ErrMessage: err.Error(),
			})
		}
	}
	writeJSON(w, &HeartbeatSetResponse{Instances: results})
}

func (h *svcHandler) FindInstances(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	appId := q.Get("appId")
	serviceName := q.Get("serviceName")
	env := q.Get("env")
	if serviceName == "" {
		writeError(w, ErrInvalidParams.Code, "serviceName is required")
		return
	}
	if appId == "" {
		appId = AppID
	}
	ids, err := h.store.FindServiceIdByServiceName(appId, serviceName)
	if err != nil || len(ids) == 0 {
		writeJSON(w, &FindInstancesResponse{Instances: []*MicroServiceInstance{}})
		return
	}
	var instances []*MicroServiceInstance
	for _, id := range ids {
		svc, err := h.store.GetService(id)
		if err != nil {
			continue
		}
		if env != "" && svc.Environment != "" && svc.Environment != env {
			continue
		}
		insts, err := h.store.ListInstance(id)
		if err != nil {
			continue
		}
		instances = append(instances, insts...)
	}
	writeJSON(w, &FindInstancesResponse{Instances: instances})
}

func (h *svcHandler) InstancesAction(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("type")
	if action != "query" {
		writeError(w, ErrInvalidParams.Code, "only type=query is supported")
		return
	}
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req BatchFindInstancesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	result := &BatchFindResult{}
	for i, s := range req.Services {
		if s == nil || s.Service == nil {
			continue
		}
		appId := s.Service.AppId
		if appId == "" {
			appId = AppID
		}
		version := s.Service.Version
		if version == "" {
			version = "latest"
		}
		env := s.Service.Environment
		serviceName := s.Service.ServiceName
		if s.Service.Alias != "" {
			serviceName = s.Service.Alias
		}
		ids, err := h.store.FindServiceIdByServiceName(appId, serviceName)
		if err != nil || len(ids) == 0 {
			continue
		}
		var instances []*MicroServiceInstance
		for _, id := range ids {
			svc, err := h.store.GetService(id)
			if err != nil {
				continue
			}
			if env != "" && svc.Environment != "" && svc.Environment != env {
				continue
			}
			if version != "latest" && svc.Version != version {
				continue
			}
			insts, err := h.store.ListInstance(id)
			if err != nil {
				continue
			}
			instances = append(instances, insts...)
		}
		if len(instances) == 0 {
			result.Updated = append(result.Updated, &FindResult{
				Index: int64(i), Rev: "0", Instances: []*MicroServiceInstance{},
			})
		} else {
			result.Updated = append(result.Updated, &FindResult{
				Index: int64(i), Rev: "0", Instances: instances,
			})
		}
	}
	writeJSON(w, &BatchFindInstancesResponse{Services: result})
}

func (h *svcHandler) AddTags(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	tags, ok := payload["tags"]
	if !ok {
		writeError(w, ErrInvalidParams.Code, "tags field is required")
		return
	}
	if err := h.store.CreateTags(serviceId, tags); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	tags, err := h.store.GetTags(serviceId)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	writeJSON(w, &GetServiceTagsResponse{Tags: tags})
}

func (h *svcHandler) UpdateTag(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	key := r.PathValue("key")
	value := r.URL.Query().Get("value")
	if err := h.store.UpdateTag(serviceId, key, value); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) DeleteTags(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	key := r.PathValue("key")
	keys := strings.Split(key, ",")
	if err := h.store.DeleteTags(serviceId, keys); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	schemaId := r.PathValue("schemaId")
	sch, err := h.store.GetSchema(serviceId, schemaId)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	w.Header().Add("X-Schema-Summary", sch.Summary)
	writeJSON(w, &GetSchemaResponse{
		Schema: sch.Schema, SchemaSummary: sch.Summary,
	})
}

func (h *svcHandler) PutSchema(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	schemaId := r.PathValue("schemaId")
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req ModifySchemaRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	if err := h.store.CreateSchema(serviceId, schemaId, &Schema{
		SchemaId: schemaId, Schema: req.Schema, Summary: req.Summary,
	}); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) PutSchemas(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	body, err := readBody(r)
	if err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	var req ModifySchemasRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, ErrInvalidParams.Code, err.Error())
		return
	}
	for _, sch := range req.Schemas {
		if sch == nil {
			continue
		}
		if err := h.store.CreateSchema(serviceId, sch.SchemaId, sch); err != nil {
			writeSvcError(w, err)
			return
		}
	}
	writeSuccess(w)
}

func (h *svcHandler) DeleteSchema(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	schemaId := r.PathValue("schemaId")
	if err := h.store.DeleteSchema(serviceId, schemaId); err != nil {
		writeSvcError(w, err)
		return
	}
	writeSuccess(w)
}

func (h *svcHandler) ListSchema(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	schemas, err := h.store.ListSchema(serviceId)
	if err != nil {
		writeSvcError(w, err)
		return
	}
	writeJSON(w, &GetAllSchemaResponse{Schemas: schemas})
}

func (h *svcHandler) WatchInstance(w http.ResponseWriter, r *http.Request) {
	serviceId := r.PathValue("serviceId")
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		writeError(w, ErrInternal.Code, fmt.Sprintf("websocket upgrade: %v", err))
		return
	}
	defer conn.Close()

	sub := &watchSubscriber{
		serviceId: serviceId,
		ch:        make(chan *WatchInstanceResponse, 100),
		closeCh:   make(chan struct{}),
		domain:    r.PathValue("project"),
	}
	h.mu.Lock()
	h.eventBus[serviceId] = append(h.eventBus[serviceId], sub)
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		subs := h.eventBus[serviceId]
		for i, s := range subs {
			if s == sub {
				h.eventBus[serviceId] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(h.eventBus[serviceId]) == 0 {
			delete(h.eventBus, serviceId)
		}
		h.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case evt := <-sub.ch:
			evt.Response = nil
			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
