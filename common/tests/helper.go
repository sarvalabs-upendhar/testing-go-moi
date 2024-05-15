package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/VictoriaMetrics/fastcache"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-legacy-kramaid"
	"github.com/sarvalabs/go-moi-identifiers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/crypto"
	cryptocommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
)

const InvalidAccount common.AccountType = 9999

type HashInterface interface {
	Hash() (common.Hash, error)
}

// GetHash is used to return the hash of any type which implements HashInterface
func GetHash[T HashInterface](t *testing.T, in T) common.Hash {
	t.Helper()

	hash, err := in.Hash()
	assert.NoError(t, err)

	return hash
}

func RandomAddress(t *testing.T) identifiers.Address {
	t.Helper()

	address := make([]byte, 32)

	if _, err := rand.Read(address); err != nil {
		t.Fatal(err)
	}

	return identifiers.NewAddressFromBytes(address)
}

func RandomAddressWithMnemonic(t *testing.T) (identifiers.Address, string) {
	t.Helper()

	mnemonic := poi.GenerateRandMnemonic().String()

	_, publicKey, err := poi.GetPrivateKeyAtPath(mnemonic, config.DefaultMoiWalletPath)
	if err != nil {
		require.NoError(t, err)
	}

	return identifiers.NewAddressFromBytes(publicKey), mnemonic
}

func RandomHash(t *testing.T) common.Hash {
	t.Helper()

	hash := make([]byte, 32)

	if _, err := rand.Read(hash); err != nil {
		t.Fatal(err)
	}

	return common.BytesToHash(hash)
}

func RandomKramaID(t *testing.T, nthValidator uint32) kramaid.KramaID {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:])
	require.NoError(t, err)

	privateKeys, moiPubBytes, err := GetPrivKeysForTest(t, signKey[:])
	require.NoError(t, err)

	kramaID, err := kramaid.NewKramaID(
		1,
		privateKeys[32:],
		nthValidator,
		hex.EncodeToString(moiPubBytes),
		true,
	)
	require.NoError(t, err)

	return kramaID
}

func RandomKramaIDs(t *testing.T, count int) []kramaid.KramaID {
	t.Helper()

	ids := make([]kramaid.KramaID, 0, count)

	for i := 0; i < count; i++ {
		ids = append(ids, RandomKramaID(t, uint32(i)))
	}

	return ids
}

func RandomPeerID(t *testing.T) peer.ID {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:])
	require.NoError(t, err)

	privateKeys, _, err := GetPrivKeysForTest(t, signKey[:])
	require.NoError(t, err)

	peerID, err := kramaid.GeneratePeerID(privateKeys[32:])
	require.NoError(t, err)

	return peerID
}

