package moiclient

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/sarvalabs/go-moi/crypto"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	identifiers "github.com/sarvalabs/go-moi-identifiers"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/go-moi/bgclient"
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/pisa"
	rpcargs "github.com/sarvalabs/go-moi/jsonrpc/args"
)

const (
	JSONRPCURLWaitTime   = 120 * time.Second
	JSONRPCURLQueryTime  = 5 * time.Second
	InitialSyncWaitTime  = 2 * time.Minute
	InitialSyncQueryTime = 5 * time.Second
)

func CreateSendIXFromIxData(t *testing.T, ixData *common.IxData, mnemonic string) *rpcargs.SendIX {
	t.Helper()

	bz, err := polo.Polorize(ixData)
	require.NoError(t, err)

	sign, err := crypto.GetSignature(bz, mnemonic)
	require.NoError(t, err)

	return &rpcargs.SendIX{
		IXArgs:    hex.EncodeToString(bz),
		Signature: sign,
	}
}

func GetContextNodes(t *testing.T, client *Client, addrs []identifiers.Address) []string {
	t.Helper()

	contextNodes := make([]string, 0)

	for _, addr := range addrs {
		resp, err := client.ContextInfo(context.Background(), &rpcargs.ContextInfoArgs{
			Address: addr,
			Options: rpcargs.TesseractNumberOrHash{
				TesseractNumber: &rpcargs.LatestTesseractHeight,
			},
		})
		require.NoError(t, err)

		contextNodes = append(contextNodes, resp.BehaviourNodes...)
		contextNodes = append(contextNodes, resp.RandomNodes...)
		contextNodes = append(contextNodes, resp.StorageNodes...)
	}

	return contextNodes
}

func GetLatestNonce(t *testing.T, client *Client, addr identifiers.Address) uint64 {
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

func GetLatestHeight(t *testing.T, client *Client, addr identifiers.Address) uint64 {
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
// and also checks if moi client response matches with http response
// Use this to check if interaction is successful on the chain.
func RetryFetchReceipt(t *testing.T, ctx context.Context, client *Client, ixHash common.Hash) *rpcargs.RPCReceipt {
	t.Helper()

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
func GetTesseract(t *testing.T, client *Client, addr identifiers.Address, height int64) *rpcargs.RPCTesseract {
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
func GetLogicID(t *testing.T, client *Client, txnID int, addr identifiers.Address, height int64) identifiers.LogicID {
	t.Helper()

	ts := GetTesseract(t, client, addr, height)

	receiptArgs := &rpcargs.ReceiptArgs{
		Hash: ts.Ixns[0].Hash,
	}

	receipt, err := client.InteractionReceipt(context.Background(), receiptArgs)
	require.NoError(t, err)

	var logicReceipt common.LogicDeployResult

	err = json.Unmarshal(receipt.IxOps[txnID].Data, &logicReceipt)
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

		depolorizedManifest, err := engineio.NewManifest(logicManifest, common.POLO)
		if err != nil {
			return nil, err
		}

		return depolorizedManifest.Encode(common.JSON)
	case "YAML":
		logicManifest := res.Bytes()

		depolorizedManifest, err := engineio.NewManifest(logicManifest, common.POLO)
		if err != nil {
			return nil, err
		}

		return depolorizedManifest.Encode(common.YAML)
	default:
		return nil, errors.New("invalid encoding type")
	}
}

type TokenLedgerState struct {
	Symbol   string
	Supply   *big.Int
	Balances map[identifiers.Address]*big.Int
}

func GetTokenLedgerState(t *testing.T, moiClient *Client,
	logicID identifiers.LogicID,
	addresses []identifiers.Address,
) TokenLedgerState {
	t.Helper()

	getLatestStorage := func(key [32]byte) hexutil.Bytes {
		s, err := moiClient.LogicStorage(context.Background(), &rpcargs.GetLogicStorageArgs{
			LogicID:    logicID,
			StorageKey: key[:],
			Options: rpcargs.TesseractNumberOrHash{
				TesseractNumber: &rpcargs.LatestTesseractHeight,
			},
		})
		require.NoError(t, err)

		return s
	}

	state := TokenLedgerState{
		Balances: make(map[identifiers.Address]*big.Int),
	}

	rawSymbol := getLatestStorage([32]byte(pisa.GenerateStorageKey(0)))
	err := polo.Depolorize(&state.Symbol, rawSymbol)
	require.NoError(t, err)

	rawSupply := getLatestStorage([32]byte(pisa.GenerateStorageKey(1)))
	err = polo.Depolorize(&state.Supply, rawSupply)
	require.NoError(t, err)

	for _, addr := range addresses {
		encoded, _ := polo.Polorize(addr)
		hashed := blake2b.Sum256(encoded)

		k := pisa.GenerateStorageKey(2, pisa.MapKey(hashed))
		rawBalance := getLatestStorage([32]byte(k))

		balance := new(big.Int)
		err = polo.Depolorize(balance, rawBalance)
		require.NoError(t, err)

		state.Balances[addr] = balance
	}

	return state
}

// GetPeerID returns a random Peer ID from the list of connected peers.
func GetPeerID(t *testing.T, client *Client) peer.ID {
	t.Helper()

	peers, err := client.Peers(context.Background(), &rpcargs.NetArgs{})

	require.NoError(t, err)
	require.True(t, len(peers) > 0)

	peerID, err := peers[rand.Intn(len(peers))].DecodedPeerID()
	require.NoError(t, err)

	return peerID
}

func NumPointer(input int64) *int64 {
	return &input
}

func GetJSONRPCUrls(t *testing.T, bgClient bgclient.Client, validatorCount int) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), JSONRPCURLWaitTime)
	defer cancel()

	jsonRPCUrls := make([]string, 0, validatorCount)

	var err error

	_, err = tests.RetryUntilTimeout(ctx, 100*time.Millisecond, func() (interface{}, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), JSONRPCURLQueryTime)
		defer cancel()

		jsonRPCUrls, err = bgClient.JSONRpcUrls(ctx)
		if err != nil {
			return nil, true
		}

		if len(jsonRPCUrls) != validatorCount {
			return nil, true
		}

		return nil, false
	})

	require.NoError(t, err)

	return jsonRPCUrls
}

