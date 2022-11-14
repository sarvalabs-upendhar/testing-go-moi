package dhruva

import (
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"gitlab.com/sarvalabs/moichain/common/tests"
	"gitlab.com/sarvalabs/moichain/dhruva/db"
	"gitlab.com/sarvalabs/moichain/types"
	"gitlab.com/sarvalabs/polo/go-polo"
	"golang.org/x/net/context"
)

// mockDB is an in-memory key-value database used for testing purposes
type mockDB struct {
	dbStorage map[string][]byte
}

type mockIterator struct {
	data      map[string][]byte
	keys      []string
	prefixKey string
}

type mockBatchWriter struct {
	db *mockDB
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
		Config:    nil,
		logger:    hclog.Default(),
		db:        NewMockDB(t),
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

func getAccMetaInfo(t *testing.T, height int64) *types.AccountMetaInfo {
	t.Helper()

	return &types.AccountMetaInfo{
		Address:       tests.RandomAddress(t),
		Type:          types.AccType(1),
		Height:        big.NewInt(height),
		TesseractHash: tests.RandomHash(t),
		LatticeExists: true,
		StateExists:   true,
	}
}

func insertTestAccMetaInfo(t *testing.T, pm *PersistenceManager) map[int64]types.Accounts {
	t.Helper()

	insertedAccounts := make(map[int64]types.Accounts, 0)

	accountCount := 10000
	for i := 0; i < accountCount; i++ {
		// test data
		AccMetaInfo := getAccMetaInfo(t, 1)
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
		bucketNo := accID.Mod(accID, big.NewInt(BucketCount))
		insertedAccounts[bucketNo.Int64()] = append(insertedAccounts[bucketNo.Int64()], AccMetaInfo)
	}

	return insertedAccounts
}

// incrementBuckets takes 10000 random addresses and increment each one by incrementNumber
func incrementBuckets(t *testing.T, pm *PersistenceManager) map[int32]int64 {
	t.Helper()

	incrementBucketSizes := make(map[int32]int64, 0)

	incrementNumber := int64(3)

	for i := 0; i < 10000; i++ {
		_, bucket := BucketIDFromAddress(tests.RandomAddress(t).Bytes())
		err := pm.incrementBucketCount(bucket.getIDBytes(), incrementNumber)

		require.NoError(t, err)

		incrementBucketSizes[int32(new(big.Int).SetBytes(bucket.getIDBytes()).Int64())] += incrementNumber
	}

	return incrementBucketSizes
}

func checkIfAccountExists(account *types.AccountMetaInfo, accounts types.Accounts) bool {
	for _, acc := range accounts {
		if reflect.DeepEqual(account, acc) {
			return true
		}
	}

	return false
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
		prefix, err := tests.GetRandomUpperCaseString(t, prefixLength)
		require.NoError(t, err)

		prefixes = append(prefixes, prefix)
		// no of entries for each prefix
		for j := 0; j < 10; j++ {
			key, err := tests.GetRandomUpperCaseString(t, keyLength)

			require.NoError(t, err)

			prefixedKey := prefix + key
			prefixedKeyBytes := []byte(prefixedKey)

			val, err := tests.GetRandomUpperCaseString(t, valueLength)

			require.NoError(t, err)

			valBytes := []byte(val)

			err = pm.CreateEntry(prefixedKeyBytes, valBytes)
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

	key, bucket := BucketIDFromAddress(accMetaInfo.Address.Bytes())

	if err := pm.CreateEntry(key, polo.Polorize(accMetaInfo)); err != nil {
		require.NoError(t, err)
	}

	if err := pm.incrementBucketCount(bucket.getIDBytes(), 1); err != nil {
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
