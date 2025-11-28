package state

import (
	"math/big"

	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/VictoriaMetrics/fastcache"

	"github.com/decred/dcrd/crypto/blake256"
	iradix "github.com/hashicorp/go-immutable-radix"
	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/state/tree"
	"github.com/sarvalabs/go-moi/storage"
)

// StorageTree: Holds key-value pairs associated with logic (identified by logicID) for storage.
// LogicTree: Manages logicObject for each logic.
// MetaStorageTree: Keeps track of the root of the storage tree for each logic.

type Object struct {
	cache     *lru.Cache
	treeCache *fastcache.Cache

	id        identifiers.Identifier
	accType   common.AccountType
	data      common.Account
	isGenesis bool // used by transition objects in execution

	db Store

	deeds *Deeds
	keys  common.AccountKeys

	assetTree       tree.MerkleTree
	logicTree       tree.MerkleTree
	metaStorageTree tree.MerkleTree
	storageTrees    map[identifiers.Identifier]tree.MerkleTree
	fileTree        tree.MerkleTree //nolint:unused

	dirtyEntries Storage
	receipts     common.Receipts

	storageTreeTxns map[identifiers.Identifier]*iradix.Txn
	assetTreeTxn    *iradix.Txn
	logicTreeTxn    *iradix.Txn

	files       map[common.Hash][]byte
	metaContext *MetaContextObject
	metrics     *Metrics
}

func (object *Object) AccType() common.AccountType {
	return object.accType
}

func NewStateObject(
	id identifiers.Identifier,
	cache *lru.Cache,
	treeCache *fastcache.Cache,
	db Store,
	account common.Account,
	metrics *Metrics,
	isGenesis bool,
) *Object {
	o := &Object{
		accType:         account.AccType,
		cache:           cache,
		treeCache:       treeCache,
		db:              db,
		data:            account,
		id:              id,
		deeds:           nil,
		keys:            nil,
		files:           make(map[common.Hash][]byte),
		dirtyEntries:    make(Storage),
		receipts:        make(common.Receipts),
		storageTreeTxns: make(map[identifiers.Identifier]*iradix.Txn),
		storageTrees:    make(map[identifiers.Identifier]tree.MerkleTree),
		metrics:         metrics,
		isGenesis:       isGenesis,
	}

	return o
}

func (object *Object) Identifier() identifiers.Identifier {
	return object.id
}

// IsGenesis indicates whether the object is a genesis object.
func (object *Object) IsGenesis() bool {
	return object.isGenesis
}

// Data returns the account info associated with the object.
func (object *Object) Data() *common.Account {
	return &object.data
}

// AccountType returns the type of the account.
func (object *Object) AccountType() common.AccountType {
	return object.accType
}

// AccountState returns the current state of the account.
func (object *Object) AccountState() common.Account {
	return object.data
}

// Deeds returns the deeds associated. If the deeds is not already loaded,
// it attempts to load it from the store. An error is returned if loading fails.
func (object *Object) Deeds() (*Deeds, error) {
	if object.deeds == nil {
		if err := object.loadDeeds(); err != nil {
			return nil, errors.Wrap(err, "failed to load deeds")
		}
	}

	return object.deeds, nil
}

func (object *Object) SetAccount(account common.Account) {
	object.data = account
}

// CreateDeedsEntry creates a new entry in the deeds with the specified key and value.
// If an entry with the same key already exists, an error is returned.
func (object *Object) CreateDeedsEntry(key identifiers.Identifier) error {
	deeds, err := object.Deeds()
	if err != nil {
		return err
	}

	if _, ok := deeds.Entries[key]; ok {
		return common.ErrAssetAlreadyRegistered
	}

	deeds.Entries[key] = struct{}{}

	return nil
}

func (object *Object) SetMetaContextObject(mCtx *MetaContextObject) {
	object.metaContext = mCtx
}

// updateAssetTree ensures the asset transaction tree is initialized and inserts the given asset object.
func (object *Object) updateAssetTree(assetID identifiers.AssetID, assetObject *AssetObject) {
	// Initialize assetTreeTxn if not already done
	if object.assetTreeTxn == nil {
		object.assetTreeTxn = iradix.New().Txn()
	}

	// Insert the updated asset object into the txn tree
	object.assetTreeTxn.Insert(assetID.Bytes(), assetObject)
}

// updateLogicTree ensures the logic transaction tree is initialized and inserts the given logic object.
func (object *Object) updateLogicTree(logicID identifiers.Identifier, logicObject *LogicObject) {
	// Initialize logicTreeTxn if not already done
	if object.logicTreeTxn == nil {
		object.logicTreeTxn = iradix.New().Txn()
	}

	// Insert the updated logic object into the txn tree
	object.logicTreeTxn.Insert(logicID.Bytes(), logicObject)
}

