package svcreg

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

type Options struct {
	Serve    ServeOptions    `cmd:"" help:"Start the service registry server (API only). Use --web to include the frontend."`
	Frontend FrontendOptions `cmd:"" help:"Start the standalone web frontend."`
	Status   StatusOptions   `cmd:"" help:"Show server status and statistics."`
	List     ListOptions     `cmd:"" help:"List resources."`
}

type StatusOptions struct {
	Server string `help:"Server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
}

func (o *StatusOptions) Run() error {
	client := &Client{Server: o.Server}
	ver, svcCount, upCount, instCount, err := client.Status()
	if err != nil {
		return fmt.Errorf("connect to %s: %w", o.Server, err)
	}
	apiVer, _ := ver["apiVersion"].(string)
	svrVer, _ := ver["version"].(string)
	listen, _ := ver["listen"].(string)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintf(w, "Endpoint:\t%s\n", o.Server)
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
	fmt.Fprintf(w, "Services:\t%d (%d online)\n", svcCount, upCount)
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
	client := &Client{Server: c.Server}
	services, err := client.GetServices()
	if err != nil {
		return fmt.Errorf("get services: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE ID\tAPP ID\tNAME\tVERSION\tENVIRONMENT\tSTATUS")
	for _, svc := range services {
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
	client := &Client{Server: c.Server}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "SERVICE ID\tAPP ID\tNAME\tVERSION\tENV\tINSTANCE ID\tHOST\tSTATUS\tENDPOINTS")

	if c.All {
		services, err := client.GetServices()
		if err != nil {
			return fmt.Errorf("get services: %w", err)
		}
		for _, svc := range services {
			if c.Environment != "" && svc.Environment != c.Environment {
				continue
			}
			insts, err := client.GetInstances(svc.ServiceId)
			if err != nil {
				continue
			}
			env := svc.Environment
			if env == "" {
				env = "-"
			}
			for _, inst := range insts {
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

	svc, err := client.GetService(c.ServiceId)
	env := "-"
	if err == nil && svc != nil && svc.Environment != "" {
		env = svc.Environment
	}
	if c.Environment != "" && env != "-" && env != c.Environment {
		fmt.Printf("Service environment %q does not match filter %q\n", env, c.Environment)
		return nil
	}

	insts, err := client.GetInstances(c.ServiceId)
	if err != nil {
		return fmt.Errorf("get instances: %w", err)
	}

	for _, inst := range insts {
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
