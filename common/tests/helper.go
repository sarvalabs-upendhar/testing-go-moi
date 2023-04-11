package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gtypes "github.com/sarvalabs/moichain/guna/types"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
	"github.com/sarvalabs/moichain/mudra"
	mudracommon "github.com/sarvalabs/moichain/mudra/common"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
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

func GetTestKramaID(t *testing.T, nthValidator uint32) id.KramaID {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:])
	require.NoError(t, err)

	privateKeys, moiPubBytes, err := GetPrivKeysForTest(signKey[:])
	require.NoError(t, err)

	kramaID, err := id.NewKramaID(
		privateKeys[32:],
		nthValidator,
		hex.EncodeToString(moiPubBytes),
		1,
		true,
	)
	require.NoError(t, err)

	return kramaID
}

func GetTestKramaIDs(t *testing.T, count int) []id.KramaID {
	t.Helper()

	ids := make([]id.KramaID, 0, count)

	for i := 0; i < count; i++ {
		ids = append(ids, GetTestKramaID(t, uint32(i)))
	}

	return ids
}

func GetTestPeerID(t *testing.T) peer.ID {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:])
	require.NoError(t, err)

	privateKeys, _, err := GetPrivKeysForTest(signKey[:])
	require.NoError(t, err)

	peerID, err := id.GeneratePeerID(privateKeys[32:])
	require.NoError(t, err)

	return peerID
}

