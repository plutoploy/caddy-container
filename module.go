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
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"reflect"
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

		c, err := client.New(client.WithHost("unix://"+sock), client.WithAPIVersionNegotiation())
		if err != nil {
			continue
		}
		if _, err = c.Ping(ctx, client.PingOptions{}); err != nil {
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

	containersRes, err := dockerClient.ContainerList(r.Context(), client.ContainerListOptions{})
	if err != nil {
		return caddyhttp.Error(http.StatusInternalServerError, err)
	}

	// Normalize result to a slice of container reflect.Values regardless of return type
	var containerVals []reflect.Value
	rv := reflect.ValueOf(containersRes)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.IsValid() {
		if rv.Kind() == reflect.Slice {
			for i := 0; i < rv.Len(); i++ {
				containerVals = append(containerVals, rv.Index(i))
			}
		} else if rv.Kind() == reflect.Struct {
			// Find first slice field and use it
			for _, f := range rv.Fields() {
				if f.Kind() == reflect.Slice {
					for j := 0; j < f.Len(); j++ {
						containerVals = append(containerVals, f.Index(j))
					}
					break
				}
			}
		}
	}

	if len(containerVals) == 0 {
		return caddyhttp.Error(http.StatusInternalServerError, fmt.Errorf("unexpected container list result type: %T", containersRes))
	}

	result := make([]ContainerInfo, 0, len(containerVals))
	for _, cv := range containerVals {
		// Extract name
		name := ""
		if fv := cv.FieldByName("Names"); fv.IsValid() && fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String {
			if fv.Len() > 0 {
				name = strings.TrimPrefix(fv.Index(0).String(), "/")
			}
		} else if nv := cv.FieldByName("Name"); nv.IsValid() && nv.Kind() == reflect.String {
			name = strings.TrimPrefix(nv.String(), "/")
		}

		// Extract ports
		var ports []string
		seen := make(map[uint64]bool)
		if pv := cv.FieldByName("Ports"); pv.IsValid() && pv.Kind() == reflect.Slice {
			for i := 0; i < pv.Len(); i++ {
				portVal := pv.Index(i)
				// try to find field PrivatePort
				if pp := portVal.FieldByName("PrivatePort"); pp.IsValid() && (pp.Kind() >= reflect.Int && pp.Kind() <= reflect.Int64 || pp.Kind() >= reflect.Uint && pp.Kind() <= reflect.Uint64) {
					portNum := pp.Convert(reflect.TypeFor[uint64]()).Interface().(uint64)
					if portNum > 0 && !seen[portNum] {
						seen[portNum] = true
						ports = append(ports, fmt.Sprintf("%d", portNum))
					}
				}
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