func RandomPeerIDs(t *testing.T, count int) []peer.ID {
	t.Helper()

	peerIDs := make([]peer.ID, 0)

	for i := 0; i < count; i++ {
		peerIDs = append(peerIDs, RandomPeerID(t))
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

func GetPrivKeysForTest(t *testing.T, seed []byte) ([]byte, []byte, error) {
	t.Helper()
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

func GetRandomStrings(t *testing.T, count int) []string {
	t.Helper()

	randomStrings := make([]string, 0, count)
	for i := 0; i < count; i++ {
		randomStrings = append(randomStrings, GetRandomUpperCaseString(t, 5))
	}

	return randomStrings
}

func GetRandomAssetInfo(t *testing.T, addr identifiers.Address) *common.AssetDescriptor {
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
		LogicID:    identifiers.LogicID(RandomHash(t).String()),
	}

	return asset
}

func CreateTestAsset(t *testing.T, address identifiers.Address) (identifiers.AssetID, *common.AssetDescriptor) {
	t.Helper()

	asset := GetRandomAssetInfo(t, address)
	assetID := identifiers.NewAssetIDv0(
		asset.IsLogical,
		asset.IsStateFul,
		asset.Dimension,
		uint16(asset.Standard),
		address,
	)

	return assetID, asset
}

func CreateTestAssets(t *testing.T, count int) ([]identifiers.AssetID, []*common.AssetDescriptor) {
	t.Helper()

	assetIDs := make([]identifiers.AssetID, 0, count)
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

func GetRandomAssetID(t *testing.T, address identifiers.Address) identifiers.AssetID {
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

func RandomAccMetaInfo(t *testing.T, height uint64) *common.AccountMetaInfo {
	t.Helper()

	return &common.AccountMetaInfo{
		Address:       RandomAddress(t),
		Type:          common.AccountType(1),
		Height:        height,
		TesseractHash: RandomHash(t),
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

	return RandomKramaIDs(t, count), GetTestPublicKeys(t, count)
}

func GetRandomAddressList(t *testing.T, count int) []identifiers.Address {
	t.Helper()

	address := make([]identifiers.Address, count)

	for i := 0; i < count; i++ {
		address[i] = RandomAddress(t)
	}

	return address
}

type CreateTesseractParams struct {
	Addresses      []identifiers.Address
	Heights        []uint64
	Participants   common.ParticipantStates
	TSDataCallback func(ts *TesseractData)

	Ixns     common.Interactions
	Receipts common.Receipts
}

type TesseractData struct {
	InteractionsHash common.Hash
	ReceiptsHash     common.Hash
	Epoch            *big.Int
	Timestamp        uint64
	Operator         string
	FuelUsed         uint64
	FuelLimit        uint64
	ConsensusInfo    common.PoXtData

	// non canonical fields
	Seal   []byte
	SealBy kramaid.KramaID
}

func DefaultTesseractData() *TesseractData {
	return &TesseractData{
		InteractionsHash: common.NilHash,
		ReceiptsHash:     common.NilHash,
		Epoch:            big.NewInt(0),
		Timestamp:        0,
		Operator:         "",
		FuelUsed:         100,
		FuelLimit:        100,
		ConsensusInfo:    common.PoXtData{},

		// non canonical fields
		Seal:   nil,
		SealBy: "",
	}
}

// CreateTesseract is a helper function to create test with the provided params
func CreateTesseract(t *testing.T, params *CreateTesseractParams) *common.Tesseract {
	t.Helper()

	var (
		interactionsHash common.Hash
		tsData           = DefaultTesseractData()
	)

	if params == nil {
		params = &CreateTesseractParams{}
	}

	if params.Participants == nil {
		params.Participants = make(common.ParticipantStates)
	}

	// A tesseract should have at least one participant
	if len(params.Addresses) == 0 {
		addr := RandomAddress(t)
		params.Addresses = []identifiers.Address{addr}
	}

	// if participants are not provided then create them based on addresses provided with an empty state
	if len(params.Participants) == 0 {
		for i, addr := range params.Addresses {
			params.Participants[addr] = common.State{}

			if len(params.Heights) != 0 {
				params.Participants[addr] = common.State{
					Height: params.Heights[i],
				}
			}
		}
	}

	if params.Ixns != nil {
		hash, err := params.Ixns.Hash()
		require.NoError(t, err)

		interactionsHash = hash
	}

	if params.TSDataCallback != nil {
		params.TSDataCallback(tsData)
	}

	return common.NewTesseract(
		params.Participants,
		interactionsHash,
		tsData.ReceiptsHash,
		tsData.Epoch,
		tsData.Timestamp,
		tsData.Operator,
		tsData.FuelUsed,
		tsData.FuelLimit,
		tsData.ConsensusInfo,
		tsData.Seal,
		tsData.SealBy,
		params.Ixns,
		params.Receipts,
	)
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
				Heights: []uint64{uint64(i)},
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

func GetAddresses(t *testing.T, count int) []identifiers.Address {
	t.Helper()

	addresses := make([]identifiers.Address, count)
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

	addr, err := identifiers.NewAddressFromHex(
		"0xff919c3bd4523d638b1878a59c62e1c9a0a628127317d63359da30e18ee67593")
	require.NoError(t, err)

	assetID := identifiers.NewAssetIDv0(false, false, 0, 0, addr)

	data := &common.IxData{
		Input: common.IxInput{
			Type: common.IxValueTransfer,
			TransferValues: map[identifiers.AssetID]*big.Int{
				assetID: big.NewInt(1),
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

func GetIxParamsWithAddress(from identifiers.Address, to identifiers.Address) *CreateIxParams {
	return &CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Input.Sender = from
			ix.Input.Receiver = to
		},
		Sign: nil,
	}
}

func GetIxParamsMapWithAddresses(
	from []identifiers.Address,
	to []identifiers.Address,
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
			Heights: []uint64{uint64(i)},
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

func CheckForTesseract(t *testing.T, expectedTS, actualTS *common.Tesseract, withInteractions bool) {
	t.Helper()

	if withInteractions {
		require.Greater(t, len(expectedTS.Interactions()), 0)
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
	sigBytes, err = vault.Sign(msg, cryptocommon.BlsBLST)
	require.NoError(t, err)

	// remove keystore.json in current directory
	err = os.Remove("./keystore.json")
	require.NoError(t, err)

	return sigBytes, pk
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

		TransferValues: map[identifiers.AssetID]*big.Int{
			"0180127603f47e5aff68052402fda5c4364e8e6cff1e107e4e821af00d0eee2edf16": big.NewInt(1033),
			"0180127603f47e5aff68052402fda5c4364e8e6cff1e107e4e821af00d0eee2edf15": big.NewInt(1093),
		},
		PerceivedValues: map[identifiers.AssetID]*big.Int{
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
		TrustNodes: RandomKramaIDs(t, 2),
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
		Status:    common.ReceiptStateReverted,
		FuelUsed:  99,
		ExtraData: []byte{1, 2},
	}

	return receipt
}

func CreateStateWithTestData(t *testing.T) common.State {
	t.Helper()

	s := common.State{
		Height:          6,
		TransitiveLink:  RandomHash(t),
		PreviousContext: RandomHash(t),
		LatestContext:   RandomHash(t),
		StateHash:       RandomHash(t),
		ContextDelta: &common.DeltaGroup{
			BehaviouralNodes: RandomKramaIDs(t, 2),
			RandomNodes:      RandomKramaIDs(t, 2),
			ReplacedNodes:    RandomKramaIDs(t, 2),
		},
	}

	return s
}

func CreatePoXtWithTestData(t *testing.T) common.PoXtData {
	t.Helper()

	return common.PoXtData{
		EvidenceHash: RandomHash(t),
		BinaryHash:   RandomHash(t),
		IdentityHash: RandomHash(t),
		ICSHash:      RandomHash(t),
		ClusterID:    "cluster",
		ICSSignature: []byte{1, 2, 3},
		ICSVoteset: &common.ArrayOfBits{
			Elements: []uint64{4, 6},
		},
		Round:           5,
		CommitSignature: []byte{1, 2, 3, 5},
		BFTVoteSet: &common.ArrayOfBits{
			Elements: []uint64{4, 8},
		},
	}
}

func CreateParticipantWithTestData(t *testing.T, count int) common.ParticipantStates {
	t.Helper()

	p := make(common.ParticipantStates)

	for i := 0; i < count; i++ {
		p[RandomAddress(t)] = CreateStateWithTestData(t)
	}

	return p
}

func CreateReceiptsWithTestData(t *testing.T, hash common.Hash) common.Receipts {
	t.Helper()

	receipts := make(common.Receipts)
	receipts[hash] = CreateReceiptWithTestData(t)

	return receipts
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

func GetLogicID(t *testing.T, address identifiers.Address) identifiers.LogicID {
	t.Helper()

	return identifiers.NewLogicIDv0(true, false, false, false, 0, address)
}

func GetLogicIDs(t *testing.T, count int) []identifiers.LogicID {
	t.Helper()

	logicIDs := make([]identifiers.LogicID, count)

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
	privKeyBytes, moiPubBytes, err := GetPrivKeysForTest(t, signKey[:])
	require.NoError(t, err)

	networkKey := privKeyBytes[32:]

	kramaID, err := kramaid.NewKramaID( // Create kramaID from private key , public key
		1,
		networkKey,
		nthValidator,
		hex.EncodeToString(moiPubBytes),
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

func NewTestTreeCache() *fastcache.Cache {
	return fastcache.New(200)
}

// CreateTestIxParticipants creates a list of address and a map of IxParticipants with default values
func CreateTestIxParticipants(t *testing.T, count int, genesisAccCount int) (
	[]identifiers.Address,
	map[identifiers.Address]common.IxParticipant,
) {
	t.Helper()

	addrs := make([]identifiers.Address, count)

	ps := make(map[identifiers.Address]common.IxParticipant, 0)

	for i := 0; i < count; i++ {
		addrs[i] = RandomAddress(t)
		ps[addrs[i]] = common.IxParticipant{
			AccType:   common.RegularAccount,
			IsSigner:  true,
			LockType:  common.WriteLock,
			IsGenesis: false,
		}
	}

	for i := 0; i < genesisAccCount; i++ {
		addrs[i] = RandomAddress(t)
		ps[addrs[i]] = common.IxParticipant{
			AccType:   common.RegularAccount,
			IsSigner:  true,
			LockType:  common.WriteLock,
			IsGenesis: true,
		}
	}

	return addrs, ps
}
