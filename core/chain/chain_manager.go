package chain

import (
	"bytes"
	"encoding/json"
	"github.com/hashicorp/go-hclog"
	"github.com/pkg/errors"
	"gitlab.com/sarvalabs/moichain/mudra"
	id "gitlab.com/sarvalabs/moichain/mudra/kramaid"
	"gitlab.com/sarvalabs/polo/go-polo"

	"log"
	"math/big"
	"os"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"gitlab.com/sarvalabs/moichain/common/ktypes"
	"gitlab.com/sarvalabs/moichain/common/kutils"
	"gitlab.com/sarvalabs/moichain/guna"
	jug "gitlab.com/sarvalabs/moichain/jug"
)

type db interface {
	ReadEntry(key []byte) ([]byte, error)
	Contains(key []byte) (bool, error)
	CreateEntry(key []byte, value []byte) error
	CreateAndPublishEntry(key ktypes.Hash, value []byte) error
	UpdateAccMetaInfo(
		id ktypes.Address,
		height *big.Int,
		tesseractHash ktypes.Hash,
		accType ktypes.AccType,
		latticeExists bool,
		tesseractExists bool) (int32, int64, error)
}

type reputationEngine interface {
	UpdateInclusivity(key id.KramaID, delta int64) error
}
type stateManager interface {
	GetStateObjectByHash(addr ktypes.Address, hash ktypes.Hash) (*guna.StateObject, error)
	CreateStateObject(addr ktypes.Address, accType ktypes.AccType) *guna.StateObject
	GetLatestStateObject(addr ktypes.Address) (*guna.StateObject, error)
	GetDirtyObject(addr ktypes.Address) (*guna.StateObject, error)
	GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error)
	DeleteStateObject(addr ktypes.Address)
	GetLatestContext(addr ktypes.Address) (ktypes.Hash, []id.KramaID, []id.KramaID, error)
	GetPublicKeys(id ...id.KramaID) (keys [][]byte, err error)
	Broadcast(addrs ktypes.Address)
	IsGenesis(addr ktypes.Address) (bool, error)
	FetchContextLock(ts *ktypes.Tesseract) ([]*ktypes.NodeSet, error)
	GetContextByHash(hash ktypes.Hash) ([]id.KramaID, []id.KramaID, error)
}
type ixpool interface {
	ResetWithHeaders(ts *ktypes.Tesseract)
}

type ChainManager struct {
	db              db
	mux             *kutils.TypeMux
	tesseractSub    *kutils.Subscription
	ixpool          ixpool
	tesseracts      *lru.Cache
	exec            *jug.Exec
	sm              stateManager
	latticeLocks    map[ktypes.Address]*sync.Mutex
	logger          hclog.Logger
	knownTesseracts *ktypes.KnownCache
	senatus         reputationEngine
}

