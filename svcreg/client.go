package svcreg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	coresv "github.com/yusiwen/myUtilities/core/svcreg"
)

type Client struct {
	Server string
}

func (c *Client) url(path string) string {
	return strings.TrimRight(c.Server, "/") + path
}

func (c *Client) get(path string, v interface{}) error {
	resp, err := http.Get(c.url(path))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		msg, _ := errResp["errorMessage"].(string)
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("server error: %s", msg)
	}
	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}
	return nil
}

func (c *Client) GetVersion() (map[string]interface{}, error) {
	var v map[string]interface{}
	err := c.get("/v4/default/registry/version", &v)
	return v, err
}

func (c *Client) GetServices() ([]*coresv.MicroService, error) {
	var resp coresv.GetServicesResponse
	if err := c.get("/v4/default/registry/microservices", &resp); err != nil {
		return nil, err
	}
	if resp.Services == nil {
		return []*coresv.MicroService{}, nil
	}
	return resp.Services, nil
}

func (c *Client) GetService(serviceId string) (*coresv.MicroService, error) {
	var resp coresv.GetServiceResponse
	if err := c.get("/v4/default/registry/microservices/"+serviceId, &resp); err != nil {
		return nil, err
	}
	return resp.Service, nil
}

func (c *Client) GetInstances(serviceId string) ([]*coresv.MicroServiceInstance, error) {
	var resp coresv.GetInstancesResponse
	if err := c.get("/v4/default/registry/microservices/"+serviceId+"/instances", &resp); err != nil {
		return nil, err
	}
	if resp.Instances == nil {
		return []*coresv.MicroServiceInstance{}, nil
	}
	return resp.Instances, nil
}

func (c *Client) Status() (map[string]interface{}, int, int, int, error) {
	ver, err := c.GetVersion()
	if err != nil {
		return nil, 0, 0, 0, err
	}
	services, err := c.GetServices()
	if err != nil {
		return nil, 0, 0, 0, err
	}
	upCount := 0
	instCount := 0
	for _, svc := range services {
		if svc.Status == "UP" {
			upCount++
		}
		insts, err := c.GetInstances(svc.ServiceId)
		if err != nil {
			continue
		}
		instCount += len(insts)
	}
	return ver, len(services), upCount, instCount, nil
}
