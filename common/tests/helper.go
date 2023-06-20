package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/jug/engineio"
	"github.com/sarvalabs/moichain/jug/pisa"
	"github.com/sarvalabs/moichain/mudra"
	mudracommon "github.com/sarvalabs/moichain/mudra/common"
	id "github.com/sarvalabs/moichain/mudra/kramaid"
	"github.com/sarvalabs/moichain/mudra/poi"
	"github.com/sarvalabs/moichain/mudra/poi/moinode"
	ptypes "github.com/sarvalabs/moichain/poorna/types"
	"github.com/sarvalabs/moichain/types"
)

const InvalidAccount types.AccountType = 9999

func RandomAddress(t *testing.T) types.Address {
	t.Helper()

	address := make([]byte, 32)

	if _, err := rand.Read(address); err != nil {
		t.Fatal(err)
	}

	return types.BytesToAddress(address)
}

func RandomAddressWithMnemonic(t *testing.T) (types.Address, string) {
	t.Helper()

	mnemonic := poi.GenerateRandMnemonic().String()

	_, publicKey, err := poi.GetPrivateKeyAtPath(mnemonic, common.DefaultMoiWalletPath)
	if err != nil {
		require.NoError(t, err)
	}

	return types.BytesToAddress(publicKey), mnemonic
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

	// Deriving MOI Id at m/44'/6174'/0'/0/0
	moiIDPrivKey := tempKey

	moiIDPath := new([3]uint32)
	moiIDPath[0] = id.HardenedStartIndex + 0 // m/44'/6174'/0'
	moiIDPath[1] = 0                         // m/44'/6174'/0'/0 ie., external
	moiIDPath[2] = 0                         // m/44'/6174'/0'/0/0

	for _, n := range moiIDPath {
		moiIDPrivKey, err = moiIDPrivKey.Derive(n)
		if err != nil {
			return nil, nil, err
		}
	}

	moiPubKeyPoint, err := moiIDPrivKey.Neuter()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubInSecp256k1, err := moiPubKeyPoint.ECPubKey()
	if err != nil {
		return nil, nil, err
	}

	moiIDPubBytes := moiIDPubInSecp256k1.SerializeCompressed()

	// to persist consensus and network private keys
	var aggPrivKey []byte
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

func GetRandomUpperCaseString(t *testing.T, length int) string {
	t.Helper()

	const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	randomString := make([]byte, length)

	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(characters))))
		require.NoError(t, err)

		randomString[i] = characters[num.Int64()]
	}

	return string(randomString)
}

func GetRandomAssetInfo(t *testing.T, addr types.Address) *types.AssetDescriptor {
	t.Helper()

	symbol := GetRandomUpperCaseString(t, 5)

	if addr.IsNil() {
		addr = RandomAddress(t)
	}

	asset := &types.AssetDescriptor{
		Operator:   addr,
		Dimension:  1,
		Supply:     big.NewInt(1000),
		Symbol:     symbol,
		IsStateFul: true,
		IsLogical:  false,
		LogicID:    types.LogicID(RandomHash(t).String()),
	}

	return asset
}

