package main

import (
	"go/doc"
	"log"
	// "time"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
)
type CaddyContainer struct {
	logger    *zap.Logger
	container []string
	port      []int
}


func fetchContainers() *CaddyContainer {
	podman, _ := net.Dial("unix", "/run/user/1000/podman/podman.sock")
	// docker, _ := dockerclient.NewDockerClient("unix:///run/user/1000/docker/docker.sock", nil)
	// dockerRoot, _ := dockerclient.NewDockerClient("unix:///var/docker/docker.sock", nil)

	containers, _ := podman.(false, false, "")
	if containers == nil {
		containers, _ = docker.ListContainers(false, false, "")
	}
	for _, c := range containers {
		return &CaddyContainer{
			container: []string{c.Names[0]},
			port:      []int{c.Ports[0].ContainerPort},
		}
	}
	return nil
}

func init() {
	caddy.RegisterModule(CaddyContainer{})
	httpcaddyfile.RegisterHandlerDirective("", parseCaddyfile)
}

func (CaddyContainer) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "http.handlers.caddy_container", New: func() caddy.Module { return new(CaddyContainer) }}
}

func (m *CaddyContainer) Provision(ctx caddy.Context) error {
	cont := fetchContainers()
	if cont == nil {
		return fmt.Errorf("no caddy container found")
	}
	m.container = cont.container
	m.port = cont.port
	return nil
}

func (m *CaddyContainer) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {

	}
	return nil
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	sr := new(CaddyContainer)
	err := sr.UnmarshalCaddyfile(h.Dispenser)
	return sr, err
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
		if m.logger != nil {
			m.logger.Warn("invalid host header", zap.String("host", r.Host))
		}
		return nil
	}
	m = fetchContainers()
	subdomain := m.CaddyModule().ID.Name()
	port := m.CaddyModule().New().CaddyModule()
	upstream := fmt.Sprintf("%s:%d", subdomain, port)
	caddyhttp.SetVar(r.Context(), "backend_upstream", upstream)
	return next.ServeHTTP(w, r)
}

func eventCallback(event *dockerclient.Event, ec chan error, args ...interface{}) {
	log.Printf("Received event: %#v\n", *event)
}
