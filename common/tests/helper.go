package tests

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/VictoriaMetrics/fastcache"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/crypto"
	cryptocommon "github.com/sarvalabs/go-moi/crypto/common"
	"github.com/sarvalabs/go-moi/crypto/poi"
	"github.com/sarvalabs/go-moi/crypto/poi/moinode"
)

const InvalidAccount common.AccountType = 9999

type HashInterface interface {
	Hash() (common.Hash, error)
}

// DefaultTestBeneficiaryID is the default beneficiary ID used in tests.
var DefaultTestBeneficiaryID identifiers.Identifier = identifiers.RandomParticipantIDv0().AsIdentifier()

// GetHash is used to return the hash of any type which implements HashInterface
func GetHash[T HashInterface](t *testing.T, in T) common.Hash {
	t.Helper()

	hash, err := in.Hash()
	assert.NoError(t, err)

	return hash
}

func RandomIdentifier(t *testing.T) identifiers.Identifier {
	t.Helper()

	return identifiers.RandomParticipantIDv0().AsIdentifier()
}

func RandomSubAccountIdentifier(t *testing.T, i uint32) identifiers.Identifier {
	t.Helper()

	id, _ := identifiers.GenerateParticipantIDv0(identifiers.RandomFingerprint(), i)

	return id.AsIdentifier()
}

func RandomIdentifierWithZeroVariant(t *testing.T) identifiers.Identifier {
	t.Helper()

	id, _ := identifiers.GenerateParticipantIDv0(identifiers.RandomFingerprint(), 0)

	return id.AsIdentifier()
}

func RandomIDWithMnemonic(t *testing.T) (identifiers.Identifier, string) {
	t.Helper()

	return identifiers.RandomParticipantIDv0().AsIdentifier(), poi.GenerateRandMnemonic().String()
}

func RandomValidatorInfo(t *testing.T, idx common.ValidatorIndex, pubkey []byte) *common.ValidatorInfo {
	t.Helper()

	return &common.ValidatorInfo{
		ID:        idx,
		KramaID:   RandomKramaID(t, 0),
		PublicKey: pubkey,
	}
}

func RandomValidatorsInfo(t *testing.T, pubkeys [][]byte) common.NodeList {
	t.Helper()

	nodelist := make(common.NodeList, len(pubkeys))

	for i, pubkey := range pubkeys {
		nodelist[i] = RandomValidatorInfo(t, common.ValidatorIndex(i), pubkey)
	}

	return nodelist
}

func RandomHash(t *testing.T) common.Hash {
	t.Helper()

	hash := make([]byte, 32)

	if _, err := rand.Read(hash); err != nil {
		t.Fatal(err)
	}

	return common.BytesToHash(hash)
}

func RandomKramaID(t *testing.T, nthValidator uint32) identifiers.KramaID {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:])
	require.NoError(t, err)

	privateKeys, err := identifiers.GetRandomPrivateKeys(signKey)
	require.NoError(t, err)

	kramaID, err := identifiers.GenerateKramaIDv0(
		identifiers.NetworkZone0,
		privateKeys[32:],
	)
	require.NoError(t, err)

	return kramaID
}

