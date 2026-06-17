package caddycontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

func init() {
	caddy.RegisterModule(new(ContainerList))
}

func (*ContainerList) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.container_list",
		New: func() caddy.Module { return new(ContainerList) },
	}
}

type ContainerList struct {
	SocketPaths []string `json:"socket_paths,omitempty"`

	cli    atomic.Pointer[client.Client]
	mu     sync.Mutex
	logger *zap.Logger
}

type ContainerInfo struct {
	Name  string `json:"name"`
	Ports string `json:"ports"`
}

func (cl *ContainerList) getClient(ctx context.Context) (*client.Client, error) {
	if c := cl.cli.Load(); c != nil {
		return c, nil
	}

	cl.mu.Lock()
	defer cl.mu.Unlock()

	// Double check
	if c := cl.cli.Load(); c != nil {
		return c, nil
	}

	socketPaths := cl.SocketPaths
	if len(socketPaths) == 0 {
		socketPaths = []string{
			os.ExpandEnv("$XDG_RUNTIME_DIR/podman/podman.sock"),
		}
	}

	for _, sock := range socketPaths {
		if _, err := os.Stat(sock); os.IsNotExist(err) {
			continue
		}

		c, err := client.NewClientWithOpts(
			client.WithHost("unix://"+sock),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			continue
		}
		if _, err = c.Ping(ctx); err != nil {
			c.Close()
			continue
		}

		cl.cli.Store(c)
		return c, nil
	}

	return nil, fmt.Errorf("no container runtime socket found")
}

func (cl *ContainerList) Provision(ctx caddy.Context) error {
	cl.logger = ctx.Logger(cl)

	// Validate socket paths if provided
	for i, path := range cl.SocketPaths {
		expanded := os.ExpandEnv(path)
		if expanded != path {
			cl.SocketPaths[i] = expanded
		}
	}

	// Set default socket path if none provided
	if len(cl.SocketPaths) == 0 {
		if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
			cl.SocketPaths = []string{
				runtimeDir + "/podman/podman.sock",
			}
		}
	}

	cl.logger.Info("provisioned container_list", zap.Strings("socket_paths", cl.SocketPaths))
	return nil
}

func (cl *ContainerList) Validate() error {
	if len(cl.SocketPaths) == 0 {
		return fmt.Errorf("no socket paths configured and XDG_RUNTIME_DIR not set")
	}
	return nil
}

func (cl *ContainerList) Cleanup() error {
	if c := cl.cli.Load(); c != nil {
		return c.Close()
	}
	return nil
}

func (cl *ContainerList) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	dockerClient, err := cl.getClient(r.Context())
	if err != nil {
		return caddyhttp.Error(http.StatusBadGateway, err)
	}

	containers, err := dockerClient.ContainerList(r.Context(), types.ContainerListOptions{})
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		var ports []string
		seen := make(map[uint16]bool)
		for _, p := range c.Ports {
			if p.PrivatePort > 0 && !seen[p.PrivatePort] {
				seen[p.PrivatePort] = true
				ports = append(ports, fmt.Sprintf("%d", p.PrivatePort))
			}
		}

		result = append(result, ContainerInfo{
			Name:  name,
			Ports: strings.Join(ports, ", "),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}

var (
	_ caddy.Provisioner           = (*ContainerList)(nil)
	_ caddy.Validator             = (*ContainerList)(nil)
	_ caddy.Module                = (*ContainerList)(nil)
	_ caddyhttp.MiddlewareHandler = (*ContainerList)(nil)
	_ caddy.CleanerUpper          = (*ContainerList)(nil)
)
