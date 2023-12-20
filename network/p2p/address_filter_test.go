package p2p

import (
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestMakeAddrsFactory(t *testing.T) {
	testcases := []struct {
		name               string
		disablePrivateIP   bool
		allowIPv6Addresses bool
		publicP2pAddresses []string
		addresses          []string
		expectedResult     []string
	}{
		{
			name:               "filter out private IPs addresses",
			disablePrivateIP:   true,
			allowIPv6Addresses: true,
			addresses: []string{
				// public ips
				"/ip4/1.1.1.1/udp/53",
				"/ip6/2001:db8::1/tcp/80",

				// private ips
				"/ip4/192.168.1.1/tcp/80",
				"/ip4/172.16.1.1/tcp/80",
				"/ip4/10.0.0.0/udp/80",
				"/ip4/127.0.0.1/udp/80",
				"/ip6/fc00::1/tcp/80",
				"/ip6/fd00::1/tcp/80",
				"/ip6/fe80::1/udp/53",
				"/ip6/::1/udp/53",
			},
			expectedResult: []string{
				"/ip4/1.1.1.1/udp/53",
				"/ip6/2001:db8::1/tcp/80",
			},
		},
		{
			name:               "filter out private IPs and ip6 addresses",
			disablePrivateIP:   true,
			allowIPv6Addresses: false,
			addresses: []string{
				// public ips
				"/ip4/1.1.1.1/udp/80",
				"/ip6/2001:db8::1/tcp/53",

				// private ips
				"/ip4/192.168.1.1/udp/80",
				"/ip4/172.16.1.1/udp/80",
				"/ip4/10.0.0.0/tcp/80",
				"/ip4/127.0.0.1/tcp/80",
				"/ip6/fc00::1/udp/53",
				"/ip6/fd00::1/udp/53",
				"/ip6/fe80::1/tcp/80",
				"/ip6/::1/tcp/80",
			},
			expectedResult: []string{
				"/ip4/1.1.1.1/udp/80",
			},
		},
		{
			name:               "no filter",
			disablePrivateIP:   false,
			allowIPv6Addresses: true,
			addresses: []string{
				// public ips
				"/ip4/1.1.1.1/tcp/53",
				"/ip6/2001:db8::1/udp/80",

				// private ips
				"/ip4/192.168.1.1/tcp/53",
				"/ip4/172.16.1.1/tcp/53",
				"/ip4/10.0.0.0/udp/53",
				"/ip4/127.0.0.1/udp/53",
				"/ip6/fc00::1/tcp/53",
				"/ip6/fd00::1/tcp/53",
				"/ip6/fe80::1/udp/80",
				"/ip6/::1/udp/80",
			},
			expectedResult: []string{
				"/ip4/1.1.1.1/tcp/53",
				"/ip6/2001:db8::1/udp/80",
				"/ip4/192.168.1.1/tcp/53",
				"/ip4/172.16.1.1/tcp/53",
				"/ip4/10.0.0.0/udp/53",
				"/ip4/127.0.0.1/udp/53",
				"/ip6/fc00::1/tcp/53",
				"/ip6/fd00::1/tcp/53",
				"/ip6/fe80::1/udp/80",
				"/ip6/::1/udp/80",
			},
		},
		{
			name:               "filter out ip6 addresses",
			disablePrivateIP:   false,
			allowIPv6Addresses: false,
			addresses: []string{
				// public ips
				"/ip4/1.1.1.1/tcp/80",
				"/ip6/2001:db8::1/udp/53",

				// private ips
				"/ip4/192.168.1.1/udp/53",
				"/ip4/172.16.1.1/udp/53",
				"/ip4/10.0.0.0/tcp/53",
				"/ip4/127.0.0.1/tcp/53",
				"/ip6/fc00::1/udp/53",
				"/ip6/fd00::1/udp/53",
				"/ip6/fe80::1/tcp/80",
				"/ip6/::1/tcp/80",
			},
			expectedResult: []string{
				"/ip4/1.1.1.1/tcp/80",
				"/ip4/192.168.1.1/udp/53",
				"/ip4/172.16.1.1/udp/53",
				"/ip4/10.0.0.0/tcp/53",
				"/ip4/127.0.0.1/tcp/53",
			},
		},
		{
			name:               "filter out private IPs and ip6 addresses from public P2P Addresses",
			disablePrivateIP:   true,
			allowIPv6Addresses: false,
			publicP2pAddresses: []string{
				"/ip4/1.1.1.1/udp/80",
				"/ip6/2001:db8::1/tcp/53",
			},
			addresses: []string{
				"/ip4/192.168.1.1/udp/80",
				"/ip4/172.16.1.1/udp/80",
				"/ip4/10.0.0.0/tcp/80",
				"/ip4/127.0.0.1/tcp/80",
				"/ip6/fc00::1/udp/53",
				"/ip6/fd00::1/udp/53",
				"/ip6/fe80::1/tcp/80",
				"/ip6/::1/tcp/80",
			},
			expectedResult: []string{
				"/ip4/1.1.1.1/udp/80",
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			var publicAddresses []multiaddr.Multiaddr
			if test.publicP2pAddresses != nil {
				publicAddresses = make([]multiaddr.Multiaddr, len(test.publicP2pAddresses))

				for i, addrStr := range test.publicP2pAddresses {
					multiaddr, err := multiaddr.NewMultiaddr(addrStr)
					require.NoError(t, err)

					publicAddresses[i] = multiaddr
				}
			}

			var multiaddrs []multiaddr.Multiaddr
			if test.addresses != nil {
				multiaddrs = make([]multiaddr.Multiaddr, len(test.addresses))

				for i, addrStr := range test.addresses {
					multiaddr, err := multiaddr.NewMultiaddr(addrStr)
					require.NoError(t, err)

					multiaddrs[i] = multiaddr
				}
			}

			addrsFactory, err := makeAddrsFactory(
				test.disablePrivateIP,
				test.allowIPv6Addresses,
				publicAddresses,
			)
			require.NoError(t, err)

			filteredAddrs := addrsFactory(multiaddrs)

			filteredAddrsInStr := make([]string, len(filteredAddrs))

			for i, addr := range filteredAddrs {
				filteredAddrsInStr[i] = addr.String()
			}

			require.ElementsMatch(t, test.expectedResult, filteredAddrsInStr)
		})
	}
}