func RandomKramaIDs(t *testing.T, count int) []identifiers.KramaID {
	t.Helper()

	ids := make([]identifiers.KramaID, 0, count)

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

	privateKeys, err := identifiers.GetRandomPrivateKeys(signKey)
	require.NoError(t, err)

	peerID, err := identifiers.GeneratePeerID(privateKeys[32:])
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

func DecodePeerIDFromKramaID(t *testing.T, kramaID identifiers.KramaID) peer.ID {
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
				resCh <- result{nil, common.ErrTimeOut}

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

func GetRandomAssetInfo(t *testing.T, id identifiers.Identifier) *common.AssetDescriptor {
	t.Helper()

	symbol := GetRandomUpperCaseString(t, 5)

	if id.IsNil() {
		id = RandomIdentifier(t)
	}

	asset := &common.AssetDescriptor{
		Creator:   id,
		Manager:   id,
		Decimals:  4,
		MaxSupply: big.NewInt(1000),
		Symbol:    symbol,
		LogicID:   identifiers.RandomLogicIDv0(),
	}

	assetID, err := identifiers.GenerateAssetIDv0(
		id.Fingerprint(),
		id.Variant(),
		uint16(0),
		asset.Flags()...,
	)
	require.NoError(t, err)

	asset.AssetID = assetID

	return asset
}

func CreateTestAsset(t *testing.T, id identifiers.Identifier) (identifiers.AssetID, *common.AssetDescriptor) {
	t.Helper()

	asset := GetRandomAssetInfo(t, id)

	return asset.AssetID, asset
}

func CreateTestAssets(t *testing.T, count int) ([]identifiers.AssetID, []*common.AssetDescriptor) {
	t.Helper()

	assetIDs := make([]identifiers.AssetID, 0, count)
	assetDescriptors := make([]*common.AssetDescriptor, 0, count)

	for i := 0; i < count; i++ {
		assetID, assetDescriptor := CreateTestAsset(t, RandomIdentifier(t))

		assetIDs = append(assetIDs, assetID)
		assetDescriptors = append(assetDescriptors, assetDescriptor)
	}

	return assetIDs, assetDescriptors
}

func GetRandomNumbers(t *testing.T, ceil int, count int) []*big.Int {
	t.Helper()

	var err error

	numbers := make([]*big.Int, count)

	for i := 0; i < count; i++ {
		numbers[i], err = rand.Int(rand.Reader, big.NewInt(int64(ceil)))
		require.NoError(t, err)
	}

	return numbers
}

func GetRandomTokenID(t *testing.T) common.TokenID {
	t.Helper()

	randUint, err := rand.Int(rand.Reader, big.NewInt(0).SetUint64(math.MaxUint64))
	require.NoError(t, err)

	return common.TokenID(randUint.Uint64())
}

func GetRandomAssetID(t *testing.T, id identifiers.Identifier) identifiers.AssetID {
	t.Helper()

	assetID, _ := CreateTestAsset(t, id)

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

func GetRandomAccMetaInfo(t *testing.T, height uint64) *common.AccountMetaInfo {
	t.Helper()

	return &common.AccountMetaInfo{
		ID:                   RandomIdentifierWithZeroVariant(t),
		Type:                 common.AccountType(1),
		InheritedAccount:     RandomIdentifier(t),
		Height:               height,
		TesseractHash:        RandomHash(t),
		StateHash:            RandomHash(t),
		ContextHash:          RandomHash(t),
		CommitHash:           RandomHash(t),
		PositionInContextSet: 3,
	}
}

func GetTestPublicKey(t *testing.T) []byte {
	t.Helper()

	return RandomIdentifier(t).Bytes()
}

func GetTestPublicKeys(t *testing.T, count int) [][]byte {
	t.Helper()

	p := make([][]byte, 0)

	for i := 0; i < count; i++ {
		pubKeys := GetTestPublicKey(t)
		p = append(p, pubKeys)
	}

	return p
}

func GetTestKramaIdsWithPublicKeys(t *testing.T, count int) ([]identifiers.KramaID, [][]byte) {
	t.Helper()

	return RandomKramaIDs(t, count), GetTestPublicKeys(t, count)
}

func GetRandomIDs(t *testing.T, count int) []identifiers.Identifier {
	t.Helper()

	id := make([]identifiers.Identifier, count)

	for i := 0; i < count; i++ {
		id[i] = RandomIdentifier(t)
	}

	return id
}

type CreateTesseractParams struct {
	IDs                  []identifiers.Identifier
	Heights              []uint64
	Participants         common.ParticipantsState
	TSDataCallback       func(ts *TesseractData)
	ParticipantsCallback func(participants common.ParticipantsState)

	Ixns       common.Interactions
	Receipts   common.Receipts
	CommitInfo *common.CommitInfo
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
	CommitInfo       *common.CommitInfo

	// non canonical fields
	Seal   []byte
	SealBy identifiers.KramaID
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
		ConsensusInfo: common.PoXtData{
			View: 1,
		},

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
		params.Participants = make(common.ParticipantsState)
	}

	// A tesseract should have at least one participant
	if len(params.IDs) == 0 {
		id := RandomIdentifier(t)
		params.IDs = []identifiers.Identifier{id}
	}

	// if participants are not provided then create them based on ids provided with an empty state
	if len(params.Participants) == 0 {
		for i, id := range params.IDs {
			params.Participants[id] = common.State{}

			if len(params.Heights) != 0 {
				params.Participants[id] = common.State{
					Height: params.Heights[i],
				}
			}
		}
	}

	if len(params.Ixns.IxList()) != 0 {
		hash, err := params.Ixns.Hash()
		require.NoError(t, err)

		interactionsHash = hash
	}

	if params.TSDataCallback != nil {
		params.TSDataCallback(tsData)
	}

	if params.ParticipantsCallback != nil {
		params.ParticipantsCallback(params.Participants)
	}

	return common.NewTesseract(
		params.Participants,
		interactionsHash,
		tsData.ReceiptsHash,
		tsData.Epoch,
		tsData.Timestamp,
		tsData.FuelUsed,
		tsData.FuelLimit,
		tsData.ConsensusInfo,
		tsData.Seal,
		tsData.SealBy,
		params.Ixns,
		params.Receipts,
		params.CommitInfo,
	)
}

func GetArrayOfBits(num int) *common.ArrayOfBits {
	if num%64 == 0 {
		num /= 64
	} else {
		num = (num / 64) + 1
	}

	a := &common.ArrayOfBits{
		Size:     8,
		Elements: make([]uint64, num),
	}

	for i := 0; i < num; i++ {
		a.Elements[i] = math.MaxUint64
	}

	return a
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

func GetIdentifiers(t *testing.T, count int) []identifiers.Identifier {
	t.Helper()

	ids := make([]identifiers.Identifier, count)
	for i := 0; i < count; i++ {
		ids[i] = RandomIdentifierWithZeroVariant(t)
	}

	return ids
}

func GetSubAccountIdentifiers(t *testing.T, count int) []identifiers.Identifier {
	t.Helper()

	ids := make([]identifiers.Identifier, count)
	for i := 0; i < count; i++ {
		ids[i] = RandomSubAccountIdentifier(t, uint32(i+1))
	}

	return ids
}

func GetHashes(t *testing.T, count int) []common.Hash {
	t.Helper()

	hashes := make([]common.Hash, count)
	for i := 0; i < count; i++ {
		hashes[i] = RandomHash(t)
	}

	return hashes
}

func XORBytes(t *testing.T, arrays ...[32]byte) [32]byte {
	t.Helper()

	var result [32]byte
	if len(arrays) == 0 {
		return result
	}

	result = arrays[0]

	for _, array := range arrays[1:] {
		for i := 0; i < 32; i++ {
			result[i] ^= array[i]
		}
	}

	return result
}

func AssetActionPayload(
	t *testing.T, assetID identifiers.AssetID, callsite string, params any,
) *common.AssetActionPayload {
	t.Helper()

	action, err := common.GetAssetActionPayload(assetID, callsite, params)
	require.NoError(t, err)

	return action
}

type CreateIxParams struct {
	IxDataCallback     func(ix *common.IxData)
	SenderSign         []byte
	SignaturesCallback func(ixData *common.IxData, sig *common.Signatures)
}

func IsPresent(participants []common.IxParticipant, id identifiers.Identifier) bool {
	for _, p := range participants {
		if p.ID == id {
			return true
		}
	}

	return false
}

func AddParticipants(t *testing.T, ixData *common.IxData, participants ...common.IxParticipant) {
	t.Helper()

	for _, p := range participants {
		found := false

		for index, oldParticipant := range ixData.Participants {
			if oldParticipant.ID == p.ID {
				found = true
				ixData.Participants[index] = p
			}
		}

		if !found {
			ixData.Participants = append(ixData.Participants, p)
		}
	}
}

func AddIxOp(t *testing.T, ixData *common.IxData, ixOpType common.IxOpType, assetID identifiers.AssetID, payload any) {
	t.Helper()

	switch ixOpType {
	case common.IxInvalid:
		return
	case common.IxAccountInherit:
		ai, ok := payload.(*common.AccountInheritPayload)
		require.True(t, ok, "failed to type cast payload")

		ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
			Type: ixOpType,
			Payload: func() []byte {
				b, err := ai.Bytes()
				require.NoError(t, err)

				return b
			}(),
		})

		if ai.Value != nil && !IsPresent(ixData.Participants, ai.Value.AssetID.AsIdentifier()) {
			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				ID:       ai.Value.AssetID.AsIdentifier(),
				LockType: common.NoLock,
			})
		}

	case common.IxAccountConfigure:
		acp, ok := payload.(*common.AccountConfigurePayload)
		require.True(t, ok, "failed to type cast payload")

		ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
			Type: ixOpType,
			Payload: func() []byte {
				b, err := acp.Bytes()
				require.NoError(t, err)

				return b
			}(),
		})

	case common.IxAssetCreate:
		acp, ok := payload.(*common.AssetCreatePayload)
		require.True(t, ok, "failed to type cast payload")

		ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
			Type: ixOpType,
			Payload: func() []byte {
				b, err := acp.Bytes()
				require.NoError(t, err)

				return b
			}(),
		})

	case common.IxParticipantCreate:
		cp, ok := payload.(*common.ParticipantCreatePayload)
		require.True(t, ok, "failed to type cast payload")

		if !IsPresent(ixData.Participants, cp.ID) {
			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				ID:       cp.ID,
				LockType: common.MutateLock,
			})
		}

		if cp.Value != nil && !IsPresent(ixData.Participants, cp.Value.AssetID.AsIdentifier()) {
			ixData.Participants = append(ixData.Participants, common.IxParticipant{
				ID:       cp.Value.AssetID.AsIdentifier(),
				LockType: common.NoLock,
			})
		}

		ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
			Type: ixOpType,
			Payload: func() []byte {
				b, err := cp.Bytes()
				require.NoError(t, err)

				return b
			}(),
		})

	case common.IxAssetAction:
		x := payload

		var err error

		if action, ok := payload.(*common.AssetActionPayload); ok {
			x, _, err = common.GetParamsFromActionPayload(action)

			require.NoError(t, err)

			assetID = action.AssetID
		}

		switch y := x.(type) {
		case *common.TransferParams:
			if !IsPresent(ixData.Participants, assetID.AsIdentifier()) {
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					ID:       assetID.AsIdentifier(),
					LockType: common.NoLock,
				})
			}

			tp := y

			ap, err := common.GetAssetActionPayload(assetID, common.TransferEndpoint, tp)
			require.NoError(t, err)

			if !tp.Beneficiary.IsNil() && !IsPresent(ixData.Participants, tp.Beneficiary) {
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					ID:       tp.Beneficiary,
					LockType: common.MutateLock,
				})
			}

			ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
				Type: ixOpType,
				Payload: func() []byte {
					b, err := ap.Bytes()
					require.NoError(t, err)

					return b
				}(),
			})

		case *common.MintParams:
			if !IsPresent(ixData.Participants, assetID.AsIdentifier()) {
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					ID:       assetID.AsIdentifier(),
					LockType: common.MutateLock,
				})
			}

			mp := y

			ap, err := common.GetAssetActionPayload(assetID, common.TransferEndpoint, mp)
			require.NoError(t, err)

			if !IsPresent(ixData.Participants, mp.Beneficiary) {
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					ID:       mp.Beneficiary,
					LockType: common.MutateLock,
				})
			}

			ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
				Type: ixOpType,
				Payload: func() []byte {
					b, err := ap.Bytes()
					require.NoError(t, err)

					return b
				}(),
			})
		default:
			require.NoError(t, common.ErrInvalidInteractionType)
		}

	case common.IxLogicDeploy, common.IxLogicInvoke:
		lp, ok := payload.(*common.LogicPayload)
		require.True(t, ok)

		if common.IxLogicDeploy != ixOpType {
			if !IsPresent(ixData.Participants, lp.LogicID.AsIdentifier()) {
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					ID:       lp.LogicID.AsIdentifier(),
					LockType: common.MutateLock,
				})
			}
		}

		for _, logic := range lp.Interfaces {
			if !IsPresent(ixData.Participants, logic) {
				ixData.Participants = append(ixData.Participants, common.IxParticipant{
					ID:       logic,
					LockType: common.MutateLock,
				})
			}
		}

		ixData.IxOps = append(ixData.IxOps, common.IxOpRaw{
			Type: ixOpType,
			Payload: func() []byte {
				b, err := lp.Bytes()
				require.NoError(t, err)

				return b
			}(),
		})

	default:
		require.NoError(t, common.ErrInvalidInteractionType)
	}
}

