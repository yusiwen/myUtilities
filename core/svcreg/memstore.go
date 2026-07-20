package svcreg

import (
	"sync"
)

type memStore struct {
	mu            sync.RWMutex
	services      map[string]*MicroService
	serviceIndex  map[string]string
	instances     map[string]*instanceRow
	schemas       map[string]*Schema
	tags          map[string]map[string]string
}

func newMemStore() *memStore {
	return &memStore{
		services:     make(map[string]*MicroService),
		serviceIndex: make(map[string]string),
		instances:    make(map[string]*instanceRow),
		schemas:      make(map[string]*Schema),
		tags:         make(map[string]map[string]string),
	}
}

func (s *memStore) Close() error { return nil }

func (s *memStore) CreateService(svc *MicroService) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.services[svc.ServiceId]; exists {
		return ErrServiceAlreadyExists
	}
	svc.Timestamp = now()
	s.services[svc.ServiceId] = svc
	s.serviceIndex[serviceIndexKey(svc.AppId, svc.ServiceName, svc.Version, svc.Environment)] = svc.ServiceId
	return nil
}

func (s *memStore) GetService(serviceId string) (*MicroService, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	svc, ok := s.services[serviceId]
	if !ok {
		return nil, ErrServiceNotExists
	}
	return svc, nil
}

func (s *memStore) ListService() ([]*MicroService, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*MicroService
	for _, svc := range s.services {
		result = append(result, svc)
	}
	return result, nil
}

func (s *memStore) UpdateServiceProperties(serviceId string, props map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	svc, ok := s.services[serviceId]
	if !ok {
		return ErrServiceNotExists
	}
	if svc.Properties == nil {
		svc.Properties = make(map[string]string)
	}
	for k, v := range props {
		svc.Properties[k] = v
	}
	svc.ModTimestamp = now()
	return nil
}

func (s *memStore) DeleteService(serviceId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	svc, ok := s.services[serviceId]
	if !ok {
		return ErrServiceNotExists
	}
	delete(s.services, serviceId)
	delete(s.serviceIndex, serviceIndexKey(svc.AppId, svc.ServiceName, svc.Version, svc.Environment))
	for key, row := range s.instances {
		if row.Instance.ServiceId == serviceId {
			delete(s.instances, key)
		}
	}
	for key := range s.schemas {
		if key[:len(serviceId)] == serviceId {
			delete(s.schemas, key)
		}
	}
	delete(s.tags, serviceId)
	return nil
}

func (s *memStore) FindServiceId(appId, serviceName, version, env string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.serviceIndex[serviceIndexKey(appId, serviceName, version, env)]
	if !ok {
		return "", ErrServiceNotExists
	}
	return id, nil
}

func (s *memStore) FindServiceIdByServiceName(appId, serviceName string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var ids []string
	for _, svc := range s.services {
		if svc.AppId == appId && svc.ServiceName == serviceName {
			ids = append(ids, svc.ServiceId)
		}
	}
	return ids, nil
}

func (s *memStore) CreateInstance(inst *MicroServiceInstance, leaseDeadline int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(instanceKey(inst.ServiceId, inst.InstanceId))
	if _, exists := s.instances[key]; exists {
		return nil
	}
	inst.Timestamp = now()
	s.instances[key] = &instanceRow{Instance: inst, LeaseDeadline: leaseDeadline}
	return nil
}

func (s *memStore) GetInstance(serviceId, instanceId string) (*MicroServiceInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.instances[string(instanceKey(serviceId, instanceId))]
	if !ok {
		return nil, ErrInstanceNotExists
	}
	return row.Instance, nil
}

func (s *memStore) ListInstance(serviceId string) ([]*MicroServiceInstance, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*MicroServiceInstance
	for _, row := range s.instances {
		if row.Instance.ServiceId == serviceId {
			result = append(result, row.Instance)
		}
	}
	return result, nil
}

func (s *memStore) DeleteInstance(serviceId, instanceId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(instanceKey(serviceId, instanceId))
	if _, ok := s.instances[key]; !ok {
		return ErrInstanceNotExists
	}
	delete(s.instances, key)
	return nil
}

func (s *memStore) UpdateInstanceProperties(serviceId, instanceId string, props map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.instances[string(instanceKey(serviceId, instanceId))]
	if !ok {
		return ErrInstanceNotExists
	}
	if row.Instance.Properties == nil {
		row.Instance.Properties = make(map[string]string)
	}
	for k, v := range props {
		row.Instance.Properties[k] = v
	}
	row.Instance.ModTimestamp = now()
	return nil
}

func (s *memStore) UpdateInstanceStatus(serviceId, instanceId, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.instances[string(instanceKey(serviceId, instanceId))]
	if !ok {
		return ErrInstanceNotExists
	}
	row.Instance.Status = status
	row.Instance.ModTimestamp = now()
	return nil
}

func (s *memStore) UpdateInstanceHeartbeat(serviceId, instanceId string, leaseDeadline int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, ok := s.instances[string(instanceKey(serviceId, instanceId))]
	if !ok {
		return ErrInstanceNotExists
	}
	row.LeaseDeadline = leaseDeadline
	return nil
}

func (s *memStore) GetExpiredInstances(deadline int64) ([]*MicroServiceInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []*MicroServiceInstance
	for key, row := range s.instances {
		if row.LeaseDeadline > 0 && row.LeaseDeadline < deadline {
			expired = append(expired, row.Instance)
			delete(s.instances, key)
		}
	}
	return expired, nil
}

func (s *memStore) CreateSchema(serviceId, schemaId string, schema *Schema) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(schemaKey(serviceId, schemaId))
	if _, exists := s.schemas[key]; exists {
		return nil
	}
	s.schemas[key] = schema
	return nil
}

func (s *memStore) GetSchema(serviceId, schemaId string) (*Schema, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sch, ok := s.schemas[string(schemaKey(serviceId, schemaId))]
	if !ok {
		return nil, ErrSchemaNotExists
	}
	return sch, nil
}

func (s *memStore) DeleteSchema(serviceId, schemaId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := string(schemaKey(serviceId, schemaId))
	if _, ok := s.schemas[key]; !ok {
		return ErrSchemaNotExists
	}
	delete(s.schemas, key)
	return nil
}

func (s *memStore) ListSchema(serviceId string) ([]*Schema, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*Schema
	for key, sch := range s.schemas {
		if len(key) > len(serviceId) && key[:len(serviceId)] == serviceId && key[len(serviceId):len(serviceId)+1] == "/" {
			result = append(result, sch)
		}
	}
	return result, nil
}

func (s *memStore) CreateTags(serviceId string, tags map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.tags[serviceId]
	if !ok {
		existing = make(map[string]string)
	}
	for k, v := range tags {
		existing[k] = v
	}
	s.tags[serviceId] = existing
	return nil
}

func (s *memStore) GetTags(serviceId string) (map[string]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tags, ok := s.tags[serviceId]
	if !ok {
		return nil, ErrTagNotExists
	}
	return tags, nil
}

func (s *memStore) UpdateTag(serviceId, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags, ok := s.tags[serviceId]
	if !ok {
		return ErrTagNotExists
	}
	tags[key] = value
	return nil
}

func (s *memStore) DeleteTags(serviceId string, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tags, ok := s.tags[serviceId]
	if !ok {
		return ErrTagNotExists
	}
	for _, k := range keys {
		delete(tags, k)
	}
	return nil
}