func NewChainManager(
	db db,
	sm stateManager,
	logger hclog.Logger,
	mux *kutils.TypeMux,
	ix ixpool,
	cache *lru.Cache,
	exec *jug.Exec,
	senatus reputationEngine,
) *ChainManager {
	c := &ChainManager{
		db:              db,
		mux:             mux,
		ixpool:          ix,
		sm:              sm,
		tesseracts:      cache,
		exec:            exec,
		knownTesseracts: ktypes.NewKnownCache(150),
		latticeLocks:    make(map[ktypes.Address]*sync.Mutex),
		logger:          logger.Named("Chain-manager"),
		senatus:         senatus,
	}

	return c
}
func (c *ChainManager) GetTesseract(hash ktypes.Hash) (*ktypes.Tesseract, error) {
	tesseractData, isCached := c.tesseracts.Get(hash)
	if !isCached {
		tesseract := new(ktypes.Tesseract)

		buf, err := c.db.ReadEntry(hash.Bytes())
		if err != nil {
			return nil, errors.Wrap(err, ktypes.ErrFetchingTesseract.Error())
		}

		if err = polo.Depolorize(tesseract, buf); err != nil {
			log.Fatal("Un marshalling error")
		}

		c.tesseracts.Add(hash, tesseract)

		return tesseract, nil
	}

	tesseract, ok := tesseractData.(*ktypes.Tesseract)
	if !ok {
		return nil, ktypes.ErrInterfaceConversion
	}

	return tesseract, nil
}
func (c *ChainManager) GetLatestTesseract(addr ktypes.Address) (*ktypes.Tesseract, error) {
	if addr == ktypes.NilAddress {
		return nil, ktypes.ErrInvalidAddress
	}

	return c.sm.GetLatestTesseract(addr)
}
func (c *ChainManager) GetReceipt(addr ktypes.Address, ixHash ktypes.Hash) (*ktypes.Receipt, error) {
	ts, err := c.GetLatestTesseract(addr)
	if err != nil {
		return nil, errors.Wrap(ktypes.ErrReceiptNotFound, err.Error())
	}

	for ts.Header.PrevHash != ktypes.NilHash {
		for _, ix := range ts.Interactions() {
			if ix.GetIxHash() == ixHash {
				return c.getReceipt(ix.GetIxHash(), ts.Body.ReceiptHash)
			}
		}

		previousTesseract, err := c.GetTesseract(ts.Header.PrevHash)
		if err != nil {
			return nil, errors.Wrap(ktypes.ErrReceiptNotFound, err.Error())
		}

		ts = previousTesseract
	}

	return nil, ktypes.ErrReceiptNotFound
}
func (c *ChainManager) getReceipt(ixHash, receiptRoot ktypes.Hash) (*ktypes.Receipt, error) {
	rawData, err := c.db.ReadEntry(receiptRoot.Bytes())
	if err != nil {
		c.logger.Error("Error fetching receipt root", "err=", err.Error(), receiptRoot.Hex(), ixHash.Hex())

		return nil, err
	}

	receipts := new(ktypes.Receipts)

	if err := polo.Depolorize(receipts, rawData); err != nil {
		return nil, err
	}

	return receipts.GetReceipt(ixHash)
}
func (c *ChainManager) HasTesseract(hash []byte) bool {
	exists, err := c.db.Contains(hash)
	if err != nil {
		log.Fatal(err)
	}

	return exists
}

func (c *ChainManager) IsGenesisAt(address ktypes.Address, hash ktypes.Hash) (bool, error) {
	ts, err := c.GetTesseract(hash)
	if err != nil {
		return false, err
	}

	object, err := c.sm.GetStateObjectByHash(ts.Header.Address, ts.Body.StateHash)
	if err != nil {
		return false, err
	}

	_, err = object.GetStorageEntry(ktypes.GetHash(address.Bytes()))
	if err != nil {
		return true, nil
	}

	return false, nil
}
func (c *ChainManager) VerifyHeaders(ts *ktypes.Tesseract) error {
	var (
		isGenesis bool
		err       error
	)

	c.logger.Trace("Verifying headers", "addr", ts.Header.Address.Hex())

	if ts.Header.ClusterID == "genesis" {
		return nil
	}

	if info, ok := ts.Header.ContextLock[guna.GenesisAddress]; !ok {
		isGenesis, err = c.sm.IsGenesis(ts.Header.Address)
	} else {
		isGenesis, err = c.IsGenesisAt(ts.Header.Address, info.TesseractHash)
	}

	if err != nil {
		return errors.Wrap(err, "Sarga account not found")
	}

	if !isGenesis {
		parent, err := c.GetTesseract(ts.Header.PrevHash)
		if err != nil {
			return err
		}

		// Check Heights
		if parent.Header.Height != ts.Header.Height-1 {
			return ktypes.ErrInvalidHeight
		}
		// TODO: Add more checks
		// Check time stamp
		if ts.Header.Timestamp < parent.Header.Timestamp {
			return ktypes.ErrInvalidBlockTime
		}
	}

	return nil
}
func (c *ChainManager) append(groupHash ktypes.Hash, ts map[ktypes.Address]*ktypes.Tesseract) error {
	for _, v := range ts {
		if !bytes.Equal(groupHash.Bytes(), v.Header.GroupHash.Bytes()) {
			return errors.New("groupHash mismatch")
		}
	}

	for k, v := range ts {
		rawData, err := c.db.ReadEntry(v.Body.ConsensusProof.ICSHash.Bytes())
		if err != nil {
			c.logger.Error("Unable to fetch ICS Info", "err", err)

			return err
		}

		if err := c.addTesseract(true, k, v, true, true); err != nil {
			return err
		}

		c.sm.Broadcast(v.Header.Address)
		c.ixpool.ResetWithHeaders(v)

		event := kutils.NewMinedTesseractEvent{
			Tesseract: v,
			Delta:     make(map[ktypes.Hash][]byte),
		}

		event.Delta[v.Body.ConsensusProof.ICSHash] = rawData

		if err := c.mux.Post(event); err != nil {
			panic(err)
		}

		c.knownTesseracts.Add(v.Hash())
	}

	return nil
}