func DecodePeerIDFromKramaID(t *testing.T, kramaID id.KramaID) peer.ID {
	t.Helper()

	peerID, err := kramaID.DecodedPeerID()
	require.NoError(t, err)

	return peerID
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

func GetPrivKeysForTest(seed []byte) ([]byte, []byte, error) {
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

func GetRandomAssetInfo(t *testing.T) (*types.AssetDescriptor, error) {
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

	asset, err := GetRandomAssetInfo(t)
	if err != nil {
		log.Panic("Failed to create asset")
	}

	asset.Owner = address
	assetID, _, _, err := gtypes.GetAssetID(asset)
	require.NoError(t, err)

	return assetID, asset
}

func GetRandomNumbers(t *testing.T, max int, count int) []*big.Int {
	t.Helper()

	var err error

	numbers := make([]*big.Int, count)

	for i := 0; i < count; i++ {
		numbers[i], err = rand.Int(rand.Reader, big.NewInt(int64(max)))
		require.NoError(t, err)
	}

	return numbers
}

func GetRandomAssetID(t *testing.T, address types.Address) types.AssetID {
	t.Helper()

	asset, err := GetRandomAssetInfo(t)
	if err != nil {
		log.Panic("Failed to create asset")
	}

	asset.Owner = address
	assetID, _, _, err := gtypes.GetAssetID(asset)
	require.NoError(t, err)

	return assetID
}

func GetAvailablePort(t *testing.T) (port int, err error) {
	t.Helper()

	var address *net.TCPAddr

	if address, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var listener *net.TCPListener

		if listener, err = net.ListenTCP("tcp", address); err == nil {
			defer func() {
				if err := listener.Close(); err != nil {
					return
				}
			}()

			tcpAddr, ok := listener.Addr().(*net.TCPAddr)
			require.Equal(t, ok, true)

			port = tcpAddr.Port

			return port, nil
		}
	}

	return
}

// GetListenAddresses returns a new multi-address on localhost associated with an empty port.
func GetListenAddresses(t *testing.T, count int) []multiaddr.Multiaddr {
	t.Helper()

	ListenAddresses := make([]multiaddr.Multiaddr, count)

	for i := 0; i < count; i++ {
		port, err := GetAvailablePort(t)
		require.NoError(t, err)

		ListenAddresses[i], err = multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
		require.NoError(t, err)
	}

	return ListenAddresses
}

func GetTesseract(t *testing.T, height uint64, ixns types.Interactions) *types.Tesseract {
	t.Helper()

	header := types.TesseractHeader{
		Address:  RandomAddress(t),
		PrevHash: RandomHash(t),
		Height:   height,
	}
	body := types.TesseractBody{}

	return types.NewTesseract(header, body, ixns, nil, []byte{1})
}

func GetRandomAccMetaInfo(t *testing.T, height uint64) *types.AccountMetaInfo {
	t.Helper()

	return &types.AccountMetaInfo{
		Address:       RandomAddress(t),
		Type:          types.AccountType(1),
		Height:        height,
		TesseractHash: RandomHash(t),
		LatticeExists: true,
		StateExists:   true,
	}
}

func GetTestPublicKey(t *testing.T) []byte {
	t.Helper()

	return RandomAddress(t).Bytes()
}

func GetTestPublicKeys(t *testing.T, count int) [][]byte {
	t.Helper()

	p := make([][]byte, 0)

	for i := 0; i < count; i++ {
		addr := GetTestPublicKey(t)
		p = append(p, addr)
	}

	return p
}

func GetTestKramaIdsWithPublicKeys(t *testing.T, count int) ([]id.KramaID, [][]byte) {
	t.Helper()

	return GetTestKramaIDs(t, count), GetTestPublicKeys(t, count)
}

func GetRandomAddressList(t *testing.T, count uint8) []types.Address {
	t.Helper()

	address := make([]types.Address, count)

	for i := uint8(0); i < count; i++ {
		address[i] = RandomAddress(t)
	}

	return address
}

type CreateTesseractParams struct {
	Address        types.Address
	Height         uint64
	Ixns           types.Interactions
	Receipts       types.Receipts
	Seal           []byte
	HeaderCallback func(header *types.TesseractHeader)
	BodyCallback   func(body *types.TesseractBody)
}

// CreateTesseract creates a tesseract using tessseract params fields
// if any field thats not available in params need to be initialized using TesseractCallback field
func CreateTesseract(t *testing.T, params *CreateTesseractParams) *types.Tesseract {
	t.Helper()

	if params == nil {
		params = &CreateTesseractParams{}
	}

	if params.Address.IsNil() {
		params.Address = RandomAddress(t)
	}

	var interactionHash types.Hash

	if params.Ixns != nil {
		hash, err := params.Ixns.Hash()
		require.NoError(t, err)

		interactionHash = hash
	}

	header := &types.TesseractHeader{
		Address: params.Address,
		Height:  params.Height,
	}

	body := &types.TesseractBody{
		InteractionHash: interactionHash,
	}

	if params.HeaderCallback != nil {
		params.HeaderCallback(header)
	}

	if params.BodyCallback != nil {
		params.BodyCallback(body)
	}

	return types.NewTesseract(*header, *body, params.Ixns, params.Receipts, params.Seal)
}

func CreateTesseracts(t *testing.T, count int, paramsMap map[int]*CreateTesseractParams) []*types.Tesseract {
	t.Helper()

	tesseracts := make([]*types.Tesseract, count)

	if paramsMap == nil {
		paramsMap = map[int]*CreateTesseractParams{}
	}

	for i := 0; i < count; i++ {
		if paramsMap[i] == nil {
			paramsMap[i] = &CreateTesseractParams{
				Height: uint64(i),
			}
		}

		tesseracts[i] = CreateTesseract(t, paramsMap[i])
	}

	return tesseracts
}

func GetTesseractHash(t *testing.T, ts *types.Tesseract) types.Hash {
	t.Helper()

	hash, err := ts.Hash()
	require.NoError(t, err)

	return hash
}

func GetAddresses(t *testing.T, count int) []types.Address {
	t.Helper()

	addresses := make([]types.Address, count)
	for i := 0; i < count; i++ {
		addresses[i] = RandomAddress(t)
	}

	return addresses
}

type CreateIxParams struct {
	IxDataCallback func(ix *types.IxData)
}

func CreateIX(t *testing.T, params *CreateIxParams) *types.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &types.IxData{
		Input: types.IxInput{},
	}

	if params.IxDataCallback != nil {
		params.IxDataCallback(data)
	}

	ix := types.NewInteraction(*data, []byte{})

	return ix
}

