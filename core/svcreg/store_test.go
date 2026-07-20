package svcreg

import (
	"testing"
	"time"
)

func newTestService() *MicroService {
	return &MicroService{
		ServiceId:   NewUUID(),
		AppId:       "test-app",
		ServiceName: "test-service",
		Version:     "1.0.0",
		Status:      MS_UP,
	}
}

func newTestInstance(serviceId string) *MicroServiceInstance {
	return &MicroServiceInstance{
		InstanceId: NewUUID(),
		ServiceId:  serviceId,
		HostName:   "test-host",
		Status:     MSI_UP,
		Endpoints:  []string{"rest://127.0.0.1:8080"},
		HealthCheck: &HealthCheck{
			Mode: CHECK_BY_HEARTBEAT, Interval: DefaultLeaseInterval, Times: DefaultLeaseTimes,
		},
	}
}

func TestStoreCreateAndGetService(t *testing.T) {
	s := newMemStore()
	svc := newTestService()

	if err := s.CreateService(svc); err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	got, err := s.GetService(svc.ServiceId)
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if got.ServiceId != svc.ServiceId {
		t.Errorf("expected serviceId %s, got %s", svc.ServiceId, got.ServiceId)
	}
	if got.AppId != "test-app" {
		t.Errorf("expected appId test-app, got %s", got.AppId)
	}
}

func TestStoreCreateDuplicateService(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)

	err := s.CreateService(svc)
	if err != ErrServiceAlreadyExists {
		t.Fatalf("expected ErrServiceAlreadyExists, got %v", err)
	}
}

func TestStoreGetNonExistentService(t *testing.T) {
	s := newMemStore()
	_, err := s.GetService("nonexistent")
	if err != ErrServiceNotExists {
		t.Fatalf("expected ErrServiceNotExists, got %v", err)
	}
}

func TestStoreListService(t *testing.T) {
	s := newMemStore()
	svc1 := newTestService()
	svc2 := newTestService()
	svc2.AppId = "test-app-2"
	svc2.ServiceName = "test-service-2"
	svc2.Version = "2.0.0"

	s.CreateService(svc1)
	s.CreateService(svc2)

	services, err := s.ListService()
	if err != nil {
		t.Fatalf("ListService: %v", err)
	}
	if len(services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(services))
	}
}

func TestStoreFindServiceId(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)

	id, err := s.FindServiceId("test-app", "test-service", "1.0.0", "")
	if err != nil {
		t.Fatalf("FindServiceId: %v", err)
	}
	if id != svc.ServiceId {
		t.Errorf("expected %s, got %s", svc.ServiceId, id)
	}

	_, err = s.FindServiceId("test-app", "other-service", "1.0.0", "")
	if err != ErrServiceNotExists {
		t.Errorf("expected ErrServiceNotExists, got %v", err)
	}
}

func TestStoreDeleteService(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)

	inst := newTestInstance(svc.ServiceId)
	s.CreateInstance(inst, time.Now().Unix()+100)

	if err := s.DeleteService(svc.ServiceId); err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	if _, err := s.GetService(svc.ServiceId); err != ErrServiceNotExists {
		t.Error("service should not exist after delete")
	}
	if _, err := s.GetInstance(svc.ServiceId, inst.InstanceId); err != ErrInstanceNotExists {
		t.Error("instances should be cascaded on service delete")
	}
}

func TestStoreCreateAndGetInstance(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)

	inst := newTestInstance(svc.ServiceId)
	leaseDeadline := time.Now().Unix() + 90
	if err := s.CreateInstance(inst, leaseDeadline); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	got, err := s.GetInstance(svc.ServiceId, inst.InstanceId)
	if err != nil {
		t.Fatalf("GetInstance: %v", err)
	}
	if got.InstanceId != inst.InstanceId {
		t.Errorf("expected instanceId %s, got %s", inst.InstanceId, got.InstanceId)
	}
}

func TestStoreUpdateInstanceHeartbeat(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)
	inst := newTestInstance(svc.ServiceId)
	s.CreateInstance(inst, time.Now().Unix()+90)

	newDeadline := time.Now().Unix() + 180
	if err := s.UpdateInstanceHeartbeat(svc.ServiceId, inst.InstanceId, newDeadline); err != nil {
		t.Fatalf("UpdateInstanceHeartbeat: %v", err)
	}

	// verify instance still exists after original lease expired
	time.Sleep(10 * time.Millisecond)
	expired, _ := s.GetExpiredInstances(time.Now().Unix() + 100)
	for _, e := range expired {
		if e.InstanceId == inst.InstanceId {
			t.Fatal("instance should not be expired after heartbeat")
		}
	}
}

