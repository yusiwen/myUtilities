package svcreg

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/yusiwen/myUtilities/core/svcreg"
)

type Options struct {
	Serve  ServeOptions  `cmd:"" help:"Start the service registry server."`
	Status StatusOptions `cmd:"" help:"Show server status and statistics."`
	List   ListOptions   `cmd:"" help:"List resources."`
}

type ServeOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/svcreg-config.json"`
	Host   string `help:"Listen address."`
	Port   int    `help:"Override HTTP server port from config."`
	DBPath string `help:"Override BoltDB file path from config."`
}

func (o *ServeOptions) resolveConfig() {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		log.Printf("Warning: could not load config: %v", err)
		return
	}
	if o.Host == "" {
		o.Host = cfg.Host
	}
	if o.Port == 0 {
		o.Port = cfg.Port
	}
	if o.DBPath == "" {
		o.DBPath = cfg.DBPath
	}
}

func (o *ServeOptions) Run() error {
	o.resolveConfig()
	store, err := svcreg.NewBoltStore(o.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", o.Host, o.Port)
	server := svcreg.NewServer(addr, store)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		fmt.Println("\nshutting down...")
		return server.Shutdown(context.Background())
	}
}

type commonOpts struct {
	Server string `help:"Server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
}

type StatusOptions struct {
	Server string `help:"Server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
}

func (o *StatusOptions) Run() error {
	base := strings.TrimRight(o.Server, "/")
	// health
	healthResp, err := http.Get(base + "/v4/default/registry/health")
	if err != nil {
		return fmt.Errorf("connect to %s: %w", base, err)
	}
	healthResp.Body.Close()

	// version
	verResp, err := http.Get(base + "/v4/default/registry/version")
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}
	var ver map[string]interface{}
	json.NewDecoder(verResp.Body).Decode(&ver)
	verResp.Body.Close()

	// services
	svcResp, err := http.Get(base + "/v4/default/registry/microservices")
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	var services svcreg.GetServicesResponse
	json.NewDecoder(svcResp.Body).Decode(&services)
	svcResp.Body.Close()

	upCount := 0
	for _, s := range services.Services {
		if s.Status == "UP" {
			upCount++
		}
	}

	// count instances
	instCount := 0
	for _, s := range services.Services {
		instResp, err := http.Get(base + "/v4/default/registry/microservices/" + s.ServiceId + "/instances")
		if err != nil {
			continue
		}
		var insts svcreg.GetInstancesResponse
		json.NewDecoder(instResp.Body).Decode(&insts)
		instResp.Body.Close()
		instCount += len(insts.Instances)
	}

	apiVer, _ := ver["apiVersion"].(string)
	svrVer, _ := ver["version"].(string)
	listen, _ := ver["listen"].(string)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "Endpoint:\t%s\n", base)
	if listen != "" {
		fmt.Fprintf(w, "Listening:\t%s\n", listen)
	}
	fmt.Fprintf(w, "Status:\tUP\n")
	if apiVer != "" {
		fmt.Fprintf(w, "API Version:\t%s\n", apiVer)
	}
	if svrVer != "" {
		fmt.Fprintf(w, "Version:\t%s\n", svrVer)
	}
	fmt.Fprintf(w, "Services:\t%d (%d online)\n", len(services.Services), upCount)
	fmt.Fprintf(w, "Instances:\t%d\n", instCount)
	w.Flush()
	return nil
}

type ListOptions struct {
	Services  ListServicesCmd  `cmd:"" name:"services" help:"List registered services."`
	Instances ListInstancesCmd `cmd:"" name:"instances" help:"List service instances."`
}

type ListServicesCmd struct {
	Server      string `help:"Server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
	Environment string `name:"environment" help:"Filter by environment (e.g. development, testing, production)."`
}

func (c *ListServicesCmd) Run() error {
	base := strings.TrimRight(c.Server, "/")
	resp, err := http.Get(base + "/v4/default/registry/microservices")
	if err != nil {
		return fmt.Errorf("get services: %w", err)
	}
	defer resp.Body.Close()

	var result svcreg.GetServicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE ID\tAPP ID\tNAME\tVERSION\tENVIRONMENT\tSTATUS")
	for _, svc := range result.Services {
		if c.Environment != "" && svc.Environment != c.Environment {
			continue
		}
		env := svc.Environment
		if env == "" {
			env = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			truncate(svc.ServiceId, 12), svc.AppId, svc.ServiceName, svc.Version, env, svc.Status)
	}
	w.Flush()
	return nil
}

type ListInstancesCmd struct {
	Server      string `help:"Server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
	ServiceId   string `name:"service-id" help:"Service ID. Optional when --all is set."`
	Environment string `name:"environment" help:"Filter by environment (e.g. development, testing, production)."`
	All         bool   `name:"all" help:"List instances for all services."`
}

func (c *ListInstancesCmd) Run() error {
	base := strings.TrimRight(c.Server, "/")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE ID\tAPP ID\tNAME\tVERSION\tENV\tINSTANCE ID\tHOST\tSTATUS\tENDPOINTS")

	if c.All {
		svcResp, err := http.Get(base + "/v4/default/registry/microservices")
		if err != nil {
			return fmt.Errorf("get services: %w", err)
		}
		var services svcreg.GetServicesResponse
		json.NewDecoder(svcResp.Body).Decode(&services)
		svcResp.Body.Close()

		for _, svc := range services.Services {
			if c.Environment != "" && svc.Environment != c.Environment {
				continue
			}
			instResp, err := http.Get(base + "/v4/default/registry/microservices/" + svc.ServiceId + "/instances")
			if err != nil {
				continue
			}
			var insts svcreg.GetInstancesResponse
			json.NewDecoder(instResp.Body).Decode(&insts)
			instResp.Body.Close()

			env := svc.Environment
			if env == "" {
				env = "-"
			}
			for _, inst := range insts.Instances {
				eps := strings.Join(inst.Endpoints, ",")
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					truncate(svc.ServiceId, 8), svc.AppId, svc.ServiceName, svc.Version, env,
					truncate(inst.InstanceId, 12), inst.HostName, inst.Status, eps)
			}
		}
		w.Flush()
		return nil
	}

	if c.ServiceId == "" {
		return fmt.Errorf("either --service-id or --all is required")
	}

	svcResp, err := http.Get(base + "/v4/default/registry/microservices/" + c.ServiceId)
	var env string
	if err == nil {
		var svc svcreg.GetServiceResponse
		json.NewDecoder(svcResp.Body).Decode(&svc)
		svcResp.Body.Close()
		if svc.Service != nil {
			env = svc.Service.Environment
		}
	}
	if env == "" {
		env = "-"
	}
	if c.Environment != "" && env != "-" && env != c.Environment {
		fmt.Printf("Service environment %q does not match filter %q\n", env, c.Environment)
		return nil
	}

	url := fmt.Sprintf("%s/v4/default/registry/microservices/%s/instances", base, c.ServiceId)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("get instances: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		msg, _ := errResp["errorMessage"].(string)
		return fmt.Errorf("server error: %s", msg)
	}

	var result svcreg.GetInstancesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	for _, inst := range result.Instances {
		eps := strings.Join(inst.Endpoints, ",")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			truncate(c.ServiceId, 8), "", "", "", env,
			truncate(inst.InstanceId, 12), inst.HostName, inst.Status, eps)
	}
	w.Flush()
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
