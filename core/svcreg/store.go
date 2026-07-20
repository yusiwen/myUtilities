package svcreg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "github.com/coreos/bbolt"
)

type instanceRow struct {
	Instance      *MicroServiceInstance `json:"instance"`
	LeaseDeadline int64                 `json:"leaseDeadline"`
}

type Store interface {
	CreateService(svc *MicroService) error
	GetService(serviceId string) (*MicroService, error)
	ListService() ([]*MicroService, error)
	UpdateServiceProperties(serviceId string, props map[string]string) error
	DeleteService(serviceId string) error
	FindServiceId(appId, serviceName, version, env string) (string, error)
	FindServiceIdByServiceName(appId, serviceName string) ([]string, error)

	CreateInstance(inst *MicroServiceInstance, leaseDeadline int64) error
	GetInstance(serviceId, instanceId string) (*MicroServiceInstance, error)
	ListInstance(serviceId string) ([]*MicroServiceInstance, error)
	DeleteInstance(serviceId, instanceId string) error
	UpdateInstanceProperties(serviceId, instanceId string, props map[string]string) error
	UpdateInstanceStatus(serviceId, instanceId, status string) error
	UpdateInstanceHeartbeat(serviceId, instanceId string, leaseDeadline int64) error
	GetExpiredInstances(deadline int64) ([]*MicroServiceInstance, error)

	CreateSchema(serviceId, schemaId string, schema *Schema) error
	GetSchema(serviceId, schemaId string) (*Schema, error)
	DeleteSchema(serviceId, schemaId string) error
	ListSchema(serviceId string) ([]*Schema, error)

	CreateTags(serviceId string, tags map[string]string) error
	GetTags(serviceId string) (map[string]string, error)
	UpdateTag(serviceId, key, value string) error
	DeleteTags(serviceId string, keys []string) error

	Close() error
}

const (
	bucketServices     = "services"
	bucketServiceIndex = "serviceIndex"
	bucketInstances    = "instances"
	bucketSchemas      = "schemas"
	bucketTags         = "tags"
)

type boltStore struct {
	mu *sync.Mutex
	db *bolt.DB
}

func NewBoltStore(dbPath string) (Store, error) {
	if strings.HasPrefix(dbPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %v", err)
		}
		dbPath = filepath.Join(home, dbPath[2:])
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}
	db, err := bolt.Open(dbPath, 0660, nil)
	if err != nil {
		return nil, err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{bucketServices, bucketServiceIndex, bucketInstances, bucketSchemas, bucketTags} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}
	return &boltStore{mu: &sync.Mutex{}, db: db}, nil
}

func (s *boltStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

func (s *boltStore) CreateService(svc *MicroService) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketServices))
		if b.Get([]byte(svc.ServiceId)) != nil {
			return ErrServiceAlreadyExists
		}
		data, err := json.Marshal(svc)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(svc.ServiceId), data); err != nil {
			return err
		}
		idx := tx.Bucket([]byte(bucketServiceIndex))
		key := serviceIndexKey(svc.AppId, svc.ServiceName, svc.Version, svc.Environment)
		return idx.Put([]byte(key), []byte(svc.ServiceId))
	})
}

func (s *boltStore) GetService(serviceId string) (*MicroService, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var svc *MicroService
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketServices))
		data := b.Get([]byte(serviceId))
		if data == nil {
			return ErrServiceNotExists
		}
		return json.Unmarshal(data, &svc)
	})
	return svc, err
}

func (s *boltStore) ListService() ([]*MicroService, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var services []*MicroService
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketServices))
		return b.ForEach(func(k, v []byte) error {
			var svc MicroService
			if err := json.Unmarshal(v, &svc); err != nil {
				return err
			}
			services = append(services, &svc)
			return nil
		})
	})
	return services, err
}

func (s *boltStore) UpdateServiceProperties(serviceId string, props map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketServices))
		data := b.Get([]byte(serviceId))
		if data == nil {
			return ErrServiceNotExists
		}
		var svc MicroService
		if err := json.Unmarshal(data, &svc); err != nil {
			return err
		}
		if svc.Properties == nil {
			svc.Properties = make(map[string]string)
		}
		for k, v := range props {
			svc.Properties[k] = v
		}
		svc.ModTimestamp = now()
		newData, err := json.Marshal(&svc)
		if err != nil {
			return err
		}
		return b.Put([]byte(serviceId), newData)
	})
}

