package moiclient

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"testing"
	"time"

	pisa "github.com/sarvalabs/go-pisa/moi"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/crypto"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

func CreateSendIXFromSendIXArgs(t *testing.T, sendIxArgs *common.SendIXArgs, mnemonic string) *rpcargs.SendIX {
	t.Helper()

	bz, err := polo.Polorize(sendIxArgs)
	require.NoError(t, err)

	sign, err := crypto.GetSignature(bz, mnemonic)
	require.NoError(t, err)

	return &rpcargs.SendIX{
		IXArgs:    hex.EncodeToString(bz),
		Signature: sign,
	}
}

func GetLatestNonce(t *testing.T, client *Client, addr common.Address) uint64 {
	t.Helper()

	nonce, err := client.InteractionCount(context.Background(), &rpcargs.InteractionCountArgs{
		Address: addr,
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	require.NoError(t, err)

	return nonce.ToUint64()
}

func GetLatestHeight(t *testing.T, client *Client, addr common.Address) uint64 {
	t.Helper()

	acc, err := client.AccountMetaInfo(context.Background(), &rpcargs.GetAccountArgs{
		Address: addr,
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	require.NoError(t, err)

	return acc.Height.ToUint64()
}

// RetryFetchReceipt keeps trying to fetch receipt for given ixHash until it is timed out
func RetryFetchReceipt(t *testing.T, ctx context.Context, client *Client, ixHash common.Hash) *rpcargs.RPCReceipt {
	t.Helper()

	log.Println("fetching receipt for ixHash ", ixHash)

	receiptArgs := &rpcargs.ReceiptArgs{
		Hash: ixHash,
	}

	for {
		select {
		case <-ctx.Done():
			require.FailNow(t, "ix receipt not found,"+
				" as forming the ICS took more time, so try running tests again", ixHash)
		default:
			receipt, err := client.InteractionReceipt(ctx, receiptArgs)
			if err == nil {
				return receipt
			}

			time.Sleep(time.Second)
		}
	}
}

// GetTesseract returns tesseract for the given senderAddr and height
func GetTesseract(t *testing.T, client *Client, addr common.Address, height int64) *rpcargs.RPCTesseract {
	t.Helper()

	args := &rpcargs.TesseractArgs{
		Address:          addr,
		WithInteractions: true,
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &height,
		},
	}

	ts, err := client.Tesseract(context.Background(), args)
	require.NoError(t, err)

	return ts
}

// GetLogicID returns logicID for the given senderAddr and height
func GetLogicID(t *testing.T, client *Client, addr common.Address, height int64) common.LogicID {
	t.Helper()

	ts := GetTesseract(t, client, addr, height)

	receiptArgs := &rpcargs.ReceiptArgs{
		Hash: ts.Ixns[0].Hash,
	}

	receipt, err := client.InteractionReceipt(context.Background(), receiptArgs)
	require.NoError(t, err)

	var logicReceipt common.LogicDeployReceipt

	err = json.Unmarshal(receipt.ExtraData, &logicReceipt)
	require.NoError(t, err)

	return logicReceipt.LogicID
}

// GetLogicManifestByEncodingType returns the manifest according to the given encoding type POLO, JSON or YAML
func GetLogicManifestByEncodingType(
	t *testing.T,
	res hexutil.Bytes,
	encoding string,
) (hexutil.Bytes, error) {
	t.Helper()

	switch encoding {
	case "POLO", "":
		return res, nil
	case "JSON":
		logicManifest := res.Bytes()

		depolorizedManifest, err := engineio.NewManifest(logicManifest, engineio.POLO)
		if err != nil {
			return nil, err
		}

		return depolorizedManifest.Encode(engineio.JSON)
	case "YAML":
		logicManifest := res.Bytes()

		depolorizedManifest, err := engineio.NewManifest(logicManifest, engineio.POLO)
		if err != nil {
			return nil, err
		}

		return depolorizedManifest.Encode(engineio.YAML)
	default:
		return nil, errors.New("invalid encoding type")
	}
}

type TokenLedgerState struct {
	Name     string
	Symbol   string
	Supply   *big.Int
	Balances map[common.Address]*big.Int
}

func GetTokenLedgerState(t *testing.T, moiClient *Client, logicID common.LogicID) TokenLedgerState {
	t.Helper()

	getLatestStorage := func(slot uint8) hexutil.Bytes {
		s, err := moiClient.LogicStorage(context.Background(), &rpcargs.GetLogicStorageArgs{
			LogicID:    logicID,
			StorageKey: pisa.Slothash(slot),
			Options: rpcargs.TesseractNumberOrHash{
				TesseractNumber: &rpcargs.LatestTesseractHeight,
			},
		})
		require.NoError(t, err)

		return s
	}

	state := TokenLedgerState{}

	rawName := getLatestStorage(0)
	rawSymbol := getLatestStorage(1)
	rawSupply := getLatestStorage(2)
	rawBalances := getLatestStorage(3)

	err := polo.Depolorize(&state.Name, rawName)
	require.NoError(t, err)

	err = polo.Depolorize(&state.Symbol, rawSymbol)
	require.NoError(t, err)

	err = polo.Depolorize(&state.Supply, rawSupply)
	require.NoError(t, err)

	err = polo.Depolorize(&state.Balances, rawBalances)
	require.NoError(t, err)

	fmt.Printf("token ledger state : %+v\n", state)

	return state
}
