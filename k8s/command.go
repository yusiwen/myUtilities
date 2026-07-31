package k8s

import (
	"fmt"
	"io"
	"net/http"
	"os"

	corek8s "github.com/yusiwen/myUtilities/core/k8s"
)

type Options struct {
	Secret   SecretOptions   `cmd:"" name:"secret" aliases:"s" help:"Generate or decode a Kubernetes Secret YAML."`
	Get      GetOptions      `cmd:"" name:"get" help:"List Kubernetes resources (pods, nodes, deployments, services)."`
	Describe DescribeOptions `cmd:"" name:"describe" help:"Describe a Kubernetes resource in detail."`
	Serve    ServeOptions    `cmd:"" name:"serve" help:"Start Kubernetes tools HTTP server."`
}

type SecretOptions struct {
	Name    string   `arg:"" name:"name" help:"Secret name (or path to YAML file when --decode)."`
	Data    []string `arg:"" optional:"" name:"data" help:"key=value pairs."`
	FromEnv string   `short:"f" name:"from-env" help:"Read key=value pairs from .env file."`
	Output  string   `short:"o" name:"output" help:"Output file path."`
	Decode  bool     `short:"d" name:"decode" help:"Decode an existing Secret YAML back to plaintext key=value."`
}

type ServeOptions struct {
	Port       int    `help:"Port to listen on." default:"8089"`
	Kubeconfig string `name:"kubeconfig" help:"Path to kubeconfig file."`
}

func (o *SecretOptions) Run() error {
	if o.Decode {
		return o.decode()
	}
	return o.encode()
}

// RegisterHandlers registers all k8s API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux, configDir string) {
	corek8s.RegisterHandlers(mux, configDir)
}

func (o *ServeOptions) Run() error {
	if o.Kubeconfig != "" {
		data, err := os.ReadFile(o.Kubeconfig)
		if err != nil {
			return fmt.Errorf("read kubeconfig: %w", err)
		}
		if err := corek8s.SaveKubeconfig(string(data)); err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux, "")
	fmt.Printf("Kubernetes tools server listening on :%d\n", o.Port)
	idx, _ := corek8s.LoadIndex()
	if idx.Active != "" {
		fmt.Printf("  Active config: %s\n", idx.Active)
		fmt.Printf("  Saved configs: %d\n", len(idx.Configs))
	} else {
		fmt.Printf("  Kubeconfig: not configured\n")
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func (o *SecretOptions) decode() error {
	data, err := os.ReadFile(o.Name)
	if err != nil {
		return fmt.Errorf("read YAML: %w", err)
	}

	result, err := corek8s.DecodeSecretYAML(data)
	if err != nil {
		return err
	}

	out := os.Stdout
	if o.Output != "" {
		f, err := os.Create(o.Output)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		out = f
	}

	for k, v := range result {
		fmt.Fprintf(out, "%s=%s\n", k, v)
	}
	return nil
}

func (o *SecretOptions) encode() error {
	pairs, err := o.resolvePairs()
	if err != nil {
		return err
	}
	if len(pairs) == 0 {
		return fmt.Errorf("no key=value pairs provided")
	}

	yamlOut, err := corek8s.EncodeSecretYAML(o.Name, pairs)
	if err != nil {
		return err
	}

	out := os.Stdout
	if o.Output != "" {
		f, err := os.Create(o.Output)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		out = f
	}

	fmt.Fprint(out, yamlOut)
	return nil
}

func (o *SecretOptions) resolvePairs() ([]string, error) {
	if len(o.Data) > 0 {
		return o.Data, nil
	}
	if o.FromEnv != "" {
		data, err := os.ReadFile(o.FromEnv)
		if err != nil {
			return nil, fmt.Errorf("read env file: %w", err)
		}
		return corek8s.ParseEnvFile(string(data)), nil
	}
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return corek8s.ParseEnvFile(string(data)), nil
	}
	return nil, fmt.Errorf("provide key=value pairs, --from-env, or pipe input")
}