func (c *ChainManager) verifySignatures(ts *ktypes.Tesseract, info *ktypes.ICSClusterInfo) (bool, error) {
	//ix := ts.Body.Interactions[0]
	ics := ktypes.NewICSNodes(6)

	var (
		randomKeys [][]byte
	)

	nodeSets, err := c.sm.FetchContextLock(ts)
	if err != nil {
		return false, err
	}

	ics.UpdateNodeSet(ktypes.SenderBehaviourSet, nodeSets[0])
	ics.UpdateNodeSet(ktypes.SenderRandomSet, nodeSets[1])
	ics.UpdateNodeSet(ktypes.ReceiverBehaviourSet, nodeSets[2])
	ics.UpdateNodeSet(ktypes.ReceiverRandomSet, nodeSets[3])

	randomKeys, err = c.sm.GetPublicKeys(ktypes.ToKIPPeerID(info.RandomSet)...)
	if err != nil {
		return false, err
	}

	ics.UpdateNodeSet(ktypes.RandomSet,
		ktypes.NewNodeSet(ktypes.ToKIPPeerID(info.RandomSet), randomKeys))

	publicKeys := make([][]byte, 0, ts.Header.Extra.VoteSet.TrueIndicesSize())
	votesCounter := make([]int, 5) //Only 5 because we don't consider observer nodes vote

	for _, index := range ts.Header.Extra.VoteSet.GetTrueIndices() {
		slotID, _, _, publicKey := ics.GetKramaID(int32(index))
		if slotID != -1 && ts.Header.Extra.VoteSet.GetIndex(index) {
			publicKeys = append(publicKeys, publicKey)
			votesCounter[slotID]++
		} else {
			c.logger.Debug("Error fetching validator address", "index", index)
		}
	}

	senderContextSize := votesCounter[0] + votesCounter[1]
	receiverContextSize := votesCounter[2] + votesCounter[3]
	randomContextSize := votesCounter[4]

	if senderContextSize < ics.SenderQuorumSize() ||
		receiverContextSize < ics.ReceiverQuorumSize() ||
		randomContextSize < ics.RandomQuorumSize() {
		return false, ktypes.ErrQuorumFailed
	}

	vote := ktypes.CanonicalVote{
		Type:   ktypes.PRECOMMIT,
		Round:  ts.Header.Extra.Round,
		GridID: ts.Header.Extra.GridID,
	}

	verified, err := mudra.VerifyAggregateSignature(polo.Polorize(vote), ts.Header.Extra.CommitSignature, publicKeys)
	if err != nil {
		return false, errors.Wrap(ktypes.ErrSignatureVerificationFailed, err.Error())
	}

	return verified, nil
}
func (c *ChainManager) AddTesseractWithOutState(
	ts *ktypes.Tesseract,
	sender id.KramaID,
	ics *ktypes.ICSClusterInfo,
) error {
	validSeal, err := c.validSeal(ts, sender)
	if !validSeal {
		c.logger.Error("Error validating tesseract seal ", "err", err)

		return ktypes.ErrInvalidSeal
	}

	if exists, _ := c.db.Contains(ts.Hash().Bytes()); exists {
		return nil
	}

	if err := c.VerifyHeaders(ts); err != nil {
		return err
	}

	verified, err := c.verifySignatures(ts, ics)
	if err != nil {
		return errors.Wrap(err, ktypes.ErrSignatureVerificationFailed.Error())
	}

	if !verified {
		return ktypes.ErrSignatureVerificationFailed
	}

	if err := c.addTesseract(true, ts.Header.Address, ts, false, false); err != nil {
		log.Panic(err)
	}

	// clean up any existing state objects
	c.sm.DeleteStateObject(ts.Address())

	// fetch context info for agora

	context, err := c.fetchContextForAgora(ts)
	if err != nil {
		c.logger.Error("Error fetching context for agora", "error", err)
	}

	event := kutils.TesseractSyncEvent{Tesseract: ts, Context: context}
	if err := c.mux.Post(event); err != nil {
		log.Panic(err)
	}

	if len(ts.Interactions()) > 0 {
		c.ixpool.ResetWithHeaders(ts)
	}

	c.logger.Info("Added tesseract without state", "Addr", ts.Header.Address.Hex(), "Hash", ts.Hash().Hex())

	return nil
}