func CreateIxns(t *testing.T, count int, paramsMap map[int]*CreateIxParams) types.Interactions {
	t.Helper()

	if paramsMap == nil {
		paramsMap = map[int]*CreateIxParams{}
	}

	ixns := make(types.Interactions, count)

	for i := 0; i < count; i++ {
		ixns[i] = CreateIX(t, paramsMap[i])
	}

	return ixns
}

func GetIxParamsWithAddress(from types.Address, to types.Address) *CreateIxParams {
	return &CreateIxParams{
		IxDataCallback: func(ix *types.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
		},
	}
}

func GetIxParamsMapWithAddresses(
	from []types.Address,
	to []types.Address,
) map[int]*CreateIxParams {
	count := len(from)
	ixParams := make(map[int]*CreateIxParams, count)

	for i := 0; i < count; i++ {
		ixParams[i] = GetIxParamsWithAddress(from[i], to[i])
	}

	return ixParams
}

// GetTesseractParamsMapWithIxns returns tsCount no.of tesseracts and each one will have ixnCount interactions
func GetTesseractParamsMapWithIxns(t *testing.T, tsCount, ixnCount int) map[int]*CreateTesseractParams {
	t.Helper()

	tesseractParams := make(map[int]*CreateTesseractParams, tsCount)
	addresses := GetAddresses(t, 2*(tsCount-1)*ixnCount) // for each interaction, sender and receiver addresses needed
	ixns := CreateIxns(
		t,
		(tsCount-1)*ixnCount,
		GetIxParamsMapWithAddresses(addresses[:2*(tsCount-1)], addresses[2*(tsCount-1):]),
	)

	// allocate interactions to each tesseract, excluding the first tesseract (which is the genesis tesseract)
	for i := 0; i < tsCount; i++ {
		tesseractParams[i] = &CreateTesseractParams{
			Height: uint64(i),
		}

		if i > 0 {
			// allocate two interactions per tesseract
			tesseractParams[i].Ixns = ixns[(i-1)*ixnCount : (i-1)*ixnCount+ixnCount]
		}
	}

	return tesseractParams
}

func GetTestAccount(t *testing.T, callBack func(acc *types.Account)) (*types.Account, types.Hash) {
	t.Helper()

	acc := new(types.Account)
	if callBack != nil {
		callBack(acc)
	}

	accHash, err := acc.Hash()
	assert.NoError(t, err)

	return acc, accHash
}

func CreateTesseractPartsWithTestData(t *testing.T) *types.TesseractParts {
	t.Helper()

	parts := &types.TesseractParts{
		Total: 3,
		Grid:  make(map[types.Address]types.TesseractHeightAndHash),
	}

	for i := 0; i < 2; i++ {
		parts.Grid[RandomAddress(t)] = types.TesseractHeightAndHash{
			Height: 3,
			Hash:   RandomHash(t),
		}
	}

	return parts
}

func CreateCommitDataWithTestData(t *testing.T) types.CommitData {
	t.Helper()

	return types.CommitData{
		Round:           4,
		CommitSignature: []byte{1, 2, 3},
		VoteSet: &types.ArrayOfBits{
			Elements: []uint64{4, 4},
		},
		EvidenceHash: RandomHash(t),
		GridID: &types.TesseractGridID{
			Hash:  RandomHash(t),
			Parts: CreateTesseractPartsWithTestData(t),
		},
	}
}

func CreateHeaderWithTestData(t *testing.T) types.TesseractHeader {
	t.Helper()

	header := types.TesseractHeader{
		Address:     RandomAddress(t),
		PrevHash:    RandomHash(t),
		Height:      4444,
		FuelUsed:    5,
		FuelLimit:   6,
		BodyHash:    RandomHash(t),
		GridHash:    RandomHash(t),
		Operator:    "operator",
		ClusterID:   "cluster-ID",
		Timestamp:   1,
		ContextLock: make(map[types.Address]types.ContextLockInfo),
		Extra:       CreateCommitDataWithTestData(t),
	}

	header.ContextLock[RandomAddress(t)] = types.ContextLockInfo{
		TesseractHash: RandomHash(t),
	}

	return header
}

