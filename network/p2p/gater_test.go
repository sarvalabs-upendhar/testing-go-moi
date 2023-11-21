package p2p

import (
	"testing"

	maddr "github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/stretchr/testify/require"
)

func TestConnectionGater_Blocking(t *testing.T) {
	testcases := []struct {
		name            string
		isBlocking      bool
		address         string
		expectedAllowed bool
	}{
		{
			name:            "block private address",
			isBlocking:      true,
			address:         "/ip4/192.168.1.1/tcp/80",
			expectedAllowed: false,
		},
		{
			name:            "shouldn't block public address",
			isBlocking:      true,
			address:         "/ip4/1.1.1.1/udp/53",
			expectedAllowed: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			gater := NewConnectionGater(test.isBlocking)
			require.NotNil(t, &gater, "%s", &gater)

			multiaddr, err := maddr.NewMultiaddr(test.address)
			require.Nil(t, err, "%s", err)

			allowed := gater.InterceptAddrDial(tests.GetTestPeerID(t), multiaddr)
			require.Equal(t, test.expectedAllowed, allowed, "%v", allowed)
		})
	}
}

func TestConnectionGater_NonBlocking(t *testing.T) {
	testcases := []struct {
		name            string
		isBlocking      bool
		address         string
		expectedAllowed bool
	}{
		{
			name:            "shouldn't block private address",
			isBlocking:      false,
			address:         "/ip4/192.168.1.1/tcp/80",
			expectedAllowed: true,
		},
		{
			name:            "shouldn't block public address",
			isBlocking:      false,
			address:         "/ip4/1.1.1.1/udp/53",
			expectedAllowed: true,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			gater := NewConnectionGater(test.isBlocking)
			require.NotNil(t, &gater, "%s", &gater)

			multiaddr, err := maddr.NewMultiaddr(test.address)
			require.Nil(t, err, "%s", err)

			allowed := gater.InterceptAddrDial(tests.GetTestPeerID(t), multiaddr)
			require.Equal(t, test.expectedAllowed, allowed, "%v", allowed)
		})
	}
}