func AppendDefaultParticipants(t *testing.T, data *common.IxData) {
	t.Helper()

	if !data.Payer.IsNil() {
		if !IsPresent(data.Participants, data.Payer) {
			data.Participants = append(data.Participants, common.IxParticipant{
				ID:       data.Payer,
				LockType: common.MutateLock,
			})
		}
	}

	if !IsPresent(data.Participants, data.Sender.ID) {
		data.Participants = append(data.Participants, common.IxParticipant{
			ID:       data.Sender.ID,
			LockType: common.MutateLock,
		})
	}
}

func CreateIX(t *testing.T, params *CreateIxParams) *common.Interaction {
	t.Helper()

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &common.IxData{
		IxOps:        []common.IxOpRaw{},
		Participants: []common.IxParticipant{},
	}

	if params.IxDataCallback != nil {
		params.IxDataCallback(data)
	}

	if data.Sender.ID == identifiers.Nil {
		data.Sender.ID = RandomIdentifierWithZeroVariant(t)
	}

	AppendDefaultParticipants(t, data)

	if len(params.SenderSign) == 0 {
		params.SenderSign = []byte{}
	}

	signatures := common.Signatures{
		{
			ID:        data.Sender.ID,
			KeyID:     data.Sender.KeyID,
			Signature: params.SenderSign,
		},
	}

	if params.SignaturesCallback != nil {
		params.SignaturesCallback(data, &signatures)
	}

	ix, err := common.NewInteraction(*data, signatures)
	require.NoError(t, err)

	return ix
}

