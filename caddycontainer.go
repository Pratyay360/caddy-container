package caddycontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(CaddyContainer{})
	httpcaddyfile.RegisterHandlerDirective("caddy_container", parseCaddyfile)
}

type CaddyContainer struct {
	Container  []string `json:"container,omitempty"`
	Port       []int    `json:"port,omitempty"`
	SocketPath string   `json:"socket_path,omitempty"`
	logger     *zap.Logger
}

// CaddyModule returns the Caddy module information.
func (CaddyContainer) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.caddy_container",
		New: func() caddy.Module { return new(CaddyContainer) },
	}
}

func (m *CaddyContainer) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)

	if m.SocketPath == "" {
		m.SocketPath = "/run/user/1000/podman/podman.sock"
	}

	cont, err := m.fetchContainers()
	if err != nil {
		return fmt.Errorf("fetching containers: %w", err)
	}
	if cont == nil {
		return fmt.Errorf("no containers found")
	}

	m.Container = cont.Container
	m.Port = cont.Port
	m.logger.Info("provisioned caddy_container",
		zap.Strings("containers", m.Container),
		zap.Ints("ports", m.Port),
	)
	return nil
}

func (m *CaddyContainer) Validate() error {
	if len(m.Container) == 0 {
		return fmt.Errorf("no containers configured")
	}
	if len(m.Port) == 0 {
		return fmt.Errorf("no ports configured")
	}
	return nil
}

func (m *CaddyContainer) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		if !d.Args(&m.SocketPath) {
			return d.ArgErr()
		}
	}
	return nil
}
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	m := new(CaddyContainer)
	err := m.UnmarshalCaddyfile(h.Dispenser)
	return m, err
}

func (m *CaddyContainer) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	hostHeader := r.Host
	if h, _, err := net.SplitHostPort(hostHeader); err == nil {
		hostHeader = h
	}
	hostHeader = strings.TrimSuffix(strings.ToLower(hostHeader), ".")
	labels := strings.Split(hostHeader, ".")
	if len(labels) < 2 || labels[0] == "" {
		http.Error(w, "Invalid host", http.StatusBadRequest)
		m.logger.Warn("invalid host header", zap.String("host", r.Host))
		return nil
	}
	subdomain := labels[0]
	upstreamHost := m.Container[0]
	upstreamPort := m.Port[0]

	for i, name := range m.Container {
		if strings.Contains(strings.ToLower(name), subdomain) {
			upstreamHost = name
			if i < len(m.Port) {
				upstreamPort = m.Port[i]
			}
			break
		}
	}

	upstream := fmt.Sprintf("%s:%d", upstreamHost, upstreamPort)
	m.logger.Debug("routing request",
		zap.String("subdomain", subdomain),
		zap.String("upstream", upstream),
	)

	caddyhttp.SetVar(r.Context(), "backend_upstream", upstream)
	return next.ServeHTTP(w, r)
}

func (m *CaddyContainer) fetchContainers() (*CaddyContainer, error) {
	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", m.SocketPath)
			},
		},
	}

	req, err := http.NewRequest("GET", "http://localhost/containers/json", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var containers []struct {
		Names []string `json:"Names"`
		Ports []struct {
			PrivatePort int    `json:"PrivatePort"`
			PublicPort  int    `json:"PublicPort"`
			Type        string `json:"Type"`
		} `json:"Ports"`
	}

	if err := json.Unmarshal(body, &containers); err != nil {
		return nil, fmt.Errorf("unmarshaling response: %w", err)
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no running containers found")
	}

	var names []string
	var ports []int

	for _, c := range containers {
		if len(c.Names) == 0 {
			continue
		}
		name := strings.TrimPrefix(c.Names[0], "/")
		names = append(names, name)
		ports = append(ports, c.Ports[0].PrivatePort)

	}

	if len(names) == 0 {
		return nil, fmt.Errorf("no valid containers found")
	}

	return &CaddyContainer{
		Container: names,
		Port:      ports,
	}, nil
}
var (
	_ caddy.Provisioner           = (*CaddyContainer)(nil)
	_ caddy.Validator             = (*CaddyContainer)(nil)
	_ caddyhttp.MiddlewareHandler = (*CaddyContainer)(nil)
	_ caddyfile.Unmarshaler       = (*CaddyContainer)(nil)
)
