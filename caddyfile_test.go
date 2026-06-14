package caddycontainer

import (
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

func TestUnmarshalCaddyfile(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectPaths []string
		expectErr   bool
	}{
		{
			name: "basic socket paths",
			input: `container_list {
				socket_paths /var/run/docker.sock /run/podman/podman.sock
			}`,
			expectPaths: []string{"/var/run/docker.sock", "/run/podman/podman.sock"},
			expectErr:   false,
		},
		{
			name: "no paths argument",
			input: `container_list {
				socket_paths
			}`,
			expectErr: true,
		},
		{
			name: "unknown subdirective",
			input: `container_list {
				invalid_subdirective
			}`,
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := caddyfile.NewTestDispenser(tc.input)
			cl := new(ContainerList)
			err := cl.UnmarshalCaddyfile(d)
			if (err != nil) != tc.expectErr {
				t.Fatalf("expected error: %v, got: %v", tc.expectErr, err)
			}
			if !tc.expectErr {
				if len(cl.SocketPaths) != len(tc.expectPaths) {
					t.Fatalf("expected socket paths %v, got %v", tc.expectPaths, cl.SocketPaths)
				}
				for i, p := range cl.SocketPaths {
					if p != tc.expectPaths[i] {
						t.Errorf("at index %d: expected %s, got %s", i, tc.expectPaths[i], p)
					}
				}
			}
		})
	}
}
