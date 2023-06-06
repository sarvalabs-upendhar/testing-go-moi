package dhruva

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/sarvalabs/moichain/common"
	"github.com/sarvalabs/moichain/dhruva/db/badger"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/moichain/common/tests"
	"github.com/sarvalabs/moichain/dhruva/db"
	"github.com/sarvalabs/moichain/types"
)

// mockDB is an in-memory key-value database used for testing purposes
type mockDB struct {
	dbStorage map[string][]byte
}

func (m *mockDB) GetLastActiveTimeStamp() uint64 {
	// TODO implement me
	panic("implement me")
}

func (m *mockDB) DropWithPrefix(prefix []byte) error {
	// TODO implement me
	panic("implement me")
}

func (m *mockDB) Snapshot(ctx context.Context, prefix []byte, sinceTS uint64, collector db.Collector) error {
	// TODO implement me
	panic("implement me")
}

type mockIterator struct {
	data      map[string][]byte
	keys      []string
	prefixKey string
}

type mockBatchWriter struct {
	db *mockDB
}

func (bw *mockBatchWriter) WriteBuffer(buf []byte) error {
	// TODO implement me
	panic("implement me")
}

func (bw *mockBatchWriter) Set(key []byte, value []byte) error {
	bw.db.dbStorage[string(key)] = value

	return nil
}

func (bw *mockBatchWriter) Flush() error {
	return nil
}

func NewMockDB(t *testing.T) *mockDB {
	t.Helper()

	return &mockDB{
		dbStorage: make(map[string][]byte),
	}
}

func NewTestPersistenceManager(t *testing.T) *PersistenceManager {
	t.Helper()

	ctx, ctxCancel := context.WithCancel(context.Background())

	return &PersistenceManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		config:    nil,
		logger:    hclog.Default(),
		db:        NewMockDB(t),
	}
}

func NewTestPersistenceManagerWithBadger(t *testing.T, badgerPath string) *PersistenceManager {
	t.Helper()

	ctx, ctxCancel := context.WithCancel(context.Background())

	bg, err := badger.NewBadgerDB(badgerPath)
	require.NoError(t, err)

	return &PersistenceManager{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		config: &common.DBConfig{
			DBFolderPath: badgerPath,
			MaxSnapSize:  1024 * 1024 * 1024,
		},
		logger: hclog.Default(),
		db:     bg,
	}
}

func (m *mockDB) NewBatchWriter() db.BatchWriter {
	return &mockBatchWriter{db: m}
}

func (m *mockDB) Insert(key []byte, value []byte) error {
	m.dbStorage[string(key)] = value

	return nil
}

func (m *mockDB) Update(key []byte, value []byte) error {
	if exists, _ := m.Has(key); exists {
		m.dbStorage[string(key)] = value

		return nil
	}

	return types.ErrKeyNotFound
}

func (m *mockDB) Delete(key []byte) error {
	if exists, _ := m.Has(key); exists {
		delete(m.dbStorage, string(key))

		return nil
	}

	return types.ErrKeyNotFound
}

func (m *mockDB) Get(key []byte) ([]byte, error) {
	if exists, _ := m.Has(key); exists {
		val := m.dbStorage[string(key)]

		return val, nil
	}

	return nil, types.ErrKeyNotFound
}

func (m *mockDB) Has(key []byte) (bool, error) {
	_, ok := m.dbStorage[string(key)]

	return ok, nil
}

func (m *mockDB) CleanUp() error {
	return nil
}

func (m *mockDB) Close() error {
	return nil
}

func (m *mockDB) NewIterator() (db.Iterator, error) {
	it := &mockIterator{
		data:      make(map[string][]byte, len(m.dbStorage)),
		keys:      make([]string, len(m.dbStorage)),
		prefixKey: "",
	}
	i := 0

	for k, v := range m.dbStorage {
		it.keys[i] = k
		it.data[k] = v
		i++
	}

	return it, nil
}

func (it *mockIterator) Close() {
}

// Seek move's forward till matching prefix key
func (it *mockIterator) Seek(key []byte) {
	it.prefixKey = string(key)

	for {
		if len(it.keys) == 0 {
			break
		}

		if strings.HasPrefix(it.keys[0], string(key)) {
			break
		}

		it.keys = it.keys[1:]
	}
}

// Next is used to move to matching prefix after first iteration onwards
func (it *mockIterator) Next() {
	it.keys = it.keys[1:]

	for {
		if len(it.keys) == 0 {
			break
		}

		if strings.HasPrefix(it.keys[0], it.prefixKey) {
			break
		}

		it.keys = it.keys[1:]
	}
}

func (it *mockIterator) ValidForPrefix(prefix []byte) bool {
	return len(it.keys) != 0
}

func (it *mockIterator) GetNext() (*types.DBEntry, error) {
	return &types.DBEntry{
		Key:   []byte(it.keys[0]),
		Value: it.data[it.keys[0]],
	}, nil
}

