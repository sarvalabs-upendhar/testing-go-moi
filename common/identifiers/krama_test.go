package identifiers

import (
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func Test_NewKramaID(t *testing.T) {
	privKeyBytes, _ := GetRandomPrivateKeys([32]byte{})

	testcases := []struct {
		name            string
		privateKey      []byte
		expectedKramaID KramaID
		expectedError   error
	}{
		{
			name:          "invalid private key",
			privateKey:    []byte{3, 2},
			expectedError: errors.New("error decoding secp256k1 key"),
		},
		{
			name:            "valid private key",
			privateKey:      privKeyBytes[32:],
			expectedKramaID: "1116Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			kramaID, err := GenerateKramaIDv0(NetworkZone0, test.privateKey)
			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedKramaID, kramaID)
		})
	}
}

func Test_NewKramaIDFromPeerID(t *testing.T) {
	var signKey [32]byte

	privKeyBytes, _ := GetRandomPrivateKeys(signKey)
	peerID, _ := GeneratePeerID(privKeyBytes[32:])

	testcases := []struct {
		name            string
		peerID          peer.ID
		networkZone     NetworkZone
		expectedKramaID KramaID
	}{
		{
			name:            "should return a valid krama id with network zone 0",
			peerID:          peerID,
			networkZone:     NetworkZone0,
			expectedKramaID: KramaID(strings.Join([]string{"11", peerID.String()}, "")),
		},
		{
			name:            "should return a valid krama id with network zone 3",
			peerID:          peerID,
			networkZone:     NetworkZone3,
			expectedKramaID: KramaID(strings.Join([]string{"1q", peerID.String()}, "")),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			kramaID := NewKramaIDFromPeerID(KindGuardian, KramaIDV0, test.networkZone, test.peerID)

			require.Equal(t, test.expectedKramaID, kramaID)
		})
	}
}

func Test_Decompose(t *testing.T) {
	testcases := []struct {
		name             string
		kramaID          KramaID
		expectedTag      KramaIDTag
		expectedMetadata KramaMetadata
		expectedPeerID   string
		expectedError    error
	}{
		{
			name:          "empty krama id",
			kramaID:       KramaID(""),
			expectedError: errors.New("invalid krama id: empty"),
		},
		{
			name:          "invalid krama id",
			kramaID:       KramaID("2116Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E"),
			expectedError: ErrUnsupportedTag,
		},
		{
			name:          "invalid peer id length",
			kramaID:       KramaID("1116Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh"),
			expectedError: errors.New("invalid krama id: invalid length"),
		},
		{
			name:             "valid krama id",
			kramaID:          KramaID("1116Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E"),
			expectedTag:      TagKramaV0,
			expectedMetadata: KramaMetadata(byte(NetworkZone0 >> 4)),
			expectedPeerID:   "16Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			tag, metadata, peerID, err := test.kramaID.Decompose()

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedTag, tag)
			require.Equal(t, test.expectedMetadata, metadata)
			require.Equal(t, test.expectedPeerID, peerID)
		})
	}
}

func Test_DecodePeerID(t *testing.T) {
	peerID, _ := peer.Decode("16Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E")

	testcases := []struct {
		name           string
		kramaID        KramaID
		expectedPeerID peer.ID
		expectedError  error
	}{
		{
			name:          "invalid krama id",
			kramaID:       KramaID(""),
			expectedError: errors.New("invalid krama id: empty"),
		},
		{
			name:          "invalid peer id",
			kramaID:       KramaID("1126Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E"),
			expectedError: errors.New("failed to parse peer ID: invalid cid: selected encoding not supported"),
		},
		{
			name:           "valid krama id",
			kramaID:        KramaID("1116Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E"),
			expectedPeerID: peerID,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			peerID, err := test.kramaID.DecodedPeerID()

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedPeerID, peerID)
		})
	}
}

func Test_RandomKramaIDv0(t *testing.T) {
	testcases := []struct {
		name string
	}{
		{
			name: "should generate random network key",
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			kramaID, err := RandomKramaIDv0()

			require.NoError(t, err)
			require.NoError(t, kramaID.Validate())
		})
	}
}

func Test_GenerateKramaIDv0(t *testing.T) {
	privKeyBytes, _ := GetRandomPrivateKeys([32]byte{})

	testcases := []struct {
		name            string
		privateKey      []byte
		expectedKramaID KramaID
		expectedError   error
	}{
		{
			name:          "invalid private key",
			privateKey:    []byte{3, 2},
			expectedError: errors.New("error decoding secp256k1 key"),
		},
		{
			name:            "valid private key",
			privateKey:      privKeyBytes[32:],
			expectedKramaID: KramaID("1116Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			kramaID, err := GenerateKramaIDv0(NetworkZone0, test.privateKey)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedKramaID, kramaID)
		})
	}
}

func Test_PeerIDLength(t *testing.T) {
	testcases := []struct {
		name          string
		tag           KramaIDTag
		expectedLen   int
		expectedError error
	}{
		{
			name:          "invalid tag",
			tag:           KramaIDTag(5),
			expectedError: ErrUnsupportedTag,
		},
		{
			name:        "valid tag",
			tag:         TagKramaV0,
			expectedLen: 53,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			length, err := peerIDLength(test.tag)

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
			require.Equal(t, test.expectedLen, length)
		})
	}
}

func TestKramaID_Validate(t *testing.T) {
	testcases := []struct {
		name          string
		kramaID       KramaID
		expectedError error
	}{
		{
			name:          "invalid tag",
			kramaID:       KramaID(""),
			expectedError: errors.New("failed to decompose krama id: invalid krama id: empty"),
		},
		{
			name:          "invalid peer id",
			kramaID:       KramaID("1126Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E"),
			expectedError: errors.New("invalid peer id: failed to parse peer ID: invalid cid"),
		},
		{
			name:    "valid krama id",
			kramaID: KramaID("1116Uiu2HAm2itTsAm1YonwaN8c4XQoqa4egGJAMxwMuXpzNKTwh68E"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := test.kramaID.Validate()

			if test.expectedError != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}