func TestStoreGetExpiredInstances(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)

	inst := newTestInstance(svc.ServiceId)
	s.CreateInstance(inst, time.Now().Unix()-1)

	expired, err := s.GetExpiredInstances(time.Now().Unix())
	if err != nil {
		t.Fatalf("GetExpiredInstances: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expected 1 expired instance, got %d", len(expired))
	}
	if expired[0].InstanceId != inst.InstanceId {
		t.Errorf("expected instance %s, got %s", inst.InstanceId, expired[0].InstanceId)
	}

	remaining, _ := s.ListInstance(svc.ServiceId)
	if len(remaining) != 0 {
		t.Error("expired instance should be removed from store")
	}
}

func TestStoreSchemaCRUD(t *testing.T) {
	s := newMemStore()
	sch := &Schema{SchemaId: "test-schema", Schema: `{"type":"object"}`, Summary: "test summary"}

	if err := s.CreateSchema("svc-1", "test-schema", sch); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	got, err := s.GetSchema("svc-1", "test-schema")
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if got.SchemaId != "test-schema" {
		t.Errorf("expected schemaId test-schema, got %s", got.SchemaId)
	}

	if err := s.DeleteSchema("svc-1", "test-schema"); err != nil {
		t.Fatalf("DeleteSchema: %v", err)
	}
	if _, err := s.GetSchema("svc-1", "test-schema"); err != ErrSchemaNotExists {
		t.Error("schema should not exist after delete")
	}
}

func TestStoreTagsCRUD(t *testing.T) {
	s := newMemStore()
	if err := s.CreateTags("svc-1", map[string]string{"env": "prod", "version": "1"}); err != nil {
		t.Fatalf("CreateTags: %v", err)
	}

	tags, err := s.GetTags("svc-1")
	if err != nil {
		t.Fatalf("GetTags: %v", err)
	}
	if tags["env"] != "prod" {
		t.Errorf("expected env=prod, got %s", tags["env"])
	}

	if err := s.UpdateTag("svc-1", "env", "staging"); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}
	tags, _ = s.GetTags("svc-1")
	if tags["env"] != "staging" {
		t.Errorf("expected env=staging, got %s", tags["env"])
	}

	if err := s.DeleteTags("svc-1", []string{"env"}); err != nil {
		t.Fatalf("DeleteTags: %v", err)
	}
	tags, _ = s.GetTags("svc-1")
	if _, exists := tags["env"]; exists {
		t.Error("env tag should be deleted")
	}
}

func TestStoreUpdateInstanceStatus(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)
	inst := newTestInstance(svc.ServiceId)
	s.CreateInstance(inst, time.Now().Unix()+90)

	if err := s.UpdateInstanceStatus(svc.ServiceId, inst.InstanceId, MSI_DOWN); err != nil {
		t.Fatalf("UpdateInstanceStatus: %v", err)
	}

	got, _ := s.GetInstance(svc.ServiceId, inst.InstanceId)
	if got.Status != MSI_DOWN {
		t.Errorf("expected status %s, got %s", MSI_DOWN, got.Status)
	}
}

func TestStoreUpdateServiceProperties(t *testing.T) {
	s := newMemStore()
	svc := newTestService()
	s.CreateService(svc)

	if err := s.UpdateServiceProperties(svc.ServiceId, map[string]string{"key": "value"}); err != nil {
		t.Fatalf("UpdateServiceProperties: %v", err)
	}

	got, _ := s.GetService(svc.ServiceId)
	if got.Properties["key"] != "value" {
		t.Errorf("expected property key=value, got %s", got.Properties["key"])
	}
}

func TestStoreFindServiceIdByServiceName(t *testing.T) {
	s := newMemStore()
	svc1 := newTestService()
	svc2 := newTestService()
	svc2.ServiceName = "test-service"
	svc2.Version = "2.0.0"
	s.CreateService(svc1)
	s.CreateService(svc2)

	ids, err := s.FindServiceIdByServiceName("test-app", "test-service")
	if err != nil {
		t.Fatalf("FindServiceIdByServiceName: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
}