func insertTestAccMetaInfo(t *testing.T, pm *PersistenceManager) map[uint64]types.Accounts {
	t.Helper()

	insertedAccounts := make(map[uint64]types.Accounts, 0)

	accountCount := 10000
	for i := 0; i < accountCount; i++ {
		// test data
		AccMetaInfo := tests.GetRandomAccMetaInfo(t, 1)
		// insert test data in to db
		_, _, err := pm.UpdateAccMetaInfo(
			AccMetaInfo.Address,
			AccMetaInfo.Height,
			AccMetaInfo.TesseractHash,
			AccMetaInfo.Type,
			AccMetaInfo.LatticeExists,
			AccMetaInfo.StateExists,
		)
		require.NoError(t, err)

		// store the data we inserted into db with key as bucket number and  value as account meta info
		accID := new(big.Int).SetBytes(AccMetaInfo.Address.Bytes())
		bucketNo := accID.Mod(accID, new(big.Int).SetUint64(MaxBucketCount))

		insertedAccounts[bucketNo.Uint64()] = append(insertedAccounts[bucketNo.Uint64()], AccMetaInfo)
	}

	return insertedAccounts
}

// incrementBuckets takes 10000 random addresses and increment each one by incrementNumber
func incrementBuckets(t *testing.T, pm *PersistenceManager) map[uint64]uint64 {
	t.Helper()

	incrementBucketSizes := make(map[uint64]uint64, 0)

	incrementNumber := uint64(3)

	for i := 0; i < 10000; i++ {
		_, bucket := BucketKeyAndID(tests.RandomAddress(t))
		err := pm.incrementBucketCount(bucket, incrementNumber)

		require.NoError(t, err)

		incrementBucketSizes[bucket] += incrementNumber
	}

	return incrementBucketSizes
}

func insertTestEntries(t *testing.T, pm *PersistenceManager) (map[string]string, []string) {
	t.Helper()

	var prefixes []string

	insertedEntries := make(map[string]string)

	entryCount := 1000
	prefixLength := 10
	keyLength := 20
	valueLength := 30

	for i := 0; i < entryCount; i++ {
		prefix := tests.GetRandomUpperCaseString(t, prefixLength)

		prefixes = append(prefixes, prefix)
		// no of entries for each prefix
		for j := 0; j < 10; j++ {
			key := tests.GetRandomUpperCaseString(t, keyLength)
			prefixedKey := prefix + key
			prefixedKeyBytes := []byte(prefixedKey)

			val := tests.GetRandomUpperCaseString(t, valueLength)

			valBytes := []byte(val)

			err := pm.CreateEntry(prefixedKeyBytes, valBytes)
			require.NoError(t, err)

			insertedEntries[prefixedKey] = val
		}
	}

	return insertedEntries, prefixes
}

func getAddresses(t *testing.T, count int) []types.Address {
	t.Helper()

	var addresses []types.Address

	for i := 0; i < count; i++ {
		addresses = append(addresses, tests.RandomAddress(t))
	}

	return addresses
}

func getHashes(t *testing.T, count int) []types.Hash {
	t.Helper()

	var addresses []types.Hash

	for i := 0; i < count; i++ {
		addresses = append(addresses, tests.RandomHash(t))
	}

	return addresses
}

func insertAccMetaInfo(t *testing.T, pm *PersistenceManager, accMetaInfo types.AccountMetaInfo) {
	t.Helper()

	key, bucket := BucketKeyAndID(accMetaInfo.Address)

	rawData, err := accMetaInfo.Bytes()
	require.NoError(t, err)

	if err := pm.CreateEntry(key, rawData); err != nil {
		require.NoError(t, err)
	}

	if err := pm.incrementBucketCount(bucket, 1); err != nil {
		require.NoError(t, err)
	}
}

func getRandomPreImageEntries(t *testing.T, count int) map[types.Hash][]byte {
	t.Helper()

	testPreImageEntries := make(map[types.Hash][]byte, count)
	for i := 0; i < count; i++ {
		testPreImageEntries[tests.RandomHash(t)] = []byte{byte(i)}
	}

	return testPreImageEntries
}

func getRandomReceipts(t *testing.T, receiptHash types.Hash, count int) types.Receipts {
	t.Helper()

	receipts := make(types.Receipts, count)

	for i := 0; i < count; i++ {
		receipts[receiptHash] = &types.Receipt{
			IxHash: tests.RandomHash(t),
			IxType: 2,
		}
	}

	return receipts
}

func insertTSGridLookup(t *testing.T, pm *PersistenceManager, tsHash types.Hash, gridHash types.Hash) {
	t.Helper()

	err := pm.SetTSGridLookup(tsHash, gridHash)
	require.NoError(t, err)
}

func insertReceipts(t *testing.T, pm *PersistenceManager, receiptHash types.Hash, receipts types.Receipts) {
	t.Helper()

	rawData, err := receipts.Bytes()
	require.NoError(t, err)

	err = pm.SetReceipts(receiptHash, rawData)
	require.NoError(t, err)
}
