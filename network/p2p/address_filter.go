package p2p

import (
	"net"

	"github.com/multiformats/go-multiaddr"
)

// CIDR notation for private IPv4 and IPv6 addresses
var (
	privateIPv4Ranges = []string{
		"10.0.0.0/8",     // private addresses
		"172.16.0.0/12",  // private addresses
		"192.168.0.0/16", // private addresses
		"127.0.0.0/8",    // loopback addresses
	}

	privateIPv6Ranges = []string{
		"fc00::/7",  // unique local addresses
		"fd00::/8",  // unique local addresses
		"fe80::/10", // link-local addresses
		"::1/128",   // loopback addresses
	}
)

// setupPrivateIPFilter adds filters for private IPv4 and IPv6 ranges to the given addressFilters.
func setupPrivateIPFilter(addressFilters *multiaddr.Filters) error {
	for _, ipRange := range append(privateIPv4Ranges, privateIPv6Ranges...) {
		_, ipNet, err := net.ParseCIDR(ipRange)
		if err != nil {
			return err
		}

		addressFilters.AddFilter(*ipNet, multiaddr.ActionDeny)
	}

	return nil
}

func setupIPV6Filter(addressFilters *multiaddr.Filters) {
	addressFilters.AddFilter(net.IPNet{
		IP:   net.IPv6zero,
		Mask: net.CIDRMask(0, 128),
	}, multiaddr.ActionDeny)
}

func makeAddrsFactory(
	disablePrivateIP,
	allowIPv6Addresses bool,
	publicP2pAddresses []multiaddr.Multiaddr,
) (func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr, error) {
	addressFilters := multiaddr.NewFilters()

	if disablePrivateIP {
		err := setupPrivateIPFilter(addressFilters)
		if err != nil {
			return nil, err
		}
	}

	if !allowIPv6Addresses {
		setupIPV6Filter(addressFilters)
	}

	return func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
		if len(publicP2pAddresses) > 0 {
			addrs = append(addrs, publicP2pAddresses...)
		}

		// Filter out addresses based on the Filters
		filteredAddrs := make([]multiaddr.Multiaddr, 0, len(addrs))

		for _, addr := range addrs {
			if !addressFilters.AddrBlocked(addr) {
				filteredAddrs = append(filteredAddrs, addr)
			}
		}

		return filteredAddrs
	}, nil
}