// CreateIXWithParticipants creates ixns with 0th participant as sender, 1 st participants as
// receiver and remaining participants as extra ones
func CreateIXWithParticipants(t *testing.T, ps []identifiers.Identifier,
	sequenceID uint64, params *CreateIxParams,
) *common.Interaction {
	t.Helper()

	if len(ps) < 2 {
		return nil
	}

	if params == nil {
		params = &CreateIxParams{}
	}

	data := &common.IxData{
		IxOps: []common.IxOpRaw{},

		Participants: []common.IxParticipant{},
	}

	AddIxOp(t, data, common.IxAssetAction, common.KMOITokenAssetID, &common.TransferParams{
		Beneficiary: ps[1],
		Amount:      big.NewInt(1),
	})

	if params.IxDataCallback != nil {
		params.IxDataCallback(data)
	}

	if data.Sender.ID == identifiers.Nil {
		data.Sender.ID = ps[0]
		data.Sender.SequenceID = sequenceID
		data.Participants = append(data.Participants, common.IxParticipant{
			ID:       data.Sender.ID,
			LockType: common.MutateLock,
		})
	}

	// for _, id := range ps {
	//	AddParticipants(t, data, common.IxParticipant{
	//		ID:       id,
	//		LockType: common.MutateLock,
	//	})
	//}

	for _, id := range ps[2:] {
		data.Participants = append(data.Participants, common.IxParticipant{
			ID:       id,
			LockType: common.MutateLock,
		})
	}

	if len(params.SenderSign) == 0 {
		params.SenderSign = []byte{}
	}

	signatures := common.Signatures{
		{
			ID:    data.Sender.ID,
			KeyID: data.Sender.KeyID,
		},
	}

	if params.SignaturesCallback != nil {
		params.SignaturesCallback(data, &signatures)
	}

	ix, err := common.NewInteraction(*data, signatures)
	require.NoError(t, err)

	return ix
}

