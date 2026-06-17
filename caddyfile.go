package caddycontainer

import (
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	httpcaddyfile.RegisterHandlerDirective("container_list", parseCaddyfile)
}

var _ caddyfile.Unmarshaler = (*ContainerList)(nil)

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	cl := new(ContainerList)
	err := cl.UnmarshalCaddyfile(h.Dispenser)
	return cl, err
}

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