func CheckForTesseract(t *testing.T, expectedTS, actualTS *types.Tesseract, withInteractions bool) {
	t.Helper()

	if withInteractions {
		require.Equal(t, expectedTS, actualTS)

		return
	}

	require.Equal(t, expectedTS.Canonical(), actualTS.Canonical())
	require.Nil(t, actualTS.Interactions())
}

func SignBytes(t *testing.T, msg []byte) (sigBytes, pk []byte) {
	t.Helper()

	// create keystore.json in current directory
	dataDir := "./"
	password := "test123"

	_, _, err := poi.RandGenKeystore(dataDir, password)
	require.NoError(t, err)

	config := &mudra.VaultConfig{
		DataDir:       dataDir,
		NodePassword:  password,
		MoiIDUsername: "",
		MoiIDPassword: "",
		MoiIDURL:      "dev",
	}

	vault, err := mudra.NewVault(config, moinode.MoiFullNode, 1)
	require.NoError(t, err)

	// gets the public key of signer
	pk = vault.GetConsensusPrivateKey().GetPublicKeyInBytes()

	// signs the bytes
	sigBytes, err = vault.Sign(msg, mudracommon.BlsBLST)
	require.NoError(t, err)

	// remove keystore.json in current directory
	err = os.Remove("./keystore.json")
	require.NoError(t, err)

	return sigBytes, pk
}

// Reads the ERC20 JSON Manifest and returns it as POLO encoded hex string
func ReadERC20Manifest(t *testing.T) []byte {
	t.Helper()

	// Read erc20.json manifest from jug/manifests
	data, err := ioutil.ReadFile("./../../jug/manifests/erc20.json")
	require.NoError(t, err)

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
	// Decode the JSON manifest into a Manifest object
	manifest, err := engineio.NewManifest(data, engineio.JSON)
	require.NoError(t, err)

	// Encode the Manifest into POLO data
	encoded, err := manifest.Encode(engineio.POLO)
	require.NoError(t, err)

	return encoded
}

func GetJSONManifest(t *testing.T) []byte {
	t.Helper()

	// Read erc20.json manifest from jug/manifests
	data, err := ioutil.ReadFile("./../../jug/manifests/erc20.json")
	require.NoError(t, err)

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
	// Decode the JSON manifest into a Manifest object
	manifest, err := engineio.NewManifest(data, engineio.JSON)
	require.NoError(t, err)

	// Encode the Manifest into POLO data
	encoded, err := manifest.Encode(engineio.JSON)
	require.NoError(t, err)

	return encoded
}

func GetYAMLManifest(t *testing.T) []byte {
	t.Helper()

	// Read erc20.json manifest from jug/manifests
	data, err := ioutil.ReadFile("./../../jug/manifests/erc20.json")
	require.NoError(t, err)

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngineRuntime(pisa.NewRuntime())
	// Decode the JSON manifest into a Manifest object
	manifest, err := engineio.NewManifest(data, engineio.YAML)
	require.NoError(t, err)

	// Encode the Manifest into POLO data
	encoded, err := manifest.Encode(engineio.YAML)
	require.NoError(t, err)

	return encoded
}

/*
// Unused functions
func GetInvalidHash(t *testing.T) string {
	t.Helper()
	randomHash := RandomHash(t).String()

	randmath.Seed(time.Now().UnixNano())
	randomNum := randmath.Intn(62)
	randAlphabet := 'g' + randmath.Intn(17)

	return randomHash[:randomNum] + string(rune(randAlphabet)) + randomHash[randomNum+1:]
}

func GetInvalidAddress(t *testing.T) string {
	t.Helper()
	randomHash := RandomHash(t).String()

	randmath.Seed(time.Now().UnixNano())
	randomNum := randmath.Intn(62)
	randAlphabet := 'g' + randmath.Intn(17)

	return randomHash[:randomNum] + string(rune(randAlphabet)) + randomHash[randomNum+1:]
}
*/
