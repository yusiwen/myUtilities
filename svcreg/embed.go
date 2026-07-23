package svcreg

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"

	coresv "github.com/yusiwen/myUtilities/core/svcreg"
)

//go:embed frontend/dist
var frontendFS embed.FS

var mimeTypes = map[string]string{
	".js":    "application/javascript",
	".css":   "text/css",
	".html":  "text/html; charset=utf-8",
	".json":  "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".woff2": "font/woff2",
}

func FrontendHandler() http.Handler {
	subFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to get frontend sub filesystem: %v", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		data, err := fs.ReadFile(subFS, p)
		if err != nil {
			data, err = fs.ReadFile(subFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		if ct, ok := mimeTypes[path.Ext(p)]; ok {
			w.Header().Set("Content-Type", ct)
		}
		w.Write(data)
	})
}

type instanceWithSvc struct {
	*coresv.MicroServiceInstance
	ServiceName string `json:"serviceName"`
	AppId       string `json:"appId"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
}

func RegisterProxyAPI(mux *http.ServeMux, client *Client) {
	mux.HandleFunc("/api/svcreg/services", func(w http.ResponseWriter, r *http.Request) {
		services, err := client.GetServices()
		if err != nil {
			writeProxyError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services)
	})
	mux.HandleFunc("/api/svcreg/services/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/svcreg/services/")
		svc, err := client.GetService(id)
		if err != nil {
			writeProxyError(w, err)
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
				writeProxyError(w, err)
				return
			}
			json.NewEncoder(w).Encode(insts)
			return
		}
		services, err := client.GetServices()
		if err != nil {
			writeProxyError(w, err)
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
			writeProxyError(w, err)
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
	registerAdminAPI(mux, client)
}

func writeProxyError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
