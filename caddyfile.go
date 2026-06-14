package caddycontainer

import (
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

func (cl *ContainerList) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "socket_paths":
				cl.SocketPaths = d.RemainingArgs()
				if len(cl.SocketPaths) == 0 {
					return d.Errf("socket_paths requires at least one argument")
				}
			default:
				return d.Errf("unrecognized subdirective: %s", d.Val())
			}
		}
	}
	return nil
}