func CheckIfNodesInitialSyncDone(t *testing.T, validatorCount int, jsonRPCUrls []string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), InitialSyncWaitTime)
	defer cancel()

	// number of goroutines
	numGoroutines := validatorCount / 10
	if validatorCount%10 != 0 {
		numGoroutines++
	}

	var wg sync.WaitGroup

	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		startIndex := i * 10
		endIndex := (i + 1) * 10

		if endIndex > validatorCount {
			endIndex = validatorCount
		}

		go func(start, end int) {
			defer wg.Done()

			for j := start; j < end; j++ {
				moiClient, err := NewClient(jsonRPCUrls[j])
				require.NoError(t, err)

				_, err = tests.RetryUntilTimeout(ctx, 50*time.Millisecond, func() (interface{}, bool) {
					ctx, cancel := context.WithTimeout(ctx, InitialSyncQueryTime)
					defer cancel()

					resp, err := moiClient.Syncing(ctx, &rpcargs.SyncStatusRequest{})
					if err != nil || !resp.NodeSyncResp.IsInitialSyncDone {
						return nil, true
					}

					return nil, false
				})

				require.NoError(t, err, jsonRPCUrls[j])
			}
		}(startIndex, endIndex)
	}

	// Wait for all goroutines to finish
	wg.Wait()
}

type StorageReader struct {
	client  *Client
	logicID identifiers.LogicID
	address identifiers.Address
}

func (c *Client) NewStorageReader(address identifiers.Address, logicID identifiers.LogicID) StorageReader {
	return StorageReader{client: c, logicID: logicID, address: address}
}

func (reader StorageReader) GetStorageEntry(key []byte) ([]byte, error) {
	content, err := reader.client.LogicStorage(context.Background(), &rpcargs.GetLogicStorageArgs{
		LogicID:    reader.logicID,
		Address:    reader.address,
		StorageKey: key,
		Options: rpcargs.TesseractNumberOrHash{
			TesseractNumber: &rpcargs.LatestTesseractHeight,
		},
	})
	if err != nil {
		return nil, err
	}

	return content, nil
}
