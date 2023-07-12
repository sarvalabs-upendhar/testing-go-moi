package api

import (
	"testing"

	"github.com/stretchr/testify/require"

	id "github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/common/tests"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

// Net Api Testcases

func TestPublicNetAPI_Peers(t *testing.T) {
	network := NewMockNetwork(t)
	netAPI := NewPublicNetAPI(network)
	peersList := tests.GetTestKramaIDs(t, 5)

	testcases := []struct {
		name         string
		expectedList []id.KramaID
		testFn       func()
	}{
		{
			name:         "Should return an empty list if no Krama ID's in peersList",
			expectedList: make([]id.KramaID, 0),
		},
		{
			name: "Returns a slice of Krama ID's connected to a client",
			testFn: func() {
				network.setPeers(peersList)
			},
			expectedList: peersList,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			if test.testFn != nil {
				test.testFn()
			}

			fetchedList, err := netAPI.Peers()

			require.NoError(t, err)
			require.Equal(t, test.expectedList, fetchedList)
		})
	}
}

func TestPublicNetAPI_Version(t *testing.T) {
	network := NewMockNetwork(t)
	netAPI := NewPublicNetAPI(network)
	version := "0.0.1"

	network.setVersion(version)

	testcases := []struct {
		name          string
		args          rpcargs.NetArgs
		expectedValue string
	}{
		{
			name:          "Should return default version",
			expectedValue: version,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(testing *testing.T) {
			version, err := netAPI.Version()

			require.NoError(t, err)
			require.Equal(t, test.expectedValue, version)
		})
	}
}