func Min24Byte(t *testing.T, lastByte byte) [24]byte {
	t.Helper()

	var minValue [24]byte

	for i := range minValue {
		minValue[i] = 0x00
	}

	minValue[23] = lastByte

	return minValue
}

func Max24Byte(t *testing.T) [24]byte {
	t.Helper()

	var maxValue [24]byte

	for i := range maxValue {
		maxValue[i] = 0xFF
	}

	return maxValue
}

// TODO: Check this usage for manual ixOp updation
func CreateIxns(t *testing.T, count int, paramsMap map[int]*CreateIxParams) []*common.Interaction {
	t.Helper()

	if paramsMap == nil {
		paramsMap = map[int]*CreateIxParams{}
	}

	ixns := make([]*common.Interaction, count)

	for i := 0; i < count; i++ {
		ixns[i] = CreateIX(t, paramsMap[i])
	}

	return ixns
}

func GetIxParamsForTransfer(t *testing.T, from identifiers.Identifier, to identifiers.Identifier) *CreateIxParams {
	t.Helper()

	return &CreateIxParams{
		IxDataCallback: func(ix *common.IxData) {
			ix.Sender.ID = from
			AddIxOp(t, ix, common.IxAssetAction, common.KMOITokenAssetID, &common.TransferParams{
				Beneficiary: to,
				Amount:      big.NewInt(10),
			})
		},
		SenderSign: nil,
	}
}

func GetIxParamsMapWithIDs(
	t *testing.T,
	from []identifiers.Identifier,
	to []identifiers.Identifier,
) map[int]*CreateIxParams {
	t.Helper()

	count := len(from)
	ixParams := make(map[int]*CreateIxParams, count)

	for i := 0; i < count; i++ {
		ixParams[i] = GetIxParamsForTransfer(t, from[i], to[i])
	}

	return ixParams
}