func (s *boltStore) DeleteService(serviceId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketServices))
		data := b.Get([]byte(serviceId))
		if data == nil {
			return ErrServiceNotExists
		}
		var svc MicroService
		if err := json.Unmarshal(data, &svc); err != nil {
			return err
		}
		if err := b.Delete([]byte(serviceId)); err != nil {
			return err
		}
		idx := tx.Bucket([]byte(bucketServiceIndex))
		key := serviceIndexKey(svc.AppId, svc.ServiceName, svc.Version, svc.Environment)
		idx.Delete([]byte(key))

		inst := tx.Bucket([]byte(bucketInstances))
		c := inst.Cursor()
		prefix := []byte(serviceId + "/")
		for k, _ := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, _ = c.Next() {
			if err := inst.Delete(k); err != nil {
				return err
			}
		}
		schemas := tx.Bucket([]byte(bucketSchemas))
		c2 := schemas.Cursor()
		prefix2 := []byte(serviceId + "/")
		for k, _ := c2.Seek(prefix2); k != nil && strings.HasPrefix(string(k), string(prefix2)); k, _ = c2.Next() {
			if err := schemas.Delete(k); err != nil {
				return err
			}
		}
		tags := tx.Bucket([]byte(bucketTags))
		tags.Delete([]byte(serviceId))
		return nil
	})
}

func (s *boltStore) FindServiceId(appId, serviceName, version, env string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var serviceId string
	err := s.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket([]byte(bucketServiceIndex))
		key := serviceIndexKey(appId, serviceName, version, env)
		data := idx.Get([]byte(key))
		if data == nil {
			return ErrServiceNotExists
		}
		serviceId = string(data)
		return nil
	})
	return serviceId, err
}

func (s *boltStore) FindServiceIdByServiceName(appId, serviceName string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketServices))
		return b.ForEach(func(k, v []byte) error {
			var svc MicroService
			if err := json.Unmarshal(v, &svc); err != nil {
				return err
			}
			if svc.AppId == appId && svc.ServiceName == serviceName {
				ids = append(ids, svc.ServiceId)
			}
			return nil
		})
	})
	return ids, err
}

func (s *boltStore) CreateInstance(inst *MicroServiceInstance, leaseDeadline int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		key := instanceKey(inst.ServiceId, inst.InstanceId)
		if b.Get(key) != nil {
			return nil
		}
		row := &instanceRow{Instance: inst, LeaseDeadline: leaseDeadline}
		data, err := json.Marshal(row)
		if err != nil {
			return err
		}
		return b.Put(key, data)
	})
}

func (s *boltStore) GetInstance(serviceId, instanceId string) (*MicroServiceInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var inst *MicroServiceInstance
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		data := b.Get(instanceKey(serviceId, instanceId))
		if data == nil {
			return ErrInstanceNotExists
		}
		var row instanceRow
		if err := json.Unmarshal(data, &row); err != nil {
			return err
		}
		inst = row.Instance
		return nil
	})
	return inst, err
}

func (s *boltStore) ListInstance(serviceId string) ([]*MicroServiceInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var instances []*MicroServiceInstance
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		c := b.Cursor()
		prefix := []byte(serviceId + "/")
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var row instanceRow
			if err := json.Unmarshal(v, &row); err != nil {
				return err
			}
			instances = append(instances, row.Instance)
		}
		return nil
	})
	return instances, err
}

func (s *boltStore) DeleteInstance(serviceId, instanceId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		key := instanceKey(serviceId, instanceId)
		if b.Get(key) == nil {
			return ErrInstanceNotExists
		}
		return b.Delete(key)
	})
}

func (s *boltStore) UpdateInstanceProperties(serviceId, instanceId string, props map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		data := b.Get(instanceKey(serviceId, instanceId))
		if data == nil {
			return ErrInstanceNotExists
		}
		var row instanceRow
		if err := json.Unmarshal(data, &row); err != nil {
			return err
		}
		if row.Instance.Properties == nil {
			row.Instance.Properties = make(map[string]string)
		}
		for k, v := range props {
			row.Instance.Properties[k] = v
		}
		row.Instance.ModTimestamp = now()
		newData, err := json.Marshal(&row)
		if err != nil {
			return err
		}
		return b.Put(instanceKey(serviceId, instanceId), newData)
	})
}

func (s *boltStore) UpdateInstanceStatus(serviceId, instanceId, status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		data := b.Get(instanceKey(serviceId, instanceId))
		if data == nil {
			return ErrInstanceNotExists
		}
		var row instanceRow
		if err := json.Unmarshal(data, &row); err != nil {
			return err
		}
		row.Instance.Status = status
		row.Instance.ModTimestamp = now()
		newData, err := json.Marshal(&row)
		if err != nil {
			return err
		}
		return b.Put(instanceKey(serviceId, instanceId), newData)
	})
}

