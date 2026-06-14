package caddycontainer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func init() {
	caddy.RegisterModule(ContainerList{})
}

func (ContainerList) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.container_list",
		New: func() caddy.Module { return new(ContainerList) },
	}
}

type ContainerList struct {
	SocketPaths []string `json:"socket_paths,omitempty"`
}

type ContainerInfo struct {
	Name  string `json:"name"`
	Ports string `json:"ports"`
}

func (cl ContainerList) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	socketPaths := cl.SocketPaths
	if len(socketPaths) == 0 {
		socketPaths = []string{
			os.ExpandEnv("$XDG_RUNTIME_DIR/podman/podman.sock"),
		}
	}

	var dockerClient *client.Client
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
		if _, err = c.Ping(r.Context()); err != nil {
			continue
		}

		dockerClient = c
		break
	}

	if dockerClient == nil {
		return caddyhttp.Error(http.StatusBadGateway, fmt.Errorf("no container runtime socket found"))
	}

	containers, err := dockerClient.ContainerList(r.Context(), container.ListOptions{})
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	result := make([]ContainerInfo, 0, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
		}

		ports := ""
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				ports += fmt.Sprintf("%d", p.PrivatePort)
			}
		}

		result = append(result, ContainerInfo{Name: name, Ports: ports})
	}

	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(result)
}