func (c *ChainManager) fetchContextForAgora(t *ktypes.Tesseract) ([]id.KramaID, error) {
	tesseractHash := t.Hash()
	address := t.Address()
	peers := make([]id.KramaID, 0)

	for tesseractHash != ktypes.NilHash {
		if len(peers) >= 10 {
			break
		}

		ts, err := c.GetTesseract(tesseractHash)
		if err != nil {
			return nil, errors.Wrap(err, "error fetching tesseract")
		}
		// fetch the context delta
		deltaGroup := ts.Body.ContextDelta[address]
		// add the delta peers to list
		peers = append(peers, deltaGroup.BehaviouralNodes...)
		peers = append(peers, deltaGroup.RandomNodes...)

		behaviour, random, err := c.sm.GetContextByHash(ts.Header.ContextLock[address].ContextHash)
		if err != nil {
			tesseractHash = ts.Header.PrevHash

			continue
		}

		peers = append(peers, behaviour...)
		peers = append(peers, random...)

		break
	}

	return peers, nil
}

func (c *ChainManager) AppendTesseracts(
	groupHash ktypes.Hash,
	ts map[ktypes.Address]*ktypes.Tesseract,
	dirtyStorage map[ktypes.Hash][]byte,
) error {
	for key, value := range dirtyStorage {
		if err := c.db.CreateAndPublishEntry(key, value); err != nil {
			c.logger.Error("Error writing keys to db")
			log.Panic(err) //We panic here, this should not occur at all.
		}
	}

	return c.append(groupHash, ts)
}
func (c *ChainManager) validSeal(ts *ktypes.Tesseract, id id.KramaID) (bool, error) {
	publicKey, err := c.sm.GetPublicKeys(id)
	if err != nil {
		c.logger.Error("Error fetching public key", "err", err)

		return false, err
	}

	return mudra.Verify(ts.Bytes(), ts.Seal, publicKey[0])
}
func (c *ChainManager) tesseractReceivedLoop() {
	for obj := range c.tesseractSub.Chan() {
		if t, ok := obj.Data.(kutils.TesseractReceivedEvent); ok {
			if !c.knownTesseracts.Contains(t.Tesseract.Hash()) {
				if err := c.AddTesseractWithOutState(t.Tesseract, t.Sender, t.ClusterInfo); err != nil {
					log.Panic("adding tesseract failed ", err, t.Tesseract.Header.Address.Hex(), t.Tesseract.Hash().Hex())
				} else {
					c.knownTesseracts.Add(t.Tesseract.Hash())
				}
			} else {
				c.logger.Info("Skipping tesseract", "hash", t.Tesseract.Hash())
			}
		}
	}
}

