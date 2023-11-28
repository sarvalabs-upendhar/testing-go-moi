package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	engineio "github.com/sarvalabs/go-moi-engineio"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/sarvalabs/go-pisa"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/common/kramaid"
	"github.com/sarvalabs/go-moi/crypto"
	mudracommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
)

const InvalidAccount common.AccountType = 9999

func RandomAddress(t *testing.T) common.Address {
	t.Helper()

	address := make([]byte, 32)

	if _, err := rand.Read(address); err != nil {
		t.Fatal(err)
	}

	return common.BytesToAddress(address)
}

func RandomAddressWithMnemonic(t *testing.T) (common.Address, string) {
	t.Helper()

	mnemonic := poi.GenerateRandMnemonic().String()

	_, publicKey, err := poi.GetPrivateKeyAtPath(mnemonic, config.DefaultMoiWalletPath)
	if err != nil {
		require.NoError(t, err)
	}

	return common.BytesToAddress(publicKey), mnemonic
}

func RandomHash(t *testing.T) common.Hash {
	t.Helper()

	hash := make([]byte, 32)

	if _, err := rand.Read(hash); err != nil {
		t.Fatal(err)
	}

	return common.BytesToHash(hash)
}

func GetTestKramaID(t *testing.T, nthValidator uint32) kramaid.KramaID {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:])
	require.NoError(t, err)

	privateKeys, moiPubBytes, err := GetPrivKeysForTest(signKey[:])
	require.NoError(t, err)

	kramaID, err := kramaid.NewKramaID(
		privateKeys[32:],
		nthValidator,
		hex.EncodeToString(moiPubBytes),
		1,
		true,
	)
	require.NoError(t, err)

	return kramaID
}

func GetTestKramaIDs(t *testing.T, count int) []kramaid.KramaID {
	t.Helper()

	ids := make([]kramaid.KramaID, 0, count)

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

	peerID, err := kramaid.GeneratePeerID(privateKeys[32:])
	require.NoError(t, err)

	return peerID
}

func GetTestPeerIDs(t *testing.T, count int) []peer.ID {
	t.Helper()

	peerIDs := make([]peer.ID, 0)

	for i := 0; i < count; i++ {
		peerIDs = append(peerIDs, GetTestPeerID(t))
	}

	return peerIDs
}

func DecodePeerIDFromKramaID(t *testing.T, kramaID kramaid.KramaID) peer.ID {
	t.Helper()

	peerID, err := kramaID.DecodedPeerID()
	require.NoError(t, err)

	return peerID
}

