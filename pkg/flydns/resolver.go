package flydns

import (
	"context"
	"net"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"
)

// ResolverForOrg takes a connection to the wireguard agent and an organization
// and returns a working net.Resolver for DNS for that organization, along with the
// address of the nameserver.
func ResolverForOrg(ctx context.Context, c *agent.Client, org *api.Organization) (*net.Resolver, string, error) {
	// do this explicitly so we can get the DNS server address
	ts, err := c.Establish(ctx, org.Slug)
	if err != nil {
		return nil, "", err
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d, err := c.Dialer(ctx, org.Slug)
			if err != nil {
				return nil, err
			}

			network = "tcp"
			server := net.JoinHostPort(ts.TunnelConfig.DNS.String(), "53")

			// the connections we get from the agent are over a unix domain socket proxy,
			// which implements the PacketConn interface, so Go's janky DNS library thinks
			// we want UDP DNS. Trip it up.
			type fakeConn struct {
				net.Conn
			}

			c, err := d.DialContext(ctx, network, server)
			if err != nil {
				return nil, err
			}

			return &fakeConn{c}, nil
		},
	}, ts.TunnelConfig.DNS.String(), nil
}