func (c *ChainManager) Start() {
	c.tesseractSub = c.mux.Subscribe(kutils.TesseractReceivedEvent{})
	c.tesseractReceivedLoop()
}
func (c *ChainManager) Close() {
	c.tesseractSub.Unsubscribe() //FIXME: Fix race condition here, log-id 1
	log.Println("Closing Chain manager")
}

func (c *ChainManager) AddGenesis(addr ktypes.Address, t *ktypes.Tesseract) error {
	return c.addTesseract(true, addr, t, true, true)
}

func (c *ChainManager) addTesseract(
	cache bool,
	addr ktypes.Address,
	t *ktypes.Tesseract,
	stateExists,
	tesseractExists bool,
) error {
	var accType ktypes.AccType

	if _, ok := c.latticeLocks[addr]; !ok {
		c.latticeLocks[addr] = new(sync.Mutex)
	}

	c.latticeLocks[addr].Lock()
	defer func() {
		c.latticeLocks[addr].Unlock()
		delete(c.latticeLocks, addr)
	}()

	tesseractHash := t.Hash()
	latticeExists := true

	if exists, err := c.db.Contains(t.Header.PrevHash.Bytes()); !exists || err != nil {
		latticeExists = false
	}

	if stateExists {
		stateObject, err := c.sm.GetDirtyObject(addr)
		if err != nil {
			return err
		}

		accType = stateObject.GetAccountType()

		for k, v := range stateObject.GetDirtyStorage() {
			if err := c.db.CreateEntry(k.Bytes(), v); err != nil {
				c.logger.Error("Error writing key to db", "key", k.Hex())
			}
		}
	}

	if err := c.db.CreateEntry(tesseractHash.Bytes(), polo.Polorize(t)); err != nil {
		return err
	}

	bucketNo, updateCount, err := c.db.UpdateAccMetaInfo(
		addr,
		new(big.Int).SetUint64(t.Header.Height),
		tesseractHash,
		accType,
		latticeExists,
		tesseractExists,
	)
	if err != nil {
		return err
	}

	// update peer occupancy metrics
	if err := c.UpdateNodeInclusivity(t.ContextDelta()); err != nil {
		return errors.Wrap(ktypes.ErrUpdatingInclusivity, err.Error())
	}

	if updateCount != -1 && bucketNo != -1 {
		if err := c.mux.Post(kutils.SyncStatusUpdate{BucketID: bucketNo, Count: updateCount}); err != nil {
			log.Panicln(err)
		}
	}

	if cache {
		c.tesseracts.Add(addr, tesseractHash)
		c.tesseracts.Add(tesseractHash, t)
	}

	log.Println("tesseract  added ....!!!!!", addr.Hex(), tesseractHash.Hex())

	return nil
}