// Balances retrieves and returns the balances of all the assets held by the participant.
func (object *Object) Balances() (map[identifiers.AssetID]map[common.TokenID]*big.Int, error) {
	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	it := assetTree.NewIterator()
	balances := make(map[identifiers.AssetID]map[common.TokenID]*big.Int)

	for it.Next() {
		if it.Leaf() {
			key, err := assetTree.GetPreImageKey(common.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			assetID := identifiers.AssetID(key)

			assetObject, err := object.getAssetObject(assetID, false)
			if err != nil {
				return nil, err
			}

			balances[assetID] = assetObject.Balance
		}
	}

	return balances, nil
}

func (object *Object) getMetaCtxObject() (*MetaContextObject, error) {
	if object.metaContext == nil {
		if err := object.loadMetaCtxObject(); err != nil {
			return nil, err
		}
	}

	return object.metaContext, nil
}

func (object *Object) loadMetaCtxObject() error {
	if object.data.ContextHash.IsNil() {
		object.metaContext = &MetaContextObject{
			SubAccounts: make(map[identifiers.Identifier]identifiers.Identifier),
		}

		return nil
	}

	object.metaContext = new(MetaContextObject)

	data, err := object.db.GetContext(object.id, object.data.ContextHash)
	if err != nil {
		return err
	}

	return object.metaContext.FromBytes(data)
}

func (object *Object) UpdateSubAccount(subAccount, targetAccount identifiers.Identifier) error {
	metaContext, err := object.getMetaCtxObject()
	if err != nil {
		return err
	}

	if metaContext.SubAccounts == nil {
		object.metaContext.SubAccounts = make(map[identifiers.Identifier]identifiers.Identifier)
	}

	metaContext.SubAccounts[subAccount] = targetAccount

	return nil
}

func (object *Object) SubAccountCount() int {
	if _, err := object.getMetaCtxObject(); err != nil {
		panic(err)
	}

	return len(object.metaContext.SubAccounts)
}

func (object *Object) UpdateInheritedAccount(inheritedAccount identifiers.Identifier) error {
	if _, err := object.getMetaCtxObject(); err != nil {
		panic(err)
	}

	object.metaContext.InheritedAccount = inheritedAccount

	return nil
}

func (object *Object) UpdateKeys(keys common.AccountKeys) {
	object.keys = keys
}

func (object *Object) loadKeys() error {
	if object.data.KeysHash.IsNil() {
		object.keys = make(common.AccountKeys, 0)

		return nil
	}

	object.keys = make(common.AccountKeys, 0)

	data, err := object.db.GetAccountKeys(object.id, object.data.KeysHash)
	if err != nil {
		return err
	}

	return object.keys.FromBytes(data)
}

func (object *Object) KeysLen() int {
	if object.keys == nil {
		if err := object.loadKeys(); err != nil {
			panic(err)
		}
	}

	return len(object.keys)
}

func (object *Object) AppendAccountKeys(keys common.AccountKeys) error {
	if object.keys == nil {
		if err := object.loadKeys(); err != nil {
			return errors.Wrap(err, "failed to load acc keys")
		}
	}

	object.keys = append(object.keys, keys...)

	return nil
}

func (object *Object) IncrementSequenceID(keyID uint64) error {
	if object.keys == nil {
		if err := object.loadKeys(); err != nil {
			return errors.Wrap(err, "failed to load acc keys")
		}
	}

	object.keys[keyID].SequenceID += 1

	return nil
}

func (object *Object) RevokeAccountKeys(revokePayload []common.KeyRevokePayload) error {
	if object.keys == nil {
		if err := object.loadKeys(); err != nil {
			return errors.Wrap(err, "failed to load acc keys")
		}
	}

	for _, revoke := range revokePayload {
		object.keys[revoke.KeyID].Revoked = true
	}

	return nil
}

func (object *Object) getAccountKey(keyID uint64) (*common.AccountKey, error) {
	if object.keys == nil {
		if err := object.loadKeys(); err != nil {
			return nil, errors.Wrap(err, "failed to load acc keys")
		}
	}

	if keyID >= uint64(object.KeysLen()) {
		return nil, common.ErrInvalidKeyID
	}

	return object.keys[keyID], nil
}

func (object *Object) SequenceID(keyID uint64) (uint64, error) {
	key, err := object.getAccountKey(keyID)
	if err != nil {
		return 0, err
	}

	return key.SequenceID, nil
}

func (object *Object) PublicKey(keyID uint64) ([]byte, error) {
	key, err := object.getAccountKey(keyID)
	if err != nil {
		return nil, err
	}

	return key.PublicKey, nil
}

func (object *Object) AccountKeys() (common.AccountKeys, error) {
	if object.keys == nil {
		if err := object.loadKeys(); err != nil {
			return nil, errors.Wrap(err, "failed to load acc keys")
		}
	}

	return object.keys, nil
}

// BalanceOf returns the balance of a specific asset, identified by its asset id.
func (object *Object) BalanceOf(id identifiers.AssetID, tokenID common.TokenID) (*big.Int, error) {
	assetObject, err := object.getAssetObject(id, false)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	amount, ok := assetObject.Balance[tokenID]
	if !ok {
		return nil, common.ErrTokenNotFound
	}

	return amount, nil
}

// AddBalance increments the balance of the specified asset based on the given amount.
func (object *Object) AddBalance(assetID identifiers.AssetID, tokenID common.TokenID,
	amount *big.Int, metadata *MetaData,
) error {
	var (
		assetObject *AssetObject
		err         error
	)

	// Insert a new asset object if the asset doesn't exist
	assetObject, err = object.getAssetObject(assetID, true)
	if err != nil {
		assetObject = NewAssetObject(nil)
		if err = object.InsertNewAssetObject(assetID, assetObject); err != nil {
			return err
		}
	}

	bal, ok := assetObject.Balance[tokenID]
	if !ok {
		assetObject.Balance = map[common.TokenID]*big.Int{tokenID: amount}
	} else {
		assetObject.Balance[tokenID].Add(bal, amount)
	}

	if metadata != nil {
		assetObject.TokenMetaData[tokenID] = metadata
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// SubBalance decrements the balance of the specified asset based the given amount.
func (object *Object) SubBalance(
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	amount *big.Int,
) (*MetaData, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	_, ok := assetObject.Balance[tokenID]
	if !ok {
		return nil, common.ErrTokenNotFound
	}

	assetObject.Balance[tokenID].Sub(assetObject.Balance[tokenID], amount)

	metadata := assetObject.TokenMetaData[tokenID]

	if assetObject.Balance[tokenID].Cmp(big.NewInt(0)) == 0 {
		delete(assetObject.Balance, tokenID)
		assetObject.deleteTokenMetadata(tokenID)
	}

	object.updateAssetTree(assetID, assetObject)

	return metadata, nil
}

// CreateLockup transfers a specified amount from the participant's asset balance into a lockup
// associated with a specified id. The lockup represents funds reserved for a specific purpose,
// reducing the available balance for the participant.
func (object *Object) CreateLockup(assetID identifiers.AssetID,
	tokenID common.TokenID, beneficiary identifiers.Identifier, amount *big.Int,
) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	bal, ok := assetObject.Balance[tokenID]
	if !ok {
		return common.ErrTokenNotFound
	}

	// Deduct the amount from asset balance
	bal.Sub(assetObject.Balance[tokenID], amount)

	if bal.Cmp(big.NewInt(0)) == 0 {
		delete(assetObject.Balance, tokenID)
	}

	if len(assetObject.Lockup) == 0 {
		assetObject.Lockup = make(map[identifiers.Identifier]map[common.TokenID]*common.AmountWithExpiry)
	}

	if lockups, ok := assetObject.Lockup[beneficiary]; ok {
		lockup, ok := lockups[tokenID]
		if ok {
			amount = new(big.Int).Add(lockup.Amount, amount)
		}

		// Increment the lockup amount if the lockup already exist
		assetObject.Lockup[beneficiary][tokenID] = &common.AmountWithExpiry{
			Amount:    amount,
			ExpiresAt: lockup.ExpiresAt,
		}
	} else {
		// Create a new lockup if it doesn't exist
		assetObject.Lockup[beneficiary] = make(map[common.TokenID]*common.AmountWithExpiry)
		assetObject.Lockup[beneficiary][tokenID] = &common.AmountWithExpiry{
			Amount: amount,
		}
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// ReleaseLockup reduces the lockup amount from a specified id for the given asset.
// If the lockup amount becomes zero, the lockup entry is deleted from the asset object.
func (object *Object) ReleaseLockup(assetID identifiers.AssetID, tokenID common.TokenID,
	beneficiary identifiers.Identifier, amount *big.Int,
) (*MetaData, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	lockup := assetObject.Lockup[beneficiary][tokenID]

	metadata := assetObject.TokenMetaData[tokenID]

	// Decrement the mandate balance by the given amount
	lockup.Amount.Sub(lockup.Amount, amount)

	// If the lockup amount is zero, remove the lockup for the specified id.
	if lockup.Amount.Cmp(big.NewInt(0)) == 0 {
		delete(assetObject.Lockup[beneficiary], tokenID)
		assetObject.deleteTokenMetadata(tokenID)
	}

	if len(assetObject.Lockup[beneficiary]) == 0 {
		delete(assetObject.Lockup, beneficiary)
	}

	object.updateAssetTree(assetID, assetObject)

	return metadata, nil
}

// Lockups retrieves all active lockups across all assets in the AssetTree.
// It iterates through the AssetTree, collects lockup information, and returns it as a slice.
func (object *Object) Lockups() ([]common.AssetMandateOrLockup, error) {
	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	it := assetTree.NewIterator()
	lockups := make([]common.AssetMandateOrLockup, 0)

	for it.Next() {
		if it.Leaf() {
			key, err := assetTree.GetPreImageKey(common.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			assetID := identifiers.AssetID(key)

			assetObject, err := object.getAssetObject(assetID, false)
			if err != nil {
				return nil, err
			}

			for id, amount := range assetObject.Lockup {
				lockups = append(lockups, common.AssetMandateOrLockup{
					AssetID: assetID,
					ID:      id,
					Amount:  amount,
				})
			}
		}
	}

	return lockups, nil
}

// GetLockup retrieves the lockup amount for the given logic and asset id.
func (object *Object) GetLockup(
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	operatorID identifiers.Identifier,
) (*common.AmountWithExpiry, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	lockUps, ok := assetObject.Lockup[operatorID]
	if !ok {
		return nil, common.ErrLockupNotFound
	}

	if amount, ok := lockUps[tokenID]; ok {
		return amount, nil
	}

	return nil, common.ErrLockupNotFound
}

// CreateMandate assigns a spending mandate to an id for the specified asset.
// The mandate grants the recipient the authorization to spend the specified amount on behalf of the participant.
func (object *Object) CreateMandate(
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	beneficiary identifiers.Identifier,
	amount *big.Int,
	expiresAt uint64,
) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if mandates, ok := assetObject.Mandate[beneficiary]; ok {
		if mandate, ok := mandates[tokenID]; ok {
			amount = new(big.Int).Add(mandate.Amount, amount)
		}

		// Increment the mandate amount if the mandate already exist
		assetObject.Mandate[beneficiary][tokenID] = &common.AmountWithExpiry{
			Amount:    amount,
			ExpiresAt: expiresAt,
		}
	} else {
		assetObject.Mandate = make(map[identifiers.Identifier]map[common.TokenID]*common.AmountWithExpiry)
		// Create a new mandate if it doesn't exist
		assetObject.Mandate[beneficiary] = make(map[common.TokenID]*common.AmountWithExpiry)
		assetObject.Mandate[beneficiary][tokenID] = &common.AmountWithExpiry{
			Amount:    amount,
			ExpiresAt: expiresAt,
		}
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// SubMandateBalance decrements the mandate balance of the specified asset by the given amount
// for the specified id.
func (object *Object) SubMandateBalance(
	assetID identifiers.AssetID, tokenID common.TokenID, operatorID identifiers.Identifier, amount *big.Int,
) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	entries, ok := assetObject.Mandate[operatorID]
	if !ok {
		return common.ErrMandateNotFound
	}

	mandate, ok := entries[tokenID]
	if !ok {
		return common.ErrMandateNotFound
	}

	// Decrement the mandate balance by the given amount
	mandate.Amount.Sub(mandate.Amount, amount)

	// If the mandate amount is zero, remove the mandate for the specified id.
	if mandate.Amount.Cmp(big.NewInt(0)) == 0 {
		delete(assetObject.Mandate[operatorID], tokenID)
	}

	if len(assetObject.Mandate[operatorID]) == 0 {
		delete(assetObject.Mandate, operatorID)
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// ConsumeMandate updates the benefactor's mandate entry and asset balance
func (object *Object) ConsumeMandate(assetID identifiers.AssetID, tokenID common.TokenID,
	operatorID identifiers.Identifier, amount *big.Int,
) (*MetaData, error) {
	// Deduct the mandate amount from the operator's mandate balance
	if err := object.SubMandateBalance(assetID, tokenID, operatorID, amount); err != nil {
		return nil, err
	}

	// Deduct the transfer amount from the sender's asset balance
	return object.SubBalance(assetID, tokenID, amount)
}

// DeleteMandate revokes a granted spending authorization from a specified id for the given asset id.
func (object *Object) DeleteMandate(
	assetID identifiers.AssetID, tokenID common.TokenID,
	beneficiary identifiers.Identifier,
) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	delete(assetObject.Mandate[beneficiary], tokenID)

	if len(assetObject.Mandate[beneficiary]) == 0 {
		delete(assetObject.Mandate, beneficiary)
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// Mandates retrieves and returns all the asset mandates with their corresponding asset IDs, ids, and amounts.
func (object *Object) Mandates() ([]common.AssetMandateOrLockup, error) {
	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	it := assetTree.NewIterator()
	mandates := make([]common.AssetMandateOrLockup, 0)

	for it.Next() {
		if it.Leaf() {
			key, err := assetTree.GetPreImageKey(common.BytesToHash(it.LeafKey()))
			if err != nil {
				return nil, err
			}

			assetID := identifiers.AssetID(key)

			assetObject, err := object.getAssetObject(assetID, false)
			if err != nil {
				return nil, err
			}

			for id, mandate := range assetObject.Mandate {
				mandates = append(mandates, common.AssetMandateOrLockup{
					AssetID: assetID,
					ID:      id,
					Amount:  mandate,
				})
			}
		}
	}

	return mandates, nil
}

// GetMandate retrieves the mandate amount for the given id and asset id.
func (object *Object) GetMandate(assetID identifiers.AssetID,
	tokenID common.TokenID, beneficiary identifiers.Identifier,
) (*common.AmountWithExpiry, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	mandates, ok := assetObject.Mandate[beneficiary]
	if !ok {
		return nil, common.ErrMandateNotFound
	}

	if mandate, ok := mandates[tokenID]; ok {
		return mandate, nil
	}

	return nil, common.ErrMandateNotFound
}

func (object *Object) SetAssetMetadata(assetID identifiers.AssetID, isStatic bool, key string, val []byte) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	defer func() {
		object.updateAssetTree(assetID, assetObject)
	}()

	if isStatic {
		return assetObject.updateStaticMetadata(key, val)
	}

	assetObject.updateDynamicMetadata(key, val)

	return nil
}

func (object *Object) GetAssetMetadata(assetID identifiers.AssetID, isStatic bool, key string) ([]byte, error) {
	properties, err := object.GetProperties(assetID)
	if err != nil {
		return nil, err
	}

	if isStatic {
		val, ok := properties.StaticMetaData[key]
		if !ok {
			return nil, common.ErrKeyNotFound
		}

		return val, nil
	}

	val, ok := properties.DynamicMetaData[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

func (object *Object) SetTokenMetadata(
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	isStatic bool,
	key string, val []byte,
) error {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return common.ErrAssetNotFound
	}

	if !assetObject.hasTokenID(tokenID) {
		return common.ErrTokenNotFound
	}

	defer func() {
		object.updateAssetTree(assetID, assetObject)
	}()

	_, ok := assetObject.TokenMetaData[tokenID]
	if !ok {
		assetObject.TokenMetaData[tokenID] = &MetaData{
			static:  make(map[string][]byte),
			dynamic: make(map[string][]byte),
		}
	}

	if isStatic {
		return assetObject.updateStaticTokenMetaData(tokenID, key, val)
	}

	assetObject.updateDynamicTokenMetaData(tokenID, key, val)

	return nil
}

func (object *Object) GetTokenMetadata(
	assetID identifiers.AssetID,
	tokenID common.TokenID,
	isStatic bool,
	key string,
) ([]byte, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	metadata, ok := assetObject.TokenMetaData[tokenID]
	if !ok {
		return nil, common.ErrTokenNotFound
	}

	if isStatic {
		val, ok := metadata.static[key]
		if !ok {
			return nil, common.ErrKeyNotFound
		}

		return val, nil
	}

	val, ok := metadata.dynamic[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

// GetProperties retrieves the current properties of the specified asset,
// such as its symbol and supply details.
func (object *Object) GetProperties(assetID identifiers.AssetID) (*common.AssetDescriptor, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	return assetObject.Properties, nil
}

// Copy creates and returns a new object that replicates the state and all associated data of the original state object.
func (object *Object) Copy() *Object {
	sObj := NewStateObject(object.id, object.cache, object.treeCache, object.db,
		object.data, object.metrics, object.isGenesis)

	sObj.dirtyEntries = object.dirtyEntries.Copy()

	if len(object.keys) > 0 {
		sObj.keys = object.keys.Copy()
	}

	if object.assetTreeTxn != nil {
		sObj.assetTreeTxn = object.assetTreeTxn.CommitOnly().Txn()
	}

	if object.assetTree != nil {
		sObj.assetTree = object.assetTree.Copy()
	}

	if object.deeds != nil {
		sObj.deeds = object.deeds.Copy()
	}

	for logicID, sTree := range object.storageTreeTxns {
		if sTree != nil {
			sObj.storageTreeTxns[logicID] = sTree.CommitOnly().Txn()
		}
	}

	if object.logicTreeTxn != nil {
		sObj.logicTreeTxn = object.logicTreeTxn.CommitOnly().Txn()
	}

	if object.logicTree != nil {
		sObj.logicTree = object.logicTree.Copy()
	}

	for id, sTree := range object.storageTrees {
		sObj.storageTrees[id] = sTree.Copy()
	}

	if object.metaStorageTree != nil {
		sObj.metaStorageTree = object.metaStorageTree.Copy()
	}

	for key, value := range object.files {
		v := make([]byte, len(value))

		copy(v, value)

		sObj.files[key] = v
	}

	return sObj
}

// SetDirtyEntry marks a specific key-value pair as dirty in the object’s dirty entries.
func (object *Object) SetDirtyEntry(key string, value []byte) {
	object.dirtyEntries[key] = value
}

// GetDirtyEntry retrieves the value associated with a given key from the dirty entries.
// Returns an error if the key is not found.
func (object *Object) GetDirtyEntry(key string) ([]byte, error) {
	val, ok := object.dirtyEntries[key]
	if !ok {
		return nil, common.ErrKeyNotFound
	}

	return val, nil
}

// Commit finalizes all changes to the object by committing the deeds, assets, logics, and
// storage trees to the database.
func (object *Object) Commit() (common.Hash, error) {
	if _, err := object.commitAccountKeys(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit account keys")
	}

	if _, err := object.commitDeeds(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit deeds ")
	}

	if _, err := object.commitAssets(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit asset tree")
	}

	if _, err := object.commitLogics(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit logic tree")
	}

	if _, err := object.commitStorage(); err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit storage tree")
	}

	_, err := object.commitContextObject()
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit context object")
	}

	accCid, err := object.commitAccount()
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit account")
	}

	return accCid, nil
}

func (object *Object) commitAccountKeys() (common.Hash, error) {
	if len(object.keys) == 0 {
		return common.NilHash, nil
	}

	data, err := object.keys.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)

	object.SetDirtyEntry(
		common.BytesToHex(storage.KeyObjectKey(object.id, hash)),
		data,
	)

	object.data.KeysHash = hash

	return hash, nil
}

// commitDeeds stores the current deeds to the dirty entries.
func (object *Object) commitDeeds() (common.Hash, error) {
	if object.deeds == nil || len(object.deeds.Entries) == 0 {
		return common.NilHash, nil
	}

	data, err := object.deeds.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)

	object.SetDirtyEntry(
		common.BytesToHex(storage.DeedsKey(object.id, hash)),
		data,
	)

	object.data.AssetDeeds = hash

	return hash, nil
}

// commitAccount stores the current account data to the dirty entries and returns its hash.
func (object *Object) commitAccount() (common.Hash, error) {
	data, err := object.data.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(data)
	key := common.BytesToHex(storage.AccountKey(object.id, hash))

	object.SetDirtyEntry(key, data)

	return hash, nil
}

// commitContextObject serializes and stores the Context object to the dirty entries.
func (object *Object) commitContextObject() (common.Hash, error) {
	// Add type checks here
	if object.metaContext == nil {
		return object.ContextHash(), nil
	}

	rawData, err := object.metaContext.Bytes()
	if err != nil {
		return common.NilHash, err
	}

	hash := common.GetHash(rawData)
	key := common.BytesToHex(storage.ContextObjectKey(object.id, hash))

	object.SetDirtyEntry(key, rawData)

	// TODO:journal this
	object.cache.Add(hash, object.metaContext)
	object.data.ContextHash = hash

	return hash, nil
}

// commitActiveStorageTrees commits all active storage trees and updates the meta storage tree with their root hashes.
func (object *Object) commitActiveStorageTrees() error {
	for logicID, txn := range object.storageTreeTxns {
		sTree, err := object.GetStorageTree(logicID)
		if err != nil {
			return err
		}

		txn.Root().Walk(func(k []byte, v interface{}) bool {
			if err = sTree.Set(k, v.([]byte)); err != nil { //nolint
				return true
			}

			return false
		})

		if err = sTree.Commit(); err != nil {
			return errors.Wrap(err, "failed to commit storage tree")
		}

		rootHash, err := sTree.RootHash()
		if err != nil {
			return err
		}

		// Add the updated logic-id <=> storage-root in master storage merkleTree
		if err = object.metaStorageTree.Set(logicID.Bytes(), rootHash.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

// commitMetaStorageTree commits the meta storage tree transactions.
func (object *Object) commitMetaStorageTree() (common.Hash, error) {
	if object.metaStorageTree == nil || !object.metaStorageTree.IsDirty() {
		return object.data.StorageRoot, nil
	}

	if err := object.metaStorageTree.Commit(); err != nil {
		return common.NilHash, err
	}

	rootHash, err := object.metaStorageTree.RootHash()
	if err != nil {
		return common.NilHash, err
	}

	object.data.StorageRoot = rootHash

	return rootHash, nil
}

// commitStorage commits active storage tree and the meta storage tree transactions.
func (object *Object) commitStorage() (common.Hash, error) {
	err := object.commitActiveStorageTrees()
	if err != nil {
		return common.NilHash, err
	}

	return object.commitMetaStorageTree()
}

// commitAssets commits the asset tree transactions.
func (object *Object) commitAssets() (common.Hash, error) {
	if object.assetTreeTxn == nil {
		return object.data.AssetRoot, nil
	}

	assetTree, err := object.getAssetTree()
	if err != nil {
		return common.NilHash, err
	}

	objects := make(map[identifiers.AssetID]*AssetObject)

	object.assetTreeTxn.Root().Walk(func(k []byte, v interface{}) bool {
		obj, _ := v.(*AssetObject)
		assetID := identifiers.AssetID(k)
		objects[assetID] = obj

		return false
	})

	for assetID, obj := range objects {
		rawData, err := obj.Bytes()
		if err != nil {
			return common.NilHash, err
		}

		err = assetTree.Set(assetID.Bytes(), rawData)
		if err != nil {
			return common.NilHash, err
		}
	}

	err = assetTree.Commit()
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit asset tree")
	}

	object.data.AssetRoot, err = assetTree.RootHash()
	if err != nil {
		return common.NilHash, err
	}

	return object.data.AssetRoot, nil
}

// commitLogics commits the logic tree transactions
func (object *Object) commitLogics() (common.Hash, error) {
	if object.logicTreeTxn == nil {
		return object.data.LogicRoot, nil
	}

	logicTree, err := object.getLogicTree()
	if err != nil {
		return common.NilHash, err
	}

	objects := make([]*LogicObject, 0)

	object.logicTreeTxn.Root().Walk(func(k []byte, v interface{}) bool {
		obj, _ := v.(*LogicObject)
		objects = append(objects, obj)

		return false
	})

	for _, obj := range objects {
		rawBytes, err := obj.Bytes()
		if err != nil {
			return common.NilHash, err
		}

		if err = logicTree.Set(obj.ID.Bytes(), rawBytes); err != nil {
			return common.NilHash, err
		}
	}

	err = logicTree.Commit()
	if err != nil {
		return common.NilHash, errors.Wrap(err, "failed to commit logic tree")
	}

	object.data.LogicRoot, err = object.logicTree.RootHash()
	if err != nil {
		return common.NilHash, err
	}

	return object.data.LogicRoot, nil
}

// flush will write all dirty entries to the database
func (object *Object) flush() error {
	if err := object.flushAssetTree(); err != nil {
		return errors.Wrap(err, "failed to flush asset tree")
	}

	if err := object.flushLogicTree(); err != nil {
		return errors.Wrap(err, "failed to flush logic tree")
	}

	if err := object.flushStorageTrees(); err != nil {
		return errors.Wrap(err, "failed to flush active storage trees")
	}

	for k, v := range object.GetDirtyStorage() {
		if err := object.db.CreateEntry(common.FromHex(k), v); err != nil {
			return errors.Wrap(err, "failed to write dirty entries")
		}
	}

	return nil
}

// Flushes the asset tree if it exists.
func (object *Object) flushAssetTree() error {
	if object.assetTree == nil {
		return nil
	}

	return object.assetTree.Flush()
}

// Flushes the logic tree if it exists.
func (object *Object) flushLogicTree() error {
	if object.logicTree == nil {
		return nil
	}

	return object.logicTree.Flush()
}

// Flushes all storage trees, including the master storage tree.
func (object *Object) flushStorageTrees() error {
	if object.metaStorageTree == nil {
		return nil
	}
	// flush active storage trees
	for _, storageTree := range object.storageTrees {
		if err := storageTree.Flush(); err != nil {
			return errors.Wrap(err, "failed to commit modified storage tree entries to store")
		}
	}
	// flush master storage trees
	return object.metaStorageTree.Flush()
}

// CreateStorageTreeForLogic creates a storage tree for the given logic ID.
func (object *Object) CreateStorageTreeForLogic(logicID identifiers.Identifier) error {
	_, err := object.createStorageTreeForLogic(logicID)

	return err
}

// CreateAsset creates an asset and returns its asset ID.
func (object *Object) CreateAsset(
	id identifiers.Identifier,
	descriptor *common.AssetDescriptor,
) error {
	assetObject := NewAssetObject(descriptor)

	if err := object.InsertNewAssetObject(descriptor.AssetID, assetObject); err != nil {
		return err
	}

	return nil
}

// MintAsset increases the supply of the specified asset by the given amount and returns the circulating supply.
func (object *Object) MintAsset(assetID identifiers.AssetID, amount *big.Int) (*big.Int, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	assetObject.Properties.CirculatingSupply.Add(assetObject.Properties.CirculatingSupply, amount)

	return assetObject.Properties.CirculatingSupply, nil
}

// BurnAsset decreases the supply of the specified asset by the given amount and returns the circulating supply.
func (object *Object) BurnAsset(assetID identifiers.AssetID, amount *big.Int) (*big.Int, error) {
	assetObject, err := object.getAssetObject(assetID, true)
	if err != nil {
		return nil, common.ErrAssetNotFound
	}

	// assetObject.Properties.MaxSupply.Sub(assetObject.Properties.MaxSupply, amount)
	assetObject.Properties.CirculatingSupply.Sub(assetObject.Properties.CirculatingSupply, amount)

	object.updateAssetTree(assetID, assetObject)

	return assetObject.Properties.CirculatingSupply, nil
}

// CreateLogic creates a new logic object and returns its logic ID.
func (object *Object) CreateLogic(logicID identifiers.Identifier, descriptor engineio.LogicDescriptor) error {
	// Generate the key for the LogicManifest from its hash
	key := common.BytesToHex(storage.LogicManifestKey(object.Identifier(), descriptor.ManifestHash))
	// Write the manifest into the dirty entries
	object.SetDirtyEntry(key, descriptor.ManifestData)

	// Create a new LogicObject from the LogicDescriptor
	logicObject := NewLogicObject(object.Identifier(), descriptor)
	// Insert the LogicObject into the state object
	if err := object.InsertNewLogicObject(logicObject.ID, logicObject); err != nil {
		return errors.Wrap(err, "could not insert logic object into state object")
	}

	// Initialise the logic for itself
	if err := object.InitLogicStorage(logicObject.LogicID()); err != nil {
		return err
	}

	return nil
}

// InitLogicStorage initializes the storage for a given logic ID.
func (object *Object) InitLogicStorage(logicID identifiers.Identifier) error {
	// Initialize a storage tree for the LogicID on the state object
	if _, err := object.createStorageTreeForLogic(logicID); err != nil {
		return err
	}

	object.storageTreeTxns[logicID] = iradix.New().Txn()

	return nil
}

// AddAccountGenesisInfo adds genesis information for an account.
func (object *Object) AddAccountGenesisInfo(id identifiers.Identifier, ixHash common.Hash) error {
	accInfo := common.AccountGenesisInfo{
		IxHash: ixHash,
	}

	rawData, err := accInfo.Bytes()
	if err != nil {
		return err
	}

	return object.SetStorageEntry(common.SargaLogicID.AsIdentifier(), id.Bytes(), rawData)
}

func (object *Object) AddManifestForAsset(standard common.AssetStandard, rawManifest []byte) error {
	return object.SetStorageEntry(common.SargaLogicID.AsIdentifier(), AssetLogicKey(standard), rawManifest)
}

func (object *Object) GetManifestForAsset(standard common.AssetStandard) ([]byte, error) {
	return object.GetStorageEntry(common.SargaLogicID.AsIdentifier(), AssetLogicKey(standard))
}

func (object *Object) IsAccountRegistered(id identifiers.Identifier) (bool, error) {
	_, err := object.GetStorageEntry(common.SargaLogicID.AsIdentifier(), id.Bytes())
	if errors.Is(err, common.ErrKeyNotFound) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, nil
}

// CreateContext creates a context object with given nodes and returns its hash.
func (object *Object) CreateContext(consensusNodes []identifiers.KramaID) error {
	if len(consensusNodes) < MinimumContextSize {
		return errors.New("liveliness size not met")
	}

	metaContextObject := new(MetaContextObject)

	consensusNodesHash, err := common.PoloHash(consensusNodes)
	if err != nil {
		return errors.Wrap(err, "failed to polorize context object")
	}

	metaContextObject.ConsensusNodesHash = consensusNodesHash
	metaContextObject.ConsensusNodes = consensusNodes
	metaContextObject.PreviousHash = common.NilHash

	object.metaContext = metaContextObject

	return nil
}

// UpdateContext updates the context with new nodes and returns the new context hash.
func (object *Object) UpdateContext(consensusNodes []identifiers.KramaID) error {
	if len(consensusNodes) == 0 || object.Identifier().IsParticipantVariant() {
		return nil
	}

	var (
		err                error
		consensusNodesHash common.Hash
	)

	metaObj, err := object.getMetaContextObjectCopy()
	if err != nil {
		return err
	}

	consensusNodesHash, err = common.PoloHash(consensusNodes)
	if err != nil {
		return errors.Wrap(err, "failed to generate consensus nodes hash")
	}

	// TODO:Sort based on the stake of the nodes

	// Set the previous Hash
	metaObj.PreviousHash = object.ContextHash()
	metaObj.ConsensusNodes = consensusNodes
	metaObj.ConsensusNodesHash = consensusNodesHash

	object.metaContext = metaObj

	return nil
}

// ContextHash returns the current context hash.
func (object *Object) ContextHash() common.Hash {
	return object.data.ContextHash
}

// getMetaContextObjectCopy retrieves a copy of the meta context object from cache or database.
func (object *Object) getMetaContextObjectCopy() (*MetaContextObject, error) {
	data, isAvailable := object.cache.Get(object.ContextHash())
	if isAvailable {
		metaContextObject, ok := data.(*MetaContextObject)
		if !ok {
			return nil, common.ErrInterfaceConversion
		}

		return metaContextObject.Copy(), nil
	}

	rawData, err := object.db.GetContext(object.id, object.ContextHash())
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch meta context object")
	}

	obj := new(MetaContextObject)
	if err := obj.FromBytes(rawData); err != nil {
		return nil, err
	}

	object.cache.Add(object.ContextHash(), obj)

	return obj.Copy(), nil
}

// loads the asset deeds from the database or initializes a new one.
func (object *Object) loadDeeds() error {
	if object.data.AssetDeeds.IsNil() {
		object.deeds = &Deeds{
			Entries: make(map[identifiers.Identifier]struct{}),
		}

		return nil
	}

	data, err := object.db.GetDeeds(object.id, object.data.AssetDeeds)
	if err != nil {
		return err
	}

	object.deeds = new(Deeds)

	if err = object.deeds.FromBytes(data); err != nil {
		return err
	}

	return nil
}

// HasStorageTree checks if a storage tree for the given logic ID exists.
func (object *Object) HasStorageTree(logicID identifiers.Identifier) (bool, error) {
	if _, ok := object.storageTrees[logicID]; ok {
		return true, nil
	}

	metaStorageTree, err := object.getMetaStorageTree()
	if err != nil {
		return false, err
	}

	if _, err := metaStorageTree.Get(logicID.Bytes()); err != nil {
		return false, nil
	}

	return true, nil
}

// GetStorageTree retrieves and returns the Merkle tree based on the specified logic ID.
// If the tree is not cached, it loads it from the meta storage tree and initializes it.
// Returns an error if loading or initialization fails.
func (object *Object) GetStorageTree(logicID identifiers.Identifier) (tree.MerkleTree, error) {
	storageTree, ok := object.storageTrees[logicID]
	if ok {
		return storageTree, nil
	}

	metaStorageTree, err := object.getMetaStorageTree()
	if err != nil {
		return nil, err
	}

	root, err := metaStorageTree.Get(logicID.Bytes())
	if err != nil {
		return nil, common.ErrLogicStorageTreeNotFound
	}

	storageTree, err = tree.NewKramaHashTree(
		object.id,
		common.BytesToHash(root),
		object.db, blake256.New(),
		storage.Storage,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic storage tree")
	}

	object.storageTrees[logicID] = storageTree

	return storageTree, nil
}

// SetStorageEntry inserts a key-value pair in the storage tree for the given logic ID.
// Returns an error if any issues arise during the process.
func (object *Object) SetStorageEntry(logicID identifiers.Identifier, key, value []byte) error {
	_, ok := object.storageTreeTxns[logicID]
	if !ok {
		if _, err := object.GetStorageTree(logicID); err != nil {
			if !errors.Is(err, common.ErrLogicStorageTreeNotFound) {
				return err
			}

			if _, err = object.createStorageTreeForLogic(logicID); err != nil {
				return err
			}
		}

		object.storageTreeTxns[logicID] = iradix.New().Txn()
	}

	// If the value has zero length, we treat it as a
	// delete operation instead of a write operation
	if len(value) == 0 {
		object.storageTreeTxns[logicID].Delete(key)

		return nil
	}

	object.storageTreeTxns[logicID].Insert(key, value)

	return nil
}

// GetStorageEntry retrieves the value associated a specific key from the storage tree for the given logic ID.
func (object *Object) GetStorageEntry(logicID identifiers.Identifier, key []byte) (value []byte, err error) {
	activeStorageTree, ok := object.storageTreeTxns[logicID]
	if ok {
		v, ok := activeStorageTree.Get(key)
		if ok {
			return v.([]byte), nil //nolint
		}
	}

	merkleTree, err := object.GetStorageTree(logicID)
	if err != nil {
		return nil, err
	}

	return merkleTree.Get(key)
}

// GetDirtyStorage returns the collection of storage entries that have been modified
// but not yet committed to the database.
func (object *Object) GetDirtyStorage() Storage {
	return object.dirtyEntries
}

// getMetaStorageTree retrieves the meta storage tree. If it's not initialized, it creates a new instance.
// Returns the meta storage tree or an error if initialization fails.
func (object *Object) getMetaStorageTree() (tree.MerkleTree, error) {
	if object.metaStorageTree != nil {
		return object.metaStorageTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(
		object.id,
		object.data.StorageRoot,
		object.db,
		blake256.New(),
		storage.Storage,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate storage tree")
	}

	object.metaStorageTree = merkleTree

	return object.metaStorageTree, nil
}

// createStorageTreeForLogic initializes a new Merkle tree for the specified logic ID and updates the meta storage tree.
// Returns the Merkle tree or an error if the creation or update fails.
func (object *Object) createStorageTreeForLogic(logicID identifiers.Identifier) (tree.MerkleTree, error) {
	if _, err := object.getMetaStorageTree(); err != nil {
		return nil, err
	}

	newStorageTree, err := tree.NewKramaHashTree(
		object.id,
		common.NilHash,
		object.db,
		blake256.New(),
		storage.Storage,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, err
	}

	object.storageTrees[logicID] = newStorageTree

	return newStorageTree, object.metaStorageTree.Set(logicID.Bytes(), common.NilHash.Bytes())
}

// isAssetRegistered checks if the given asset ID is registered.
func (object *Object) isAssetRegistered(assetID identifiers.AssetID) error {
	_, err := object.getAssetObject(assetID, true)
	if err != nil {
		return err
	}

	return nil
}

// isAssetRegistered checks if the given logic ID is registered.
func (object *Object) isLogicRegistered(logicID identifiers.Identifier) error {
	_, err := object.getLogicObject(logicID)
	if err != nil {
		return err
	}

	return nil
}

// getAssetTree retrieves the Merkle tree used for managing assets. If the tree is not initialized,
// it creates a new instance. Returns the asset Merkle tree or an error if the initialization fails.
func (object *Object) getAssetTree() (tree.MerkleTree, error) {
	if object.assetTree != nil {
		return object.assetTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(
		object.id,
		object.data.AssetRoot,
		object.db,
		blake256.New(),
		storage.Asset,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate asset tree")
	}

	object.assetTree = merkleTree

	return object.assetTree, nil
}

// getLogicTree retrieves the Merkle tree used for managing logic. If the tree is not initialized,
// it creates a new instance. Returns the logic Merkle tree or an error if the initialization fails.
func (object *Object) getLogicTree() (tree.MerkleTree, error) {
	if object.logicTree != nil {
		return object.logicTree, nil
	}

	merkleTree, err := tree.NewKramaHashTree(
		object.id,
		object.data.LogicRoot,
		object.db,
		blake256.New(),
		storage.Logic,
		object.treeCache,
		object.metrics.TreeMetrics,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initiate logic tree")
	}

	object.logicTree = merkleTree

	return object.logicTree, nil
}

// getAssetObject retrieves the asset object for the specified asset ID.
func (object *Object) getAssetObject(assetID identifiers.AssetID, checkTxn bool) (*AssetObject, error) {
	if checkTxn && object.assetTreeTxn != nil {
		if v, ok := object.assetTreeTxn.Get(assetID.Bytes()); ok {
			if assetObject, ok := v.(*AssetObject); ok {
				return assetObject, nil
			}
		}
	}

	assetTree, err := object.getAssetTree()
	if err != nil {
		return nil, err
	}

	rawObject, err := assetTree.Get(assetID.Bytes())
	if err != nil {
		return nil, err
	}

	assetObject := new(AssetObject)
	if err = assetObject.FromBytes(rawObject); err != nil {
		return nil, err
	}

	return assetObject, nil
}

func (object *Object) AssetProperties(id identifiers.AssetID) (*common.AssetDescriptor, error) {
	assetObject, err := object.getAssetObject(id, false)
	if err != nil {
		return nil, err
	}

	return assetObject.Properties, nil
}

// getLogicObject retrieves the logic object for the specified logic ID.
func (object *Object) getLogicObject(logicID identifiers.Identifier) (*LogicObject, error) {
	if object.logicTreeTxn != nil {
		v, ok := object.logicTreeTxn.Get(logicID.Bytes())
		if ok {
			return v.(*LogicObject), nil //nolint
		}
	}

	logicTree, err := object.getLogicTree()
	if err != nil {
		return nil, err
	}

	rawObject, err := logicTree.Get(logicID.Bytes())
	if err != nil {
		return nil, err
	}

	logicObject := new(LogicObject)
	if err = logicObject.FromBytes(rawObject); err != nil {
		return nil, err
	}

	return logicObject, nil
}

// InsertNewAssetObject inserts the assetID and assetObject into the assetTree
// If the assetID is registered, this returns an error
func (object *Object) InsertNewAssetObject(assetID identifiers.AssetID, assetObject *AssetObject) error {
	if err := object.isAssetRegistered(assetID); err == nil {
		return common.ErrAssetAlreadyRegistered
	}

	object.updateAssetTree(assetID, assetObject)

	return nil
}

// InsertNewLogicObject inserts the logicID and logicObject into the logicsTree
// If the logicID is registered, this returns an error
func (object *Object) InsertNewLogicObject(logicID identifiers.Identifier, logicObject *LogicObject) error {
	if err := object.isLogicRegistered(logicID); err == nil {
		return errors.New("logic already registered")
	}

	object.updateLogicTree(logicID, logicObject)

	return nil
}

// FetchAssetObject returns the AssetObject associated with the given assetID,
// This returns an error if the assetID is not registered
func (object *Object) FetchAssetObject(assetID identifiers.AssetID, fromTxn bool) (*AssetObject, error) {
	return object.getAssetObject(assetID, fromTxn)
}

// FetchLogicObject returns the LogicObject associated with the given logicID,
// This returns an error if the logicID is not registered
func (object *Object) FetchLogicObject(logicID identifiers.Identifier) (*LogicObject, error) {
	return object.getLogicObject(logicID)
}

// FetchLogicStorageObject returns a LogicStorageObject
func (object *Object) FetchLogicStorageObject() *LogicStorageObject {
	return NewLogicStorageObject(object)
}

func (object *Object) HasSufficientFuel(amount *big.Int) (bool, error) {
	if amount.Sign() == -1 {
		return false, errors.New("invalid transfer amount")
	}

	// Fetch sender balance object
	balance, err := object.BalanceOf(common.KMOITokenAssetID, common.DefaultTokenID)
	if err != nil {
		return false, err
	}

	// Check if sender has sufficient balance
	if balance.Cmp(amount) == -1 {
		return false, nil
	}

	return true, nil
}

func (object *Object) DeductFuel(amount *big.Int) {
	// Remove amount from sender balance for asset
	_, _ = object.SubBalance(common.KMOITokenAssetID, common.DefaultTokenID, amount)
}

func (object *Object) ConsensusNodes() []identifiers.KramaID {
	if _, err := object.getMetaCtxObject(); err != nil {
		panic(err)
	}

	return object.metaContext.ConsensusNodes
}

func (object *Object) ConsensusNodesHash() common.Hash {
	if _, err := object.getMetaCtxObject(); err != nil {
		panic(err)
	}

	return object.metaContext.ConsensusNodesHash
}

func (object *Object) InheritAccount(payload *common.AccountInheritPayload, sender *Object) {
	_ = object.UpdateInheritedAccount(payload.TargetAccount)

	accountKeys, _ := sender.AccountKeys()
	object.UpdateKeys(accountKeys.CopyForInheritAccount())
}

func (object *Object) InheritedAccount() identifiers.Identifier {
	if _, err := object.getMetaCtxObject(); err != nil {
		panic(err)
	}

	return object.metaContext.InheritedAccount
}