func CreateTestAsset(t *testing.T, address types.Address) (types.AssetID, *types.AssetDescriptor) {
	t.Helper()

	asset := GetRandomAssetInfo(t, RandomAddress(t))

	assetID := types.NewAssetIDv0(asset.IsLogical, asset.IsStateFul, asset.Dimension, asset.Standard, address)

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

	assetID, _ := CreateTestAsset(t, address)

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

	return types.NewTesseract(header, body, ixns, nil, []byte{1}, GetTestKramaID(t, 0))
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
	Sealer         id.KramaID
	Seal           []byte
	ClusterID      string
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

	header := &types.TesseractHeader{
		Address:   params.Address,
		Height:    params.Height,
		FuelUsed:  big.NewInt(100),
		FuelLimit: big.NewInt(100),
		ClusterID: params.ClusterID,
	}

	if params.Ixns != nil {
		hash, err := params.Ixns.Hash()
		require.NoError(t, err)

		interactionHash = hash
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

	return types.NewTesseract(*header, *body, params.Ixns, params.Receipts, params.Seal, params.Sealer)
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

	return ts.Hash()
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
	Sign           []byte
}

func CreateIX(t *testing.T, params *CreateIxParams) *types.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &types.IxData{
		Input: types.IxInput{
			Type: types.IxValueTransfer,
			TransferValues: map[types.AssetID]*big.Int{
				types.AssetID("add"): big.NewInt(1),
			},
		},
	}

	if params.IxDataCallback != nil {
		params.IxDataCallback(data)
	}

	if len(params.Sign) == 0 {
		params.Sign = []byte{}
	}

	ix, err := types.NewInteraction(*data, params.Sign)
	require.NoError(t, err)

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
		Sign: nil,
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

// HeaderCallbackWithGridHash returns callback which assigns extra field with new commit data having random grid hash
func HeaderCallbackWithGridHash(t *testing.T) func(header *types.TesseractHeader) {
	t.Helper()

	return func(header *types.TesseractHeader) {
		header.Extra = types.CommitData{
			GridID: &types.TesseractGridID{
				Hash:  RandomHash(t),
				Parts: &types.TesseractParts{},
			},
		}
	}
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
			Height:         uint64(i),
			HeaderCallback: HeaderCallbackWithGridHash(t),
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
		Total: 2,
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
		FuelUsed:    big.NewInt(5),
		FuelLimit:   big.NewInt(6),
		BodyHash:    RandomHash(t),
		GroupHash:   RandomHash(t),
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

func CheckIfPartsSorted(t *testing.T, parts ptypes.RPCTesseractParts) {
	t.Helper()

	for i := 1; i < len(parts); i++ {
		require.True(t, parts[i-1].Address.Hex() < parts[i].Address.Hex())
	}
}

func SignBytes(t *testing.T, msg []byte) (sigBytes, pk []byte) {
	t.Helper()

	// create keystore.json in current directory
	dataDir := "./"
	password := "test123"

	_, _, err := poi.RandGenKeystore(dataDir, password)
	require.NoError(t, err)

	config := &mudra.VaultConfig{
		DataDir:      dataDir,
		NodePassword: password,
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

// ReadManifest Reads the manifest file at the given
// filepath and returns it as POLO encoded hex string
func ReadManifest(t *testing.T, filePath string) []byte {
	t.Helper()

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngineRuntime(pisa.NewRuntime())

	// Decode the manifest into a Manifest object
	manifest, err := engineio.ReadManifestFile(filePath)
	require.NoError(t, err)

	// Encode the Manifest into POLO data
	encoded, err := manifest.Encode(engineio.POLO)
	require.NoError(t, err)

	return encoded
}

func GetManifests(t *testing.T, filepath string) (poloEncoded, jsonEncoded, yamlEncoded []byte) {
	t.Helper()

	// Register the PISA element registry with the EngineIO package
	engineio.RegisterEngineRuntime(pisa.NewRuntime())

	// Read manifest at file path
	manifest, err := engineio.ReadManifestFile(filepath)
	require.NoError(t, err)

	// Encode the Manifest into POLO data
	poloEncoded, err = manifest.Encode(engineio.POLO)
	require.NoError(t, err)

	// Encode the Manifest into JSON data
	jsonEncoded, err = manifest.Encode(engineio.JSON)
	require.NoError(t, err)

	// Encode the Manifest into YAML data
	yamlEncoded, err = manifest.Encode(engineio.YAML)
	require.NoError(t, err)

	return
}

func CreateIXInputWithTestData(
	t *testing.T,
	ixType types.IxType,
	payload []byte,
	perceivedProofs []byte,
) types.IxInput {
	t.Helper()

	IxInput := types.IxInput{
		Type:  ixType,
		Nonce: 2,

		Sender:   RandomAddress(t),
		Receiver: RandomAddress(t),
		Payer:    RandomAddress(t),

		TransferValues: map[types.AssetID]*big.Int{
			"0180127603f47e5aff68052402fda5c4364e8e6cff1e107e4e821af00d0eee2edf16": big.NewInt(1033),
			"0180127603f47e5aff68052402fda5c4364e8e6cff1e107e4e821af00d0eee2edf15": big.NewInt(1093),
		},
		PerceivedValues: map[types.AssetID]*big.Int{
			"0180127603f47e5aff68053102fda5c4364e8e6cff1e107e4e821af00d0eee2edf16": big.NewInt(1233),
			"0180127603f47e5aff68053102fda5c4364e8e6cff1e107e4e821af00d0eee2ed416": big.NewInt(1333),
		},
		PerceivedProofs: perceivedProofs,

		FuelLimit: new(big.Int).SetUint64(1043),
		FuelPrice: new(big.Int).SetUint64(1),

		Payload: payload,
	}

	return IxInput
}

func CreateComputeWithTestData(t *testing.T, hash types.Hash, kramaIDs []id.KramaID) types.IxCompute {
	t.Helper()

	IxCompute := types.IxCompute{
		Mode:         3,
		Hash:         hash,
		ComputeNodes: kramaIDs,
	}

	return IxCompute
}

func CreateTrustWithTestData(t *testing.T) types.IxTrust {
	t.Helper()

	IxTrust := types.IxTrust{
		MTQ:        8,
		TrustNodes: GetTestKramaIDs(t, 2),
	}

	return IxTrust
}

func CreateReceiptWithTestData(t *testing.T) *types.Receipt {
	t.Helper()

	receipt := &types.Receipt{
		IxType:    2,
		IxHash:    RandomHash(t),
		FuelUsed:  big.NewInt(99),
		Hashes:    make(types.ReceiptAccHashes),
		ExtraData: []byte{1, 2},
	}

	for i := 0; i < 3; i++ {
		receipt.Hashes[RandomAddress(t)] = &types.Hashes{
			StateHash:   RandomHash(t),
			ContextHash: RandomHash(t),
		}
	}

	return receipt
}

func CreateBodyWithTestData(t *testing.T) types.TesseractBody {
	t.Helper()

	body := types.TesseractBody{
		StateHash:       RandomHash(t),
		ContextHash:     RandomHash(t),
		InteractionHash: RandomHash(t),
		ReceiptHash:     RandomHash(t),
		ContextDelta:    make(map[types.Address]*types.DeltaGroup),
		ConsensusProof: types.PoXCData{
			BinaryHash:   RandomHash(t),
			IdentityHash: RandomHash(t),
			ICSHash:      RandomHash(t),
		},
	}

	body.ContextDelta[RandomAddress(t)] = &types.DeltaGroup{
		BehaviouralNodes: GetTestKramaIDs(t, 2),
	}

	return body
}

func WriteToAccountsFile(filePath string, accounts []AccountWithMnemonic) error {
	file, err := json.MarshalIndent(accounts, "", "\t")
	if err != nil {
		return err
	}

	if err := ioutil.WriteFile(filePath, file, os.ModePerm); err != nil {
		return err
	}

	fmt.Println("Accounts file created")

	return nil
}

func GetAddressFromAccountsFile(filePath string) ([]string, error) {
	accounts := make([]AccountWithMnemonic, 0)
	addresses := make([]string, 0)

	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(file, &accounts); err != nil {
		return nil, err
	}

	for index := range accounts {
		addresses = append(addresses, accounts[index].Addr.String())
	}

	return addresses, nil
}

func GetAccountMnemonicsFromFile(filePath string) ([]AccountWithMnemonic, error) {
	accounts := make([]AccountWithMnemonic, 0)

	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(file, &accounts); err != nil {
		return nil, err
	}

	return accounts, nil
}

func GetIXSignature(t *testing.T, ixArgs *types.SendIXArgs, mnemonic string) []byte {
	t.Helper()

	rawIX, err := ixArgs.Bytes()
	require.NoError(t, err)

	sign, err := mudra.GetSignature(rawIX, mnemonic)
	require.NoError(t, err)

	rawSign, err := hex.DecodeString(sign)
	require.NoError(t, err)

	return rawSign
}
