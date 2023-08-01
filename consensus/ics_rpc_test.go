package consensus

import (
	"context"
	"io/ioutil"
	"testing"

	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/crypto"
	"github.com/sarvalabs/go-moi/crypto/poi"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/tests"
	networkmsg "github.com/sarvalabs/go-moi/network/message"
	"github.com/stretchr/testify/require"
)

func TestICSRequest(t *testing.T) {
	address, mnemonic := tests.RandomAddressWithMnemonic(t)

	assetPayload := &common.AssetCreatePayload{
		Symbol: "MOI",
	}

	assetCreatePayloadBytes, err := assetPayload.Bytes()
	require.NoError(t, err)

	ixArgs := common.SendIXArgs{
		Sender:  address,
		Type:    common.IxAssetCreate,
		Payload: assetCreatePayloadBytes,
	}

	rawSign := tests.GetIXSignature(t, &ixArgs, mnemonic)

	ixData := common.IxData{
		Input: common.IxInput{
			Sender:  address,
			Type:    common.IxAssetCreate,
			Payload: assetCreatePayloadBytes,
		},
	}

	dir, err := ioutil.TempDir("", "testDir")
	require.NoError(t, err)

	_, kramaID, err := poi.RandGenKeystore(dir, "nodepass")
	require.NoError(t, err)

	vConfig := &crypto.VaultConfig{
		DataDir:      dir,
		NodePassword: "nodepass",
	}

	vault, err := crypto.NewVault(vConfig, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	verifiedRawInteractions := getRawInteraction(t, ixData, rawSign)
	rawInteractions := getRawInteraction(t, ixData, nil)

	rawVerifiedCanonicalReq, verifiedSign := getSignature(t, kramaID, verifiedRawInteractions, vault)
	rawCanonicalReq, sign := getSignature(t, kramaID, rawInteractions, vault)

	engine := &MockEngine{
		requests: make(chan Request),
	}

	ICSRPCService := NewICSRPCService(engine)

	testcases := []struct {
		name          string
		request       *networkmsg.ICSRequest
		response      *networkmsg.ICSResponse
		expectedError error
	}{
		{
			name: "signature of interactions verified in krama engine",
			request: &networkmsg.ICSRequest{
				ReqData:   rawVerifiedCanonicalReq,
				Signature: verifiedSign,
			},
			response: &networkmsg.ICSResponse{},
		},
		{
			name: "signature verification of interactions failed in krama engine",
			request: &networkmsg.ICSRequest{
				ReqData:   rawCanonicalReq,
				Signature: sign,
			},
			response:      &networkmsg.ICSResponse{},
			expectedError: errors.New("failed to verify ix signature"),
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			err := ICSRPCService.ICSRequest(context.TODO(), test.request, test.response)
			if test.expectedError != nil {
				require.ErrorContains(t, err, test.expectedError.Error())

				return
			}

			require.NoError(t, err)
		})
	}
}