func (c *ChainManager) SetupGenesis(path string) error {
	if path == "nil" {
		c.logger.Debug("Skipping genesis")

		return nil
	}

	genesisAccounts := new(Genesis)

	data, err := os.ReadFile(path)
	if err != nil {
		return ktypes.ErrGenesisSetupFailed
	}

	if err = json.Unmarshal(data, genesisAccounts); err != nil {
		return err
	}

	if err = c.setupSargaAccount(genesisAccounts.Accounts); err != nil {
		return ktypes.ErrGenesisSetupFailed
	}

	for _, v := range genesisAccounts.Accounts {
		addr := ktypes.HexToAddress(v.Address)
		if addr == guna.GenesisAddress {
			continue
		}

		// Update the context
		stateObject := c.sm.CreateStateObject(addr, v.AccType)
		bContext := ktypes.ToKIPPeerID(v.BehaviourContext)
		rContext := ktypes.ToKIPPeerID(v.RandomContext)

		if _, err = stateObject.CreateContext(bContext, rContext); err != nil {
			return errors.New("context initiation failed in genesis")
		}

		if len(v.AssetDetails) > 0 {
			asset := v.AssetDetails[0]

			assetID, err := stateObject.CreateAsset(
				uint8(asset.Dimension),
				asset.IsFungible,
				asset.IsMintable,
				asset.Symbol,
				int64(asset.TotalSupply),
				nil,
			)
			if err != nil {
				c.logger.Error("Error creating asset", err)
			}

			c.logger.Info("Asset created ", "id", assetID)
		} else if v.Balances != nil {
			for _, balance := range v.Balances {
				stateObject.AddBalance(ktypes.AssetID(balance.AssetID), new(big.Int).SetInt64(balance.Amount))
			}
		}

		stateHash, err := stateObject.Commit()
		if err != nil {
			return err
		}

		contextDelta := map[ktypes.Address]*ktypes.DeltaGroup{
			addr: {
				BehaviouralNodes: bContext,
				RandomNodes:      rContext,
			},
		}

		tesseract := CreateGenesisTesseract(addr, stateHash, stateObject.GetContextHash(), contextDelta)

		if err := c.AddGenesis(addr, tesseract); err != nil {
			return errors.New("error adding genesis tesseract ")
		}
	}

	return nil
}

func (c *ChainManager) AddKnownHashes(tesseracts []*ktypes.Tesseract) {
	for _, v := range tesseracts {
		c.knownTesseracts.Add(v.Hash())
	}
}

func (c *ChainManager) UpdateNodeInclusivity(delta ktypes.ContextDelta) error {
	for _, deltaGroup := range delta {
		for _, kramaID := range deltaGroup.BehaviouralNodes {
			if err := c.senatus.UpdateInclusivity(kramaID, 1); err != nil {
				return err
			}
		}

		for _, kramaID := range deltaGroup.RandomNodes {
			if err := c.senatus.UpdateInclusivity(kramaID, 1); err != nil {
				return err
			}
		}

		for _, kramaID := range deltaGroup.ReplacedNodes {
			if err := c.senatus.UpdateInclusivity(kramaID, -1); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *ChainManager) setupSargaAccount(accounts []AccountInfo) error {
	var contextDelta ktypes.ContextDelta

	stateObject := c.sm.CreateStateObject(guna.GenesisAddress, ktypes.SargaAccount)

	for _, info := range accounts {
		addr := ktypes.HexToAddress(info.Address)
		if addr == guna.GenesisAddress {
			bContext := ktypes.ToKIPPeerID(info.BehaviourContext)
			rContext := ktypes.ToKIPPeerID(info.RandomContext)
			contextDelta = map[ktypes.Address]*ktypes.DeltaGroup{
				addr: {
					BehaviouralNodes: bContext,
					RandomNodes:      rContext,
				},
			}

			if _, err := stateObject.CreateContext(bContext, rContext); err != nil {
				return errors.New("context initiation failed in genesis")
			}
		} else {
			stateObject.AddAccountGenesisInfo(addr, GenesisIxHash)
		}
	}

	stateHash, err := stateObject.Commit()
	if err != nil {
		return err
	}

	tesseract := CreateGenesisTesseract(guna.GenesisAddress, stateHash, stateObject.GetContextHash(), contextDelta)

	if err := c.AddGenesis(guna.GenesisAddress, tesseract); err != nil {
		c.logger.Error("Error adding genesis", "err", err)

		return errors.New("error adding genesis tesseract ")
	}

	return nil
}
