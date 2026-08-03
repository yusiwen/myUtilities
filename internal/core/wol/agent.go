package wol

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	corenet "github.com/yusiwen/myUtilities/internal/core/net"
)

const agentMaxRetries = 5

// AgentConfig configures the WOL notification agent.
type AgentConfig struct {
	Server    string
	Token     string
	Hostname  string
	Interface string
	Boot      bool
	Shutdown  bool
	Register  bool
}

// RunAgent executes a single register/boot/shutdown notification with retry backoff.
func RunAgent(cfg AgentConfig) error {
	hostname := cfg.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %v", err)
		}
	}

	server := cfg.Server
	if !strings.Contains(server, "://") {
		server = "http://" + server
	}

	serverURL, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("agent: invalid server URL %q: %v", cfg.Server, err)
	}

	if cfg.Register {
		return runRegister(cfg, hostname, serverURL)
	}

	action := "boot"
	if cfg.Shutdown {
		action = "shutdown"
	}
	return runNotify(cfg, hostname, serverURL, action)
}

func runRegister(cfg AgentConfig, hostname string, serverURL *url.URL) error {
	var macStr string
	if cfg.Interface != "" {
		iface, err := corenet.GetInterfaceByName(cfg.Interface)
		if err != nil {
			return fmt.Errorf("agent: %v", err)
		}
		if iface.HardwareAddr == nil {
			return fmt.Errorf("agent: interface %s has no MAC address", cfg.Interface)
		}
		macStr = iface.HardwareAddr.String()
	} else {
		mac, err := corenet.GetOutboundMAC(serverURL.Host)
		if err != nil {
			return fmt.Errorf("agent: unable to determine outbound MAC: %v", err)
		}
		macStr = mac.String()
	}

	log.Printf("Agent: registering hostname %q with MAC %s to server %s", hostname, macStr, cfg.Server)

	body := fmt.Sprintf(`{"name":"%s","mac":"%s"}`, hostname, macStr)
	for i := range agentMaxRetries {
		resp, err := postWithToken(cfg.Server+"/api/register", cfg.Token, "application/json", strings.NewReader(body))
		if err == nil {
			if resp.StatusCode == http.StatusCreated {
				resp.Body.Close()
				log.Printf("Agent: successfully registered %q (%s) at %s", hostname, macStr, cfg.Server)
				return nil
			}
			resp.Body.Close()
			return fmt.Errorf("agent: server returned status %d for registration of %s", resp.StatusCode, hostname)
		}
		if i < agentMaxRetries-1 {
			wait := time.Duration(i+1) * time.Second
			log.Printf("Agent: attempt %d failed, retrying in %v: %v", i+1, wait, err)
			time.Sleep(wait)
		} else {
			return fmt.Errorf("agent: failed to register %q after %d retries: %v", hostname, agentMaxRetries, err)
		}
	}
	return nil
}

func runNotify(cfg AgentConfig, hostname string, serverURL *url.URL, action string) error {
	mac, err := corenet.GetOutboundMAC(serverURL.Host)
	if err != nil {
		log.Printf("Agent: warning: unable to determine outbound MAC: %v", err)
	}

	log.Printf("Agent: sending %s notification for hostname %q to server %s", action, hostname, cfg.Server)

	for i := range agentMaxRetries {
		u := fmt.Sprintf("%s/api/notify/%s?type=%s", cfg.Server, url.PathEscape(hostname), action)
		if len(mac) > 0 {
			u += "&mac=" + url.QueryEscape(mac.String())
		}
		resp, err := postWithToken(u, cfg.Token, "application/json", nil)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("Agent: %s notification sent for %q at %s", action, hostname, cfg.Server)
				return nil
			}
			return fmt.Errorf("agent: server returned status %d for %s", resp.StatusCode, hostname)
		}
		if i < agentMaxRetries-1 {
			wait := time.Duration(i+1) * time.Second
			log.Printf("Agent: attempt %d failed, retrying in %v: %v", i+1, wait, err)
			time.Sleep(wait)
		} else {
			return fmt.Errorf("agent: failed to send %s notification for %q after %d retries: %v", action, hostname, agentMaxRetries, err)
		}
	}
	return nil
}

func postWithToken(url, token, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Auth-Token", token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return http.DefaultClient.Do(req)
}
