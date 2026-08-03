package svcreg

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrServiceAlreadyExists = &Error{Code: 400010, Message: "micro-service already exists"}
	ErrServiceNotExists     = &Error{Code: 400012, Message: "service does not exist"}
	ErrInstanceNotExists    = &Error{Code: 400017, Message: "instance does not exist"}
	ErrInvalidParams        = &Error{Code: 400001, Message: "parameter is invalid"}
	ErrInternal             = &Error{Code: 500003, Message: "internal error"}
	ErrSchemaNotExists      = &Error{Code: 400016, Message: "schema does not exist"}
	ErrTagNotExists         = &Error{Code: 400018, Message: "tag does not exist"}
	ErrNotEnoughQuota       = &Error{Code: 400100, Message: "not enough quota"}
)

type Error struct {
	Code    int32  `json:"errorCode"`
	Message string `json:"errorMessage"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func NewError(code int32, msg string) *Error {
	return &Error{Code: code, Message: msg}
}

const (
	ResponseSuccess = 0

	MS_UP   = "UP"
	MS_DOWN = "DOWN"

	MSI_UP           = "UP"
	MSI_DOWN         = "DOWN"
	MSI_STARTING     = "STARTING"
	MSI_TESTING      = "TESTING"
	MSI_OUTOFSERVICE = "OUTOFSERVICE"

	ENV_DEV    = "development"
	ENV_TEST   = "testing"
	ENV_ACCEPT = "acceptance"
	ENV_PROD   = "production"

	REGISTERBY_SDK      = "SDK"
	REGISTERBY_SIDECAR  = "SIDECAR"
	REGISTERBY_PLATFORM = "PLATFORM"

	CHECK_BY_HEARTBEAT = "push"
	CHECK_BY_PLATFORM  = "pull"

	EVT_CREATE = "CREATE"
	EVT_UPDATE = "UPDATE"
	EVT_DELETE = "DELETE"
	EVT_EXPIRE = "EXPIRE"

	ExistTypeMicroservice = "microservice"
	ExistTypeSchema       = "schema"

	AppID   = "default"
	Version = "0.0.1"

	DefaultLeaseInterval = 30
	DefaultLeaseTimes    = 3
	LeaseScanInterval    = 10 * time.Second
)

func NewUUID() string {
	return uuid.New().String()
}

type Response struct {
	Code    int32  `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

func SuccessResponse() *Response {
	return &Response{Code: ResponseSuccess}
}

type MicroService struct {
	ServiceId    string             `json:"serviceId,omitempty"`
	AppId        string             `json:"appId,omitempty"`
	ServiceName  string             `json:"serviceName,omitempty"`
	Version      string             `json:"version,omitempty"`
	Description  string             `json:"description,omitempty"`
	Level        string             `json:"level,omitempty"`
	Schemas      []string           `json:"schemas,omitempty"`
	Paths        []*ServicePath     `json:"paths,omitempty"`
	Status       string             `json:"status,omitempty"`
	Properties   map[string]string  `json:"properties,omitempty"`
	Timestamp    string             `json:"timestamp,omitempty"`
	Providers    []*MicroServiceKey `json:"providers,omitempty"`
	Alias        string             `json:"alias,omitempty"`
	LBStrategy   map[string]string  `json:"LBStrategy,omitempty"`
	ModTimestamp string             `json:"modTimestamp,omitempty"`
	Environment  string             `json:"environment,omitempty"`
	RegisterBy   string             `json:"registerBy,omitempty"`
	Framework    *FrameWork         `json:"framework,omitempty"`
}

type MicroServiceKey struct {
	Tenant      string `json:"tenant,omitempty"`
	Environment string `json:"environment,omitempty"`
	AppId       string `json:"appId,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Alias       string `json:"alias,omitempty"`
	Version     string `json:"version,omitempty"`
}

type ServicePath struct {
	Path     string            `json:"path,omitempty"`
	Property map[string]string `json:"property,omitempty"`
}

type FrameWork struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type MicroServiceInstance struct {
	InstanceId     string            `json:"instanceId,omitempty"`
	ServiceId      string            `json:"serviceId,omitempty"`
	Endpoints      []string          `json:"endpoints,omitempty"`
	HostName       string            `json:"hostName,omitempty"`
	Status         string            `json:"status,omitempty"`
	Properties     map[string]string `json:"properties,omitempty"`
	HealthCheck    *HealthCheck      `json:"healthCheck,omitempty"`
	Timestamp      string            `json:"timestamp,omitempty"`
	DataCenterInfo *DataCenterInfo   `json:"dataCenterInfo,omitempty"`
	ModTimestamp   string            `json:"modTimestamp,omitempty"`
	Version        string            `json:"version,omitempty"`
}

type HealthCheck struct {
	Mode     string `json:"mode,omitempty"`
	Port     int32  `json:"port,omitempty"`
	Interval int32  `json:"interval,omitempty"`
	Times    int32  `json:"times,omitempty"`
	Url      string `json:"url,omitempty"`
}

type DataCenterInfo struct {
	Name          string `json:"name,omitempty"`
	Region        string `json:"region,omitempty"`
	AvailableZone string `json:"availableZone,omitempty"`
}

type HeartbeatRequest struct {
	ServiceId  string `json:"serviceId,omitempty"`
	InstanceId string `json:"instanceId,omitempty"`
}

type HeartbeatSetElement struct {
	ServiceId  string `json:"serviceId,omitempty"`
	InstanceId string `json:"instanceId,omitempty"`
}

type HeartbeatSetRequest struct {
	Instances []*HeartbeatSetElement `json:"instances,omitempty"`
}

type InstanceHbRst struct {
	ServiceId  string `json:"serviceId,omitempty"`
	InstanceId string `json:"instanceId,omitempty"`
	ErrMessage string `json:"errMessage,omitempty"`
}

type HeartbeatSetResponse struct {
	Response  *Response        `json:"-"`
	Instances []*InstanceHbRst `json:"instances,omitempty"`
}

type CreateServiceRequest struct {
	Service   *MicroService             `json:"service,omitempty"`
	Rules     []*AddOrUpdateServiceRule `json:"rules,omitempty"`
	Tags      map[string]string         `json:"tags,omitempty"`
	Instances []*MicroServiceInstance   `json:"instances,omitempty"`
}

type CreateServiceResponse struct {
	Response  *Response `json:"-"`
	ServiceId string    `json:"serviceId,omitempty"`
}

type GetServiceResponse struct {
	Response *Response     `json:"-"`
	Service  *MicroService `json:"service,omitempty"`
}

type GetServicesResponse struct {
	Response *Response       `json:"-"`
	Services []*MicroService `json:"services,omitempty"`
}

type GetExistenceResponse struct {
	Response  *Response `json:"-"`
	ServiceId string    `json:"serviceId,omitempty"`
	SchemaId  string    `json:"schemaId,omitempty"`
	Summary   string    `json:"summary,omitempty"`
}

type UpdateServicePropsRequest struct {
	ServiceId  string            `json:"serviceId,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type DeleteServiceRequest struct {
	ServiceId string `json:"serviceId,omitempty"`
	Force     bool   `json:"force,omitempty"`
}

type GetServiceRequest struct {
	ServiceId string `json:"serviceId,omitempty"`
}

type RegisterInstanceRequest struct {
	Instance *MicroServiceInstance `json:"instance,omitempty"`
}

type RegisterInstanceResponse struct {
	Response   *Response `json:"-"`
	InstanceId string    `json:"instanceId,omitempty"`
}

type UnregisterInstanceRequest struct {
	ServiceId  string `json:"serviceId,omitempty"`
	InstanceId string `json:"instanceId,omitempty"`
}

type FindInstancesRequest struct {
	ConsumerServiceId string   `json:"consumerServiceId,omitempty"`
	AppId             string   `json:"appId,omitempty"`
	ServiceName       string   `json:"serviceName,omitempty"`
	VersionRule       string   `json:"versionRule,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	Environment       string   `json:"environment,omitempty"`
	Alias             string   `json:"alias,omitempty"`
}

type FindInstancesResponse struct {
	Response  *Response               `json:"-"`
	Instances []*MicroServiceInstance `json:"instances,omitempty"`
}

type GetOneInstanceResponse struct {
	Response *Response             `json:"-"`
	Instance *MicroServiceInstance `json:"instance,omitempty"`
}

type GetInstancesResponse struct {
	Response  *Response               `json:"-"`
	Instances []*MicroServiceInstance `json:"instances,omitempty"`
}

type UpdateInstanceStatusRequest struct {
	ServiceId  string `json:"serviceId,omitempty"`
	InstanceId string `json:"instanceId,omitempty"`
	Status     string `json:"status,omitempty"`
}

type UpdateInstancePropsRequest struct {
	ServiceId  string            `json:"serviceId,omitempty"`
	InstanceId string            `json:"instanceId,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

type AddOrUpdateServiceRule struct {
	RuleType    string `json:"ruleType,omitempty"`
	Attribute   string `json:"attribute,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	Description string `json:"description,omitempty"`
}

type Schema struct {
	SchemaId string `json:"schemaId,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Schema   string `json:"schema,omitempty"`
}

type ModifySchemaRequest struct {
	ServiceId string `json:"serviceId,omitempty"`
	SchemaId  string `json:"schemaId,omitempty"`
	Schema    string `json:"schema,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type GetSchemaResponse struct {
	Response      *Response `json:"-"`
	Schema        string    `json:"schema,omitempty"`
	SchemaSummary string    `json:"schemaSummary,omitempty"`
}

type GetAllSchemaResponse struct {
	Response *Response `json:"-"`
	Schemas  []*Schema `json:"schemas,omitempty"`
}

type ModifySchemasRequest struct {
	ServiceId string    `json:"serviceId,omitempty"`
	Schemas   []*Schema `json:"schemas,omitempty"`
}

type GetServiceTagsResponse struct {
	Response *Response         `json:"-"`
	Tags     map[string]string `json:"tags,omitempty"`
}

type AddServiceTagsRequest struct {
	ServiceId string            `json:"serviceId,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
}

type UpdateServiceTagRequest struct {
	ServiceId string `json:"serviceId,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
}

type DeleteServiceTagsRequest struct {
	ServiceId string   `json:"serviceId,omitempty"`
	Keys      []string `json:"keys,omitempty"`
}

type WatchInstanceResponse struct {
	Response *Response             `json:"-"`
	Action   string                `json:"action,omitempty"`
	Key      *MicroServiceKey      `json:"key,omitempty"`
	Instance *MicroServiceInstance `json:"instance,omitempty"`
}

type WatchInstanceRequest struct {
	SelfServiceId string `json:"selfServiceId,omitempty"`
}

type MicroServiceInstanceKey struct {
	ServiceId  string `json:"serviceId,omitempty"`
	InstanceId string `json:"instanceId,omitempty"`
}

type GetExistenceRequest struct {
	Type        string `json:"type,omitempty"`
	AppId       string `json:"appId,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	Version     string `json:"version,omitempty"`
	ServiceId   string `json:"serviceId,omitempty"`
	SchemaId    string `json:"schemaId,omitempty"`
	Environment string `json:"environment,omitempty"`
}

type GetServicesRequest struct {
	WithShared bool `json:"withShared"`
}

type ClusterHealthResponse struct {
	Response *Response `json:"-"`
}

type BatchFindInstancesRequest struct {
	ConsumerServiceId string          `json:"consumerServiceId,omitempty"`
	Services          []*FindService  `json:"services,omitempty"`
	Instances         []*FindInstance `json:"instances,omitempty"`
}

type BatchFindInstancesResponse struct {
	Response  *Response        `json:"-"`
	Services  *BatchFindResult `json:"services,omitempty"`
	Instances *BatchFindResult `json:"instances,omitempty"`
}

type FindService struct {
	Service *MicroServiceKey `json:"service"`
	Rev     string           `json:"rev,omitempty"`
}

type FindInstance struct {
	Instance *HeartbeatSetElement `json:"instance"`
	Rev      string               `json:"rev,omitempty"`
}

type BatchFindResult struct {
	Failed      []*FindFailedResult `json:"failed,omitempty"`
	NotModified []int64             `json:"notModified,omitempty"`
	Updated     []*FindResult       `json:"updated,omitempty"`
}

type FindResult struct {
	Index     int64                   `json:"index"`
	Rev       string                  `json:"rev"`
	Instances []*MicroServiceInstance `json:"instances,omitempty"`
}

type FindFailedResult struct {
	Indexes []int64   `json:"indexes"`
	Error   *RawError `json:"error"`
}

type RawError struct {
	Code    int32  `json:"errorCode,string"`
	Message string `json:"errorMessage"`
	Detail  string `json:"detail,omitempty"`
}
