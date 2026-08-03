package svcreg

import (
	"encoding/json"
	"net/http"
	"strings"
)

type instanceWithSvc struct {
	*MicroServiceInstance
	ServiceName string `json:"serviceName"`
	AppId       string `json:"appId"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
}

// RegisterProxyAPI registers the frontend-facing proxy endpoints.
func RegisterProxyAPI(mux *http.ServeMux, client *Client) {
	mux.HandleFunc("/api/svcreg/services", func(w http.ResponseWriter, r *http.Request) {
		services, err := client.GetServices()
		if err != nil {
			WriteProxyError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services)
	})
	mux.HandleFunc("/api/svcreg/services/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/svcreg/services/")
		svc, err := client.GetService(id)
		if err != nil {
			WriteProxyError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(svc)
	})
	mux.HandleFunc("/api/svcreg/instances", func(w http.ResponseWriter, r *http.Request) {
		serviceId := r.URL.Query().Get("serviceId")
		w.Header().Set("Content-Type", "application/json")
		if serviceId != "" {
			insts, err := client.GetInstances(serviceId)
			if err != nil {
				WriteProxyError(w, err)
				return
			}
			json.NewEncoder(w).Encode(insts)
			return
		}
		services, err := client.GetServices()
		if err != nil {
			WriteProxyError(w, err)
			return
		}
		all := []instanceWithSvc{}
		for _, svc := range services {
			insts, err := client.GetInstances(svc.ServiceId)
			if err != nil {
				continue
			}
			for _, inst := range insts {
				all = append(all, instanceWithSvc{
					MicroServiceInstance: inst,
					ServiceName:          svc.ServiceName,
					AppId:                svc.AppId,
					Version:              svc.Version,
					Environment:          svc.Environment,
				})
			}
		}
		json.NewEncoder(w).Encode(all)
	})
	mux.HandleFunc("/api/svcreg/status", func(w http.ResponseWriter, r *http.Request) {
		ver, svcCount, upCount, instCount, err := client.Status()
		if err != nil {
			WriteProxyError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"version":       ver,
			"serviceCount":  svcCount,
			"onlineCount":   upCount,
			"instanceCount": instCount,
		})
	})
	RegisterAdminAPI(mux, client)
}

// WriteProxyError writes a proxy error response.
func WriteProxyError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
