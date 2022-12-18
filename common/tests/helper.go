package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"

	gtypes "github.com/sarvalabs/moichain/guna/types"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/types"
)

func RandomAddress(t *testing.T) types.Address {
	t.Helper()

	address := make([]byte, 32)

	if _, err := rand.Read(address); err != nil {
		t.Fatal(err)
	}

	return types.BytesToAddress(address)
}

func RandomHash(t *testing.T) types.Hash {
	t.Helper()

	hash := make([]byte, 32)

	if _, err := rand.Read(hash); err != nil {
		t.Fatal(err)
	}

	return types.BytesToHash(hash)
}

func GetTestKramaIDs(t *testing.T, count int) []id.KramaID {
	t.Helper()

	ids := make([]id.KramaID, 0, count)

	for i := 0; i < count; i++ {
		var signKey [32]byte

		_, err := rand.Read(signKey[:])
		require.NoError(t, err)

		privateKeys, moiPubBytes, err := getPrivKeysForTest(signKey[:])
		require.NoError(t, err)

		kramaID, err := id.NewKramaID(
			privateKeys[32:],
			uint32(i),
			hex.EncodeToString(moiPubBytes),
			1,
			true,
		)
		require.NoError(t, err)

		ids = append(ids, kramaID)
	}

	return ids
}

func RetryUntilTimeout(ctx context.Context, f func() (interface{}, bool)) (interface{}, error) {
	type result struct {
		data interface{}
		err  error
	}

	resCh := make(chan result, 1)

	go func() {
		defer close(resCh)

		for {
			select {
			case <-ctx.Done():
				resCh <- result{nil, errors.New("timeout")}

				return
			default:
				res, retry := f()
				if !retry {
					resCh <- result{res, nil}

					return
				}
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	res := <-resCh

	return res.data, res.err
}

func getPrivKeysForTest(seed []byte) ([]byte, []byte, error) {
	// Let's derive 'm' in the path
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams) // here key is master key
	if err != nil {
		return nil, nil, err
	}

	/* Deriving MOI id address */
	masPubKey, err := masterKey.Neuter()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubInSecp256k1, err := masPubKey.ECPubKey()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubBytes := moiIDPubInSecp256k1.SerializeCompressed()

	// Hardened keys index starts from 2147483648 (2^31)
	// So.,
	// 44 = 2147483648 + 44 = 2147483692
	// 6174 = 2147483648 + 6174 = 2147489822
	igcParams := [2]uint32{2147483692, 2147489822}

	tempKey := masterKey
	for _, n := range igcParams {
		tempKey, err = tempKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}
	// Now tempKey points to extended private key at path: m/44'/6174'

	var aggPrivKey []byte // to concat both private keys

	// Let's derive PrivateKey for signing, so load keyPair at path: m/44'/6174'/5020'/0/n
	validatorPrivKey := tempKey

	var validatorPath [3]uint32
	validatorPath[0] = id.HardenedStartIndex + 5020 // hardened
	validatorPath[1] = 0
	validatorPath[2] = 0

	for _, n := range validatorPath {
		validatorPrivKey, err = validatorPrivKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}
	// Now validatorPrivKey points to extended private key at path: m/44'/6174'/5020'/0/n

	// Casting to Elliptic curve Private key
	privKeyInEC, err := validatorPrivKey.ECPrivKey()
	if err != nil {
		return nil, nil, err
	}

	signingPrivKeyInBytes := privKeyInEC.Serialize()

	aggPrivKey = append(aggPrivKey, signingPrivKeyInBytes...)

	// Let's derive PrivateKey for communication, so load keyPair at path: m/44'/6174'/6020'/0/n
	ntwPrivKey := tempKey

	var networkPath [3]uint32
	networkPath[0] = id.HardenedStartIndex + 6020 // hardened
	networkPath[1] = 0
	networkPath[2] = 0

	for _, n := range networkPath {
		ntwPrivKey, err = ntwPrivKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}
	// Now ntwPrivKey points to extended private key at path: m/44'/6174'/6020'/0/n

	// Casting to Elliptic curve Private key
	nPrivKeyInEC, err := ntwPrivKey.ECPrivKey()
	if err != nil {
		return nil, nil, err
	}

	ntwPrivKeyInBytes := nPrivKeyInEC.Serialize()

	aggPrivKey = append(aggPrivKey, ntwPrivKeyInBytes...)

	return aggPrivKey, moiIDPubBytes, nil
}

func GetRandomUpperCaseString(t *testing.T, length int) (string, error) {
	t.Helper()

	const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	randomString := make([]byte, length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(characters))))
		if err != nil {
			return "", err
		}

		randomString[i] = characters[num.Int64()]
	}

	return string(randomString), nil
}

func getRandomAssetInfo(t *testing.T) (*types.AssetDescriptor, error) {
	t.Helper()

	symbol, err := GetRandomUpperCaseString(t, 5)
	if err != nil {
		return nil, err
	}

	asset := &types.AssetDescriptor{
		Owner:      RandomAddress(t),
		Dimension:  1,
		Supply:     big.NewInt(1000),
		Symbol:     symbol,
		IsFungible: true,
		IsMintable: false,
		LogicID:    types.LogicID(RandomHash(t).String()),
	}

	return asset, nil
}

func CreateTestAsset(t *testing.T, address types.Address) (types.AssetID, *types.AssetDescriptor) {
	t.Helper()

	asset, err := getRandomAssetInfo(t)
	if err != nil {
		log.Panic("Failed to create asset")
	}

	asset.Owner = address
	assetID, _, _, err := gtypes.GetAssetID(asset)
	require.NoError(t, err)

	return assetID, asset
}

func GetRandomAssetID(t *testing.T, address types.Address) types.AssetID {
	t.Helper()

	asset, err := getRandomAssetInfo(t)
	if err != nil {
		log.Panic("Failed to create asset")
	}

	asset.Owner = address
	assetID, _, _, err := gtypes.GetAssetID(asset)
	require.NoError(t, err)

	return assetID
}

func GetTesseract(t *testing.T, height uint64) *types.Tesseract {
	t.Helper()

	header := types.TesseractHeader{
		Address:  RandomAddress(t),
		PrevHash: RandomHash(t),
		Height:   height,
	}
	body := types.TesseractBody{}
	tesseract := types.Tesseract{
		Header: header,
		Body:   body,
		Seal:   []byte{1},
	}

	return &tesseract
}

func GetRandomAddressList(t *testing.T, count uint8) []types.Address {
	t.Helper()

	address := make([]types.Address, count)

	for i := uint8(0); i < count; i++ {
		address[i] = RandomAddress(t)
	}

	return address
}