// GetTesseractParamsMapWithIxnsAndReceipts returns tsCount (no.of tesseracts)
// and each one will have ixnCount interactions
func GetTesseractParamsMapWithIxnsAndReceipts(t *testing.T, tsCount, ixnCount int) map[int]*CreateTesseractParams {
	t.Helper()

	tesseractParams := make(map[int]*CreateTesseractParams, tsCount)
	ids := GetIdentifiers(t, 2*(tsCount-1)*ixnCount) // for each interaction, sender and receiver ids needed
	receipts := CreateReceiptsWithTestData(t, RandomHash(t))
	ixns := CreateIxns(
		t,
		(tsCount-1)*ixnCount,
		GetIxParamsMapWithIDs(t, ids[:2*(tsCount-1)], ids[2*(tsCount-1):]),
	)

	// allocate interactions to each tesseract, excluding the first tesseract (which is the genesis tesseract)
	for i := 0; i < tsCount; i++ {
		tesseractParams[i] = &CreateTesseractParams{
			Heights: []uint64{uint64(i)},
		}

		if i > 0 {
			// allocate two interactions per tesseract
			tesseractParams[i].Ixns = common.NewInteractionsWithLeaderCheck(false,
				ixns[(i-1)*ixnCount:(i-1)*ixnCount+ixnCount]...)
		}

		tesseractParams[i].Receipts = receipts
		tesseractParams[i].CommitInfo = &common.CommitInfo{
			Operator: RandomKramaID(t, 0),
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

func ValidateTesseract(t *testing.T, expectedTS *common.Tesseract, ts *common.Tesseract,
	withInteractions bool, withCommitInfo bool,
) {
	t.Helper()

	require.Equal(t, expectedTS.Participants(), ts.Participants())
	require.Equal(t, expectedTS.Epoch(), ts.Epoch())
	require.Equal(t, expectedTS.Timestamp(), ts.Timestamp())
	require.Equal(t, expectedTS.FuelUsed(), ts.FuelUsed())
	require.Equal(t, expectedTS.FuelLimit(), ts.FuelLimit())
	require.Equal(t, expectedTS.ConsensusInfo(), ts.ConsensusInfo())
	require.Equal(t, expectedTS.Seal(), ts.Seal())
	require.Equal(t, expectedTS.SealBy(), ts.SealBy())

	if !withInteractions { // check if tesseracts matches
		require.Equal(t, 0, ts.Interactions().Len()) // make sure returned tesseract has zero ixns
		require.Equal(t, 0, len(ts.Receipts()))
	} else {
		require.Equal(t, expectedTS.Interactions().IxList(), ts.Interactions().IxList())
		require.Equal(t, expectedTS.Receipts(), ts.Receipts())
	}

	if !withCommitInfo {
		require.Nil(t, ts.CommitInfo())
	} else {
		require.Equal(t, expectedTS.CommitInfo(), ts.CommitInfo())
	}
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

func CreateIXDataWithTestData(t *testing.T, callback func(ixData *common.IxData)) common.IxData {
	t.Helper()

	ixData := &common.IxData{
		Sender: common.Sender{
			ID:         RandomIdentifier(t),
			SequenceID: 2,
		},
		Payer: RandomIdentifier(t),

		FuelLimit: 1043,
		FuelPrice: new(big.Int).SetUint64(1),

		Funds: []common.IxFund{
			{
				AssetID: GetRandomAssetID(t, RandomIdentifier(t)),
				Amount:  big.NewInt(10),
			},
		},

		IxOps: []common.IxOpRaw{
			{
				Type:    common.IxAssetCreate,
				Payload: CreateRawAssetCreatePayload(t),
			},
		},

		Participants: []common.IxParticipant{
			{
				ID:       RandomIdentifier(t),
				LockType: common.MutateLock,
			},
		},

		Preferences: &common.IxPreferences{
			Compute: []byte{1, 2, 3},
			Consensus: &common.IxConsensusPreference{
				MTQ:        1,
				TrustNodes: RandomKramaIDs(t, 3),
			},
		},

		Perception: []byte{1, 2, 3},
	}

	ixData.Participants = append(ixData.Participants, []common.IxParticipant{
		{
			ID: ixData.Sender.ID,
		},
		{
			ID: ixData.Payer,
		},
	}...)

	if callback != nil {
		callback(ixData)
	}

	return *ixData
}

func CreateReceiptWithTestData(t *testing.T) *common.Receipt {
	t.Helper()

	// create dummy logs
	log := common.Log{
		ID:      RandomIdentifier(t),
		LogicID: GetLogicID(t, RandomIdentifier(t)),
		Topics:  GetHashes(t, 1),
		Data:    []byte{1},
	}

	receipt := &common.Receipt{
		IxHash:   RandomHash(t),
		Status:   common.ReceiptStateReverted,
		FuelUsed: 99,
		IxOps: []*common.IxOpResult{
			{
				IxType: 2,
				Status: common.ResultDefectRaised,
				Data:   []byte{1, 2},
				Logs:   []common.Log{log},
			},
		},
	}

	return receipt
}

func CreateStateWithTestData(t *testing.T) common.State {
	t.Helper()

	s := common.State{
		Height:         6,
		TransitiveLink: RandomHash(t),
		LockedContext:  RandomHash(t),
		StateHash:      RandomHash(t),
		ContextDelta: &common.DeltaGroup{
			ConsensusNodes: RandomKramaIDs(t, 2),
			ReplacedNodes:  RandomKramaIDs(t, 2),
		},
	}

	return s
}

func CreatePoXtWithTestData(t *testing.T, view uint64) common.PoXtData {
	t.Helper()

	return common.PoXtData{
		// TODO: Improve fields here
		BinaryHash:   RandomHash(t),
		IdentityHash: RandomHash(t),
		View:         view,
		ICSSeed:      RandomHash(t),
		ICSProof:     RandomHash(t).Bytes(),
	}
}

func CreateCommitInfoWithTestData(t *testing.T) common.CommitInfo {
	t.Helper()

	return common.CommitInfo{
		QC: &common.Qc{
			ID: RandomIdentifier(t),
		},
		Operator:                  RandomKramaID(t, 1),
		ClusterID:                 "cluster-1",
		View:                      5,
		RandomSet:                 []common.ValidatorIndex{0, 1},
		RandomSetSizeWithoutDelta: 4,
	}
}

func CreateParticipantWithTestData(t *testing.T, count int) common.ParticipantsState {
	t.Helper()

	p := make(common.ParticipantsState)

	for i := 0; i < count; i++ {
		p[RandomIdentifier(t)] = CreateStateWithTestData(t)
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

func GetIXSignature(t *testing.T, ixData *common.IxData, mnemonic string) []byte {
	t.Helper()

	rawIX, err := ixData.Bytes()
	require.NoError(t, err)

	sign, err := crypto.GetSignature(rawIX, mnemonic)
	require.NoError(t, err)

	rawSign, err := hex.DecodeString(sign)
	require.NoError(t, err)

	return rawSign
}

func GetLogicID(t *testing.T, id identifiers.Identifier) identifiers.Identifier {
	t.Helper()

	logicID, err := identifiers.GenerateLogicIDv0(id.Fingerprint(), 0)

	require.NoError(t, err)

	return logicID.AsIdentifier()
}

func GetLogicIDs(t *testing.T, count int) []identifiers.Identifier {
	t.Helper()

	logicIDs := make([]identifiers.Identifier, count)

	for i := 0; i < count; i++ {
		logicIDs[i] = GetLogicID(t, RandomIdentifier(t))
	}

	return logicIDs
}

// GetKramaIDAndNetworkKey returns kramaID and network key pair
func GetKramaIDAndNetworkKey(t *testing.T, nthValidator uint32) (identifiers.KramaID, []byte) {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:]) // fill sign key with random bytes
	require.NoError(t, err)

	// get private key and public key
	privKeyBytes, err := identifiers.GetRandomPrivateKeys(signKey)
	require.NoError(t, err)

	networkKey := privKeyBytes[32:]

	kramaID, err := identifiers.GenerateKramaIDv0( // Create kramaID from private key , public key
		identifiers.NetworkZone0,
		networkKey,
	)
	require.NoError(t, err)

	return kramaID, networkKey
}

// GetKramaIDAndConsensusKey returns kramaID and consensus key
func GetKramaIDAndConsensusKey(t *testing.T, nthValidator uint32) (identifiers.KramaID, []byte) {
	t.Helper()

	var signKey [32]byte

	_, err := rand.Read(signKey[:]) // fill sign key with random bytes
	require.NoError(t, err)

	// get private key and public key
	privKeyBytes, err := identifiers.GetRandomPrivateKeys(signKey)
	require.NoError(t, err)

	kramaID, err := identifiers.GenerateKramaIDv0( // Create kramaID from private key , public key
		identifiers.NetworkZone0,
		privKeyBytes[32:],
	)
	require.NoError(t, err)

	return kramaID, privKeyBytes[:32]
}

func GetRandomNumber(t *testing.T, ceil int) int {
	t.Helper()

	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(ceil)))
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

// CreateTestIxParticipants creates a list of id and a map of IxParticipants with default values
func CreateTestIxParticipants(t *testing.T, count int, genesisAccCount int) (
	[]identifiers.Identifier,
	map[identifiers.Identifier]common.ParticipantInfo,
) {
	t.Helper()

	ids := make([]identifiers.Identifier, count)

	ps := make(map[identifiers.Identifier]common.ParticipantInfo, 0)

	for i := 0; i < count; i++ {
		ids[i] = RandomIdentifier(t)
		ps[ids[i]] = common.ParticipantInfo{
			IsSigner:  true,
			LockType:  common.MutateLock,
			IsGenesis: false,
		}
	}

	for i := 0; i < genesisAccCount; i++ {
		ids[i] = RandomIdentifier(t)
		ps[ids[i]] = common.ParticipantInfo{
			IsSigner:  true,
			LockType:  common.MutateLock,
			IsGenesis: true,
		}
	}

	return ids, ps
}

func GetStorageMap(keys []string, values []string) map[string]string {
	storage := make(map[string]string)

	for i, key := range keys {
		storage[string(common.FromHex(key))] = values[i] // each hex character should be a byte
	}

	return storage
}

func GetHexEntries(t *testing.T, count int) []string {
	t.Helper()

	entries := make([]string, count)

	for i := 0; i < count; i++ {
		entries[i] = RandomHash(t).Hex()
	}

	return entries
}

func CreateTxPayload(t *testing.T, ixType common.IxOpType, beneficiary identifiers.Identifier) []byte {
	t.Helper()

	switch ixType {
	case common.IxParticipantCreate:
		return CreateRawParticipantCreatePayload(t, beneficiary)
	case common.IxAssetCreate:
		return CreateRawAssetCreatePayload(t)
	case common.IxAssetAction:
		return CreateRawAssetTransferPayload(t, beneficiary)
	// case common.IxAssetMint, common.IxAssetBurn:
	//	return CreateRawAssetSupplyPayload(t, beneficiary)
	case common.IxLogicDeploy, common.IxLogicInvoke, common.IxLogicEnlist:
		return CreateRawLogicPayload(t, identifiers.RandomLogicIDv0())
	default:
		return []byte{}
	}
}

func CreateParticipantCreatePayload(t *testing.T, id identifiers.Identifier) *common.ParticipantCreatePayload {
	t.Helper()

	if id.IsNil() {
		id = RandomIdentifier(t)
	}

	return &common.ParticipantCreatePayload{
		ID: id,
		KeysPayload: []common.KeyAddPayload{
			{
				Weight: 1000,
			},
		},
		Value: CreateAssetTransferPayload(t, id),
	}
}

func CreateAssetCreatePayload(t *testing.T) *common.AssetCreatePayload {
	t.Helper()

	return &common.AssetCreatePayload{
		Symbol:    GetRandomUpperCaseString(t, 5),
		MaxSupply: big.NewInt(2000),
		Standard:  common.MAS0,
	}
}

func CreateAssetTransferPayload(t *testing.T, id identifiers.Identifier) *common.AssetActionPayload {
	t.Helper()

	if id.IsNil() {
		id = RandomIdentifierWithZeroVariant(t)
	}

	payload, err := common.GetAssetActionPayload(common.KMOITokenAssetID, common.TransferEndpoint, &common.TransferParams{
		Beneficiary: id,
		Amount:      big.NewInt(1),
	})

	require.NoError(t, err)

	return payload
}

func CreateLogicPayload(t *testing.T, id identifiers.LogicID) *common.LogicPayload {
	t.Helper()

	return &common.LogicPayload{
		Manifest: []byte{1, 2, 3},
		LogicID:  id,
		Callsite: "hello",
	}
}

func CreateRawAssetCreatePayload(t *testing.T) []byte {
	t.Helper()

	payload := CreateAssetCreatePayload(t)

	rawPayload, err := payload.Bytes()
	require.NoError(t, err)

	return rawPayload
}

func CreateRawParticipantCreatePayload(t *testing.T, id identifiers.Identifier) []byte {
	t.Helper()

	payload := CreateParticipantCreatePayload(t, id)

	rawPayload, err := payload.Bytes()
	require.NoError(t, err)

	return rawPayload
}

func CreateRawAssetTransferPayload(t *testing.T, id identifiers.Identifier) []byte {
	t.Helper()

	payload := CreateAssetTransferPayload(t, id)

	rawPayload, err := payload.Bytes()
	require.NoError(t, err)

	return rawPayload
}

func CreateRawLogicPayload(t *testing.T, id identifiers.LogicID) []byte {
	t.Helper()

	payload := CreateLogicPayload(t, id)

	rawPayload, err := payload.Bytes()
	require.NoError(t, err)

	return rawPayload
}

func GetTestAssetIDFromAssetDescriptor(
	t *testing.T,
	id identifiers.Identifier,
	asset *common.AssetDescriptor,
) identifiers.AssetID {
	t.Helper()

	assetID, err := identifiers.GenerateAssetIDv0(
		id.Fingerprint(),
		id.Variant(),
		uint16(0),
		asset.Flags()...)
	require.NoError(t, err)

	return assetID
}

func CreateTestValidators(t *testing.T, count int) []*common.Validator {
	t.Helper()

	validators := make([]*common.Validator, count)

	for i := 0; i < count; i++ {
		kramaID, _ := identifiers.RandomKramaIDv0()

		validators[i] = &common.Validator{
			ID:              common.ValidatorIndex(i),
			KramaID:         kramaID,
			ActiveStake:     big.NewInt(int64(GetRandomNumber(t, 1000000))),
			InactiveStake:   big.NewInt(int64(GetRandomNumber(t, 1000000))),
			BehaviourTokens: big.NewInt(int64(GetRandomNumber(t, 1000000))),
			SocialTokens:    big.NewInt(int64(GetRandomNumber(t, 1000000))),
			Rewards:         big.NewInt(int64(GetRandomNumber(t, 1000000))),
			WalletAddress:   identifiers.RandomParticipantIDv0().AsIdentifier(),
		}
	}

	return validators
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
	moiIDPath[0] = identifiers.HardenedStartIndex + 0 // m/44'/6174'/0'
	moiIDPath[1] = 0                                  // m/44'/6174'/0'/0 ie., external
	moiIDPath[2] = 0                                  // m/44'/6174'/0'/0/0

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
	validatorPath[0] = identifiers.HardenedStartIndex + 5020 // hardened
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
	networkPath[0] = identifiers.HardenedStartIndex + 6020 // hardened
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

func TokenWithExpiry(
	t *testing.T,
	tokenID common.TokenID,
	amount *big.Int,
	expiry uint64,
) map[common.TokenID]*common.AmountWithExpiry {
	t.Helper()

	return map[common.TokenID]*common.AmountWithExpiry{
		tokenID: {
			Amount:    amount,
			ExpiresAt: expiry,
		},
	}
}

func TokenWithoutExpiry(
	t *testing.T,
	tokenID common.TokenID,
	amount *big.Int,
) map[common.TokenID]*big.Int {
	t.Helper()

	return map[common.TokenID]*big.Int{
		tokenID: amount,
	}
}