func RetryUntilTimeout(ctx context.Context, delay time.Duration, f func() (interface{}, bool)) (interface{}, error) {
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
			time.Sleep(delay)
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
	moiIDPath[0] = kramaid.HardenedStartIndex + 0 // m/44'/6174'/0'
	moiIDPath[1] = 0                              // m/44'/6174'/0'/0 ie., external
	moiIDPath[2] = 0                              // m/44'/6174'/0'/0/0

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
	validatorPath[0] = kramaid.HardenedStartIndex + 5020 // hardened
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
	networkPath[0] = kramaid.HardenedStartIndex + 6020 // hardened
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

func GetRandomAssetInfo(t *testing.T, addr common.Address) *common.AssetDescriptor {
	t.Helper()

	symbol := GetRandomUpperCaseString(t, 5)

	if addr.IsNil() {
		addr = RandomAddress(t)
	}

	asset := &common.AssetDescriptor{
		Operator:   addr,
		Dimension:  1,
		Supply:     big.NewInt(1000),
		Symbol:     symbol,
		IsStateFul: true,
		IsLogical:  false,
		LogicID:    common.LogicID(RandomHash(t).String()),
	}

	return asset
}

func CreateTestAsset(t *testing.T, address common.Address) (common.AssetID, *common.AssetDescriptor) {
	t.Helper()

	asset := GetRandomAssetInfo(t, address)

	assetID := common.NewAssetIDv0(asset.IsLogical, asset.IsStateFul, asset.Dimension, asset.Standard, address)

	return assetID, asset
}

func CreateTestAssets(t *testing.T, count int) ([]common.AssetID, []*common.AssetDescriptor) {
	t.Helper()

	assetIDs := make([]common.AssetID, 0, count)
	assetDescriptors := make([]*common.AssetDescriptor, 0, count)

	for i := 0; i < count; i++ {
		assetID, assetDescriptor := CreateTestAsset(t, RandomAddress(t))

		assetIDs = append(assetIDs, assetID)
		assetDescriptors = append(assetDescriptors, assetDescriptor)
	}

	return assetIDs, assetDescriptors
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

func GetRandomAssetID(t *testing.T, address common.Address) common.AssetID {
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

func GetTesseract(t *testing.T, height uint64, ixns common.Interactions) *common.Tesseract {
	t.Helper()

	header := common.TesseractHeader{
		Address:  RandomAddress(t),
		PrevHash: RandomHash(t),
		Height:   height,
	}
	body := common.TesseractBody{}

	return common.NewTesseract(header, body, ixns, nil, []byte{1}, GetTestKramaID(t, 0))
}

func GetRandomAccMetaInfo(t *testing.T, height uint64) *common.AccountMetaInfo {
	t.Helper()

	return &common.AccountMetaInfo{
		Address:       RandomAddress(t),
		Type:          common.AccountType(1),
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

func GetTestKramaIdsWithPublicKeys(t *testing.T, count int) ([]kramaid.KramaID, [][]byte) {
	t.Helper()

	return GetTestKramaIDs(t, count), GetTestPublicKeys(t, count)
}

func GetRandomAddressList(t *testing.T, count uint8) []common.Address {
	t.Helper()

	address := make([]common.Address, count)

	for i := uint8(0); i < count; i++ {
		address[i] = RandomAddress(t)
	}

	return address
}

type CreateTesseractParams struct {
	Address        common.Address
	Height         uint64
	Ixns           common.Interactions
	Receipts       common.Receipts
	Sealer         kramaid.KramaID
	Seal           []byte
	ClusterID      string
	HeaderCallback func(header *common.TesseractHeader)
	BodyCallback   func(body *common.TesseractBody)
}

// CreateTesseract creates a tesseract using tessseract params fields
// if any field thats not available in params need to be initialized using TesseractCallback field
func CreateTesseract(t *testing.T, params *CreateTesseractParams) *common.Tesseract {
	t.Helper()

	if params == nil {
		params = &CreateTesseractParams{}
	}

	if params.Address.IsNil() {
		params.Address = RandomAddress(t)
	}

	var interactionHash common.Hash

	header := &common.TesseractHeader{
		Address:   params.Address,
		Height:    params.Height,
		FuelUsed:  100,
		FuelLimit: 100,
		ClusterID: params.ClusterID,
	}

	if params.Ixns != nil {
		hash, err := params.Ixns.Hash()
		require.NoError(t, err)

		interactionHash = hash
	}

	body := &common.TesseractBody{
		InteractionHash: interactionHash,
	}

	if params.HeaderCallback != nil {
		params.HeaderCallback(header)
	}

	if params.BodyCallback != nil {
		params.BodyCallback(body)
	}

	return common.NewTesseract(*header, *body, params.Ixns, params.Receipts, params.Seal, params.Sealer)
}

func CreateTesseracts(t *testing.T, count int, paramsMap map[int]*CreateTesseractParams) []*common.Tesseract {
	t.Helper()

	tesseracts := make([]*common.Tesseract, count)

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

func GetTesseractHash(t *testing.T, ts *common.Tesseract) common.Hash {
	t.Helper()

	return ts.Hash()
}

func GetAddresses(t *testing.T, count int) []common.Address {
	t.Helper()

	addresses := make([]common.Address, count)
	for i := 0; i < count; i++ {
		addresses[i] = RandomAddress(t)
	}

	return addresses
}

func GetHashes(t *testing.T, count int) []common.Hash {
	t.Helper()

	hashes := make([]common.Hash, count)
	for i := 0; i < count; i++ {
		hashes[i] = RandomHash(t)
	}

	return hashes
}

type CreateIxParams struct {
	IxDataCallback func(ix *common.IxData)
	Sign           []byte
}

func CreateIX(t *testing.T, params *CreateIxParams) *common.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &common.IxData{
		Input: common.IxInput{
			Type: common.IxValueTransfer,
			TransferValues: map[common.AssetID]*big.Int{
				common.AssetID("add"): big.NewInt(1),
			},
		},
	}

	if params.IxDataCallback != nil {
		params.IxDataCallback(data)
	}

	if len(params.Sign) == 0 {
		params.Sign = []byte{}
	}

	ix, err := common.NewInteraction(*data, params.Sign)
	require.NoError(t, err)

	return ix
}

func CreateIxns(t *testing.T, count int, paramsMap map[int]*CreateIxParams) common.Interactions {
	t.Helper()

	if paramsMap == nil {
		paramsMap = map[int]*CreateIxParams{}
	}

	ixns := make(common.Interactions, count)

	for i := 0; i < count; i++ {
		ixns[i] = CreateIX(t, paramsMap[i])
	}

	return ixns
}

func GetIxParamsWithAddress(from common.Address, to common.Address) *CreateIxParams {
	return &CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
		},
		Sign: nil,
	}
}

func GetIxParamsMapWithAddresses(
	from []common.Address,
	to []common.Address,
) map[int]*CreateIxParams {
	count := len(from)
	ixParams := make(map[int]*CreateIxParams, count)

	for i := 0; i < count; i++ {
		ixParams[i] = GetIxParamsWithAddress(from[i], to[i])
	}

	return ixParams
}

// HeaderCallbackWithGridHash returns callback which assigns extra field with new commit data having random grid hash
func HeaderCallbackWithGridHash(t *testing.T) func(header *common.TesseractHeader) {
	t.Helper()

	return func(header *common.TesseractHeader) {
		header.Extra = common.CommitData{
			GridID: &common.TesseractGridID{
				Hash:  RandomHash(t),
				Parts: &common.TesseractParts{},
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

func GetTestAccount(t *testing.T, callBack func(acc *common.Account)) (*common.Account, common.Hash) {
	t.Helper()

	acc := new(common.Account)
	if callBack != nil {
		callBack(acc)
	}

	accHash, err := acc.Hash()
	assert.NoError(t, err)

	return acc, accHash
}

func CreateTesseractPartsWithTestData(t *testing.T) *common.TesseractParts {
	t.Helper()

	parts := &common.TesseractParts{
		Total: 2,
		Grid:  make(map[common.Address]common.TesseractHeightAndHash),
	}

	for i := 0; i < 2; i++ {
		parts.Grid[RandomAddress(t)] = common.TesseractHeightAndHash{
			Height: 3,
			Hash:   RandomHash(t),
		}
	}

	return parts
}

func CreateCommitDataWithTestData(t *testing.T) common.CommitData {
	t.Helper()

	return common.CommitData{
		Round:           4,
		CommitSignature: []byte{1, 2, 3},
		VoteSet: &common.ArrayOfBits{
			Elements: []uint64{4, 4},
		},
		EvidenceHash: RandomHash(t),
		GridID: &common.TesseractGridID{
			Hash:  RandomHash(t),
			Parts: CreateTesseractPartsWithTestData(t),
		},
	}
}

func CreateHeaderWithTestData(t *testing.T) common.TesseractHeader {
	t.Helper()

	header := common.TesseractHeader{
		Address:     RandomAddress(t),
		PrevHash:    RandomHash(t),
		Height:      4444,
		FuelUsed:    5,
		FuelLimit:   6,
		BodyHash:    RandomHash(t),
		GroupHash:   RandomHash(t),
		Operator:    "operator",
		ClusterID:   "cluster-ID",
		Timestamp:   1,
		ContextLock: make(map[common.Address]common.ContextLockInfo),
		Extra:       CreateCommitDataWithTestData(t),
	}

	header.ContextLock[RandomAddress(t)] = common.ContextLockInfo{
		TesseractHash: RandomHash(t),
	}

	return header
}

func CheckForTesseract(t *testing.T, expectedTS, actualTS *common.Tesseract, withInteractions bool) {
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

	config := &crypto.VaultConfig{
		DataDir:      dataDir,
		NodePassword: password,
	}

	vault, err := crypto.NewVault(config, moinode.MoiFullNode, 1)
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
	engineio.RegisterRuntime(pisa.NewRuntime(), nil)

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
	engineio.RegisterRuntime(pisa.NewRuntime(), nil)

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
	ixType common.IxType,
	payload []byte,
	perceivedProofs []byte,
) common.IxInput {
	t.Helper()

	IxInput := common.IxInput{
		Type:  ixType,
		Nonce: 2,

		Sender:   RandomAddress(t),
		Receiver: RandomAddress(t),
		Payer:    RandomAddress(t),

		TransferValues: map[common.AssetID]*big.Int{
			"0180127603f47e5aff68052402fda5c4364e8e6cff1e107e4e821af00d0eee2edf16": big.NewInt(1033),
			"0180127603f47e5aff68052402fda5c4364e8e6cff1e107e4e821af00d0eee2edf15": big.NewInt(1093),
		},
		PerceivedValues: map[common.AssetID]*big.Int{
			"0180127603f47e5aff68053102fda5c4364e8e6cff1e107e4e821af00d0eee2edf16": big.NewInt(1233),
			"0180127603f47e5aff68053102fda5c4364e8e6cff1e107e4e821af00d0eee2ed416": big.NewInt(1333),
		},
		PerceivedProofs: perceivedProofs,

		FuelLimit: 1043,
		FuelPrice: new(big.Int).SetUint64(1),

		Payload: payload,
	}

	return IxInput
}

func CreateComputeWithTestData(t *testing.T, hash common.Hash, kramaIDs []kramaid.KramaID) common.IxCompute {
	t.Helper()

	IxCompute := common.IxCompute{
		Mode:         3,
		Hash:         hash,
		ComputeNodes: kramaIDs,
	}

	return IxCompute
}

func CreateTrustWithTestData(t *testing.T) common.IxTrust {
	t.Helper()

	IxTrust := common.IxTrust{
		MTQ:        8,
		TrustNodes: GetTestKramaIDs(t, 2),
	}

	return IxTrust
}

func CreateReceiptWithTestData(t *testing.T) *common.Receipt {
	t.Helper()

	// create dummy logs
	logs := &common.Log{
		Addresses: GetAddresses(t, 1),
		LogicID:   GetLogicID(t, RandomAddress(t)),
		Topics:    GetHashes(t, 1),
		Data:      []byte{1},
	}

	receipt := &common.Receipt{
		IxType:    2,
		IxHash:    RandomHash(t),
		Logs:      []*common.Log{logs},
		Status:    common.ReceiptFailed,
		FuelUsed:  99,
		Hashes:    make(common.ReceiptAccHashes),
		ExtraData: []byte{1, 2},
	}

	for i := 0; i < 3; i++ {
		receipt.Hashes[RandomAddress(t)] = &common.Hashes{
			StateHash:   RandomHash(t),
			ContextHash: RandomHash(t),
		}
	}

	return receipt
}

func CreateBodyWithTestData(t *testing.T) common.TesseractBody {
	t.Helper()

	body := common.TesseractBody{
		StateHash:       RandomHash(t),
		ContextHash:     RandomHash(t),
		InteractionHash: RandomHash(t),
		ReceiptHash:     RandomHash(t),
		ContextDelta:    make(map[common.Address]*common.DeltaGroup),
		ConsensusProof: common.PoXtData{
			BinaryHash:   RandomHash(t),
			IdentityHash: RandomHash(t),
			ICSHash:      RandomHash(t),
		},
	}

	body.ContextDelta[RandomAddress(t)] = &common.DeltaGroup{
		BehaviouralNodes: GetTestKramaIDs(t, 2),
	}

	return body
}

func WriteToAccountsFile(filePath string, accounts []AccountWithMnemonic) error {
	file, err := json.MarshalIndent(accounts, "", "\t")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filePath, file, os.ModePerm); err != nil {
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

func GetIXSignature(t *testing.T, ixArgs *common.SendIXArgs, mnemonic string) []byte {
	t.Helper()

	rawIX, err := ixArgs.Bytes()
	require.NoError(t, err)

	sign, err := crypto.GetSignature(rawIX, mnemonic)
	require.NoError(t, err)

	rawSign, err := hex.DecodeString(sign)
	require.NoError(t, err)

	return rawSign
}

func GetLogicID(t *testing.T, address common.Address) common.LogicID {
	t.Helper()

	return common.NewLogicIDv0(true, false, false, false, 0, address)
}

func GetLogicIDs(t *testing.T, count int) []common.LogicID {
	t.Helper()

	logicIDs := make([]common.LogicID, count)

	for i := 0; i < count; i++ {
		logicIDs[i] = GetLogicID(t, RandomAddress(t))
	}

	return logicIDs
}

// GetKramaIDAndNetworkKey returns kramaID and network key pair
func GetKramaIDAndNetworkKey(t *testing.T, nthValidator uint32) (kramaid.KramaID, []byte) {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:]) // fill sign key with random bytes
	require.NoError(t, err)

	// get private key and public key
	privKeyBytes, moiPubBytes, err := GetPrivKeysForTest(signKey[:])
	require.NoError(t, err)

	networkKey := privKeyBytes[32:]

	kramaID, err := kramaid.NewKramaID( // Create kramaID from private key , public key
		networkKey,
		nthValidator,
		hex.EncodeToString(moiPubBytes),
		1,
		true,
	)
	require.NoError(t, err)

	return kramaID, networkKey
}

func GetRandomNumber(t *testing.T, max int) int {
	t.Helper()

	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	require.NoError(t, err)

	return int(nBig.Int64())
}

// WaitForResponse waits for response on respChannel
// and checks if datatype of data received on channel is equal to datatype of data received as argument
func WaitForResponse(t *testing.T, respChan chan Result, data interface{}) interface{} {
	t.Helper()

	res := <-respChan
	require.NoError(t, res.Err)

	require.Equal(t, reflect.TypeOf(data), reflect.TypeOf(res.Data))

	return res.Data
}

type Result struct {
	Data interface{}
	Err  error
}
