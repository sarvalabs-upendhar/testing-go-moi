package storage

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/config"
	"github.com/sarvalabs/go-moi/storage/db/badger"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common/tests"
	"github.com/sarvalabs/go-moi/storage/db"
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

	return &PersistenceManager{
		config: nil,
		logger: hclog.Default(),
		db:     NewMockDB(t),
	}
}

func NewTestPersistenceManagerWithBadger(t *testing.T, badgerPath string) *PersistenceManager {
	t.Helper()

	bg, err := badger.NewBadgerDB(badgerPath, db.NilMetrics(), hclog.NewNullLogger())
	require.NoError(t, err)

	return &PersistenceManager{
		config: &config.DBConfig{
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

	return common.ErrKeyNotFound
}

func (m *mockDB) Delete(key []byte) error {
	if exists, _ := m.Has(key); exists {
		delete(m.dbStorage, string(key))

		return nil
	}

	return common.ErrKeyNotFound
}

func (m *mockDB) Get(key []byte) ([]byte, error) {
	if exists, _ := m.Has(key); exists {
		val := m.dbStorage[string(key)]

		return val, nil
	}

	return nil, common.ErrKeyNotFound
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

func (it *mockIterator) GetNext() (*common.DBEntry, error) {
	return &common.DBEntry{
		Key:   []byte(it.keys[0]),
		Value: it.data[it.keys[0]],
	}, nil
}

func insertTestAccMetaInfo(t *testing.T, pm *PersistenceManager) map[uint64]common.Accounts {
	t.Helper()

	insertedAccounts := make(map[uint64]common.Accounts)

	accountCount := 5000
	for i := 0; i < accountCount; i++ {
		// test data
		AccMetaInfo := tests.GetRandomAccMetaInfo(t, 1)
		// insert test data in to db
		_, _, err := pm.UpdateAccMetaInfo(
			AccMetaInfo.Address,
			AccMetaInfo.Height,
			AccMetaInfo.TesseractHash,
			AccMetaInfo.StateHash,
			AccMetaInfo.ContextHash,
			AccMetaInfo.Type,
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

	incrementBucketSizes := make(map[uint64]uint64)

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

	entryCount := 100
	prefixLength := 10
	keyLength := 20
	valueLength := 30

	batchWriter := pm.NewBatchWriter()

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

			err := batchWriter.Set(prefixedKeyBytes, valBytes)
			require.NoError(t, err)

			insertedEntries[prefixedKey] = val
		}
	}

	err := batchWriter.Flush()
	require.NoError(t, err)

	return insertedEntries, prefixes
}

func insertAccMetaInfo(t *testing.T, pm *PersistenceManager, accMetaInfo common.AccountMetaInfo) {
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

func getRandomPreImageEntries(t *testing.T, count int) map[common.Hash][]byte {
	t.Helper()

	testPreImageEntries := make(map[common.Hash][]byte, count)
	for i := 0; i < count; i++ {
		testPreImageEntries[tests.RandomHash(t)] = []byte{byte(i)}
	}

	return testPreImageEntries
}

func getRandomReceipts(t *testing.T, receiptHash common.Hash, count int) common.Receipts {
	t.Helper()

	receipts := make(common.Receipts, count)

	for i := 0; i < count; i++ {
		receipts[receiptHash] = &common.Receipt{
			IxHash: tests.RandomHash(t),
			IxType: 2,
		}
	}

	return receipts
}

func insertReceipts(t *testing.T, pm *PersistenceManager, tsHash common.Hash, receipts common.Receipts) {
	t.Helper()

	rawData, err := receipts.Bytes()
	require.NoError(t, err)

	err = pm.SetReceipts(tsHash, rawData)
	require.NoError(t, err)
}

func insertTesseracts(t *testing.T, pm *PersistenceManager, tesseracts ...*common.Tesseract) {
	t.Helper()

	for i := 0; i < len(tesseracts); i++ {
		rawBytes, err := tesseracts[i].Canonical().Bytes()
		require.NoError(t, err)

		err = pm.SetTesseract(tesseracts[i].Hash(), rawBytes)
		require.NoError(t, err)
	}
}

func insertIxns(t *testing.T, pm *PersistenceManager, tesseracts ...*common.Tesseract) {
	t.Helper()

	for i := 0; i < len(tesseracts); i++ {
		rawBytes, err := tesseracts[i].Interactions().Bytes()
		require.NoError(t, err)

		err = pm.SetInteractions(tesseracts[i].Hash(), rawBytes)
		require.NoError(t, err)
	}
}

func insertReceiptsInDB(t *testing.T, pm *PersistenceManager, tesseracts ...*common.Tesseract) {
	t.Helper()

	for i := 0; i < len(tesseracts); i++ {
		rawBytes, err := tesseracts[i].Receipts().Bytes()
		require.NoError(t, err)

		err = pm.SetReceipts(tesseracts[i].Hash(), rawBytes)
		require.NoError(t, err)
	}
}

func validateTesseract(t *testing.T, ts *common.Tesseract, expectedTS *common.Tesseract, withInteractions bool) {
	t.Helper()

	if !withInteractions || ts.ClusterID() == common.GenesisIdentifier { // check if tesseracts matches
		require.Equal(t, expectedTS.Canonical(), ts.Canonical())
		require.Equal(t, 0, len(ts.Interactions())) // make sure returned tesseract has zero ixns
		require.Equal(t, 0, len(ts.Receipts()))

		return
	}

	ts.Hash() // calculate hash to fill hash field in tesseract
	require.Equal(t, expectedTS, ts)
}