func (s *boltStore) UpdateInstanceHeartbeat(serviceId, instanceId string, leaseDeadline int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		key := instanceKey(serviceId, instanceId)
		data := b.Get(key)
		if data == nil {
			return ErrInstanceNotExists
		}
		var row instanceRow
		if err := json.Unmarshal(data, &row); err != nil {
			return err
		}
		row.LeaseDeadline = leaseDeadline
		newData, err := json.Marshal(&row)
		if err != nil {
			return err
		}
		return b.Put(key, newData)
	})
}

func (s *boltStore) GetExpiredInstances(deadline int64) ([]*MicroServiceInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var expired []*MicroServiceInstance
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketInstances))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var row instanceRow
			if err := json.Unmarshal(v, &row); err != nil {
				continue
			}
			if row.LeaseDeadline > 0 && row.LeaseDeadline < deadline {
				expired = append(expired, row.Instance)
				if err := b.Delete(k); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return expired, err
}

func (s *boltStore) CreateSchema(serviceId, schemaId string, schema *Schema) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSchemas))
		key := schemaKey(serviceId, schemaId)
		if b.Get(key) != nil {
			return nil
		}
		data, err := json.Marshal(schema)
		if err != nil {
			return err
		}
		return b.Put(key, data)
	})
}

func (s *boltStore) GetSchema(serviceId, schemaId string) (*Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sch *Schema
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSchemas))
		data := b.Get(schemaKey(serviceId, schemaId))
		if data == nil {
			return ErrSchemaNotExists
		}
		return json.Unmarshal(data, &sch)
	})
	return sch, err
}

func (s *boltStore) DeleteSchema(serviceId, schemaId string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSchemas))
		key := schemaKey(serviceId, schemaId)
		if b.Get(key) == nil {
			return ErrSchemaNotExists
		}
		return b.Delete(key)
	})
}

func (s *boltStore) ListSchema(serviceId string) ([]*Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var schemas []*Schema
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketSchemas))
		c := b.Cursor()
		prefix := []byte(serviceId + "/")
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var sch Schema
			if err := json.Unmarshal(v, &sch); err != nil {
				return err
			}
			schemas = append(schemas, &sch)
		}
		return nil
	})
	return schemas, err
}

func (s *boltStore) CreateTags(serviceId string, tags map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketTags))
		var existing map[string]string
		data := b.Get([]byte(serviceId))
		if data != nil {
			json.Unmarshal(data, &existing)
		}
		if existing == nil {
			existing = make(map[string]string)
		}
		for k, v := range tags {
			existing[k] = v
		}
		newData, err := json.Marshal(existing)
		if err != nil {
			return err
		}
		return b.Put([]byte(serviceId), newData)
	})
}

func (s *boltStore) GetTags(serviceId string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var tags map[string]string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketTags))
		data := b.Get([]byte(serviceId))
		if data == nil {
			return ErrTagNotExists
		}
		return json.Unmarshal(data, &tags)
	})
	return tags, err
}

func (s *boltStore) UpdateTag(serviceId, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketTags))
		data := b.Get([]byte(serviceId))
		if data == nil {
			return ErrTagNotExists
		}
		var tags map[string]string
		if err := json.Unmarshal(data, &tags); err != nil {
			return err
		}
		tags[key] = value
		newData, err := json.Marshal(tags)
		if err != nil {
			return err
		}
		return b.Put([]byte(serviceId), newData)
	})
}

func (s *boltStore) DeleteTags(serviceId string, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketTags))
		data := b.Get([]byte(serviceId))
		if data == nil {
			return ErrTagNotExists
		}
		var tags map[string]string
		if err := json.Unmarshal(data, &tags); err != nil {
			return err
		}
		for _, k := range keys {
			delete(tags, k)
		}
		newData, err := json.Marshal(tags)
		if err != nil {
			return err
		}
		return b.Put([]byte(serviceId), newData)
	})
}

func serviceIndexKey(appId, serviceName, version, env string) string {
	if env == "" {
		env = "default"
	}
	return fmt.Sprintf("%s/%s/%s/%s", appId, serviceName, version, env)
}

func instanceKey(serviceId, instanceId string) []byte {
	return []byte(serviceId + "/" + instanceId)
}

func schemaKey(serviceId, schemaId string) []byte {
	return []byte(serviceId + "/" + schemaId)
}

func now() string {
	return time.Now().Format(time.RFC3339)
}
