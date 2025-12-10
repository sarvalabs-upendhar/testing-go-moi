package state

import (
	"reflect"
	"testing"
	"time"

	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/stretchr/testify/require"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/common/tests"
)

func createTestSystemObject(t *testing.T, params *createStateObjectParams) *SystemObject {
	t.Helper()

	obj := createTestStateObject(t, params)

	so := NewSystemObject(obj)

	return so
}

func TestSystemObject_Copy(t *testing.T) {
	testcases := []struct {
		name     string
		setup    func(t *testing.T) *SystemObject
		validate func(t *testing.T, original, copied *SystemObject)
	}{
		{
			name: "should copy simple fields correctly",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()
				so := createTestSystemObject(t, nil)
				so.totalValidators = 10
				so.genesisTime = time.Unix(1234567890, 0)

				return so
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				require.Equal(t, original.totalValidators, copied.totalValidators)
				require.Equal(t, original.genesisTime, copied.genesisTime)
			},
		},
		{
			name: "should share validators slice (copy-on-write)",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()
				so := createTestSystemObject(t, nil)
				validators := []*common.Validator{
					{
						ID:              0,
						KramaID:         tests.RandomKramaID(t, 0),
						ConsensusPubKey: tests.RandomIdentifier(t).Bytes(),
					},
					{
						ID:              1,
						KramaID:         tests.RandomKramaID(t, 1),
						ConsensusPubKey: tests.RandomIdentifier(t).Bytes(),
					},
				}
				so.vals = validators

				return so
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				// Validators slice should be shared (same underlying array)
				require.Equal(t, len(original.vals), len(copied.vals))
				require.Equal(t, original.vals[0], copied.vals[0])
				require.Equal(t, original.vals[1], copied.vals[1])

				// Verify they point to the same underlying slice
				require.Equal(t,
					reflect.ValueOf(original.vals).Pointer(),
					reflect.ValueOf(copied.vals).Pointer(),
				)
			},
		},
		{
			name: "should create independent dirty fields map",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()
				so := createTestSystemObject(t, nil)
				so.dirtyFields[fieldTotalValidators] = true
				so.dirtyFields[fieldGenesisTime] = true

				return so
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				// Dirty fields should be copied
				require.True(t, copied.dirtyFields[fieldTotalValidators].(bool)) //nolint
				require.True(t, copied.dirtyFields[fieldGenesisTime].(bool))     //nolint

				// Maps should be independent
				require.NotEqual(t,
					reflect.ValueOf(original.dirtyFields).Pointer(),
					reflect.ValueOf(copied.dirtyFields).Pointer(),
				)

				// Modifying one shouldn't affect the other
				copied.dirtyFields[fieldValidators] = true
				_, ok := original.dirtyFields[fieldValidators]
				require.False(t, ok)
			},
		},
		{
			name: "should deep copy dirty indices",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()
				so := createTestSystemObject(t, nil)
				so.dirtyIndices[fieldValidators] = []uint64{0, 1, 2}

				return so
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				// Dirty indices should be copied
				require.Equal(t, len(original.dirtyIndices[fieldValidators]), len(copied.dirtyIndices[fieldValidators]))
				require.Equal(t, original.dirtyIndices[fieldValidators], copied.dirtyIndices[fieldValidators])

				// Maps should be independent
				require.NotEqual(t,
					reflect.ValueOf(original.dirtyIndices).Pointer(),
					reflect.ValueOf(copied.dirtyIndices).Pointer(),
				)

				// Slices should be independent (deep copy)
				require.NotEqual(t,
					reflect.ValueOf(original.dirtyIndices[fieldValidators]).Pointer(),
					reflect.ValueOf(copied.dirtyIndices[fieldValidators]).Pointer(),
				)

				// Modifying one shouldn't affect the other
				copied.dirtyIndices[fieldValidators][0] = 999
				require.Equal(t, uint64(0), original.dirtyIndices[fieldValidators][0])
				require.Equal(t, uint64(999), copied.dirtyIndices[fieldValidators][0])
			},
		},
		{
			name: "should increment reference counts for shared field references",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()
				so := createTestSystemObject(t, nil)

				return so
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				// Reference count should be incremented
				require.Equal(t, uint(2), original.sharedFieldReferences[fieldValidators].Refs())
				require.Equal(t, uint(2), copied.sharedFieldReferences[fieldValidators].Refs())

				// Both should point to the same reference object
				require.Equal(t,
					reflect.ValueOf(original.sharedFieldReferences[fieldValidators]).Pointer(),
					reflect.ValueOf(copied.sharedFieldReferences[fieldValidators]).Pointer(),
				)
			},
		},
		{
			name: "should create independent shared field references map",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()

				return createTestSystemObject(t, nil)
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				// Maps should be independent
				require.NotEqual(t,
					reflect.ValueOf(original.sharedFieldReferences).Pointer(),
					reflect.ValueOf(copied.sharedFieldReferences).Pointer(),
				)
			},
		},
		{
			name: "should copy underlying Object",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()

				return createTestSystemObject(t, &createStateObjectParams{
					id: tests.RandomIdentifierWithZeroVariant(t),
					soCallback: func(obj *Object) {
						obj.data = common.Account{
							AccType:     common.RegularAccount,
							AssetRoot:   tests.RandomHash(t),
							LogicRoot:   tests.RandomHash(t),
							StorageRoot: tests.RandomHash(t),
							ContextHash: tests.RandomHash(t),
							AssetDeeds:  tests.RandomHash(t),
							KeysHash:    tests.RandomHash(t),
							FileRoot:    tests.RandomHash(t),
						}
						obj.accType = common.RegularAccount
					},
				})
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				require.NotNil(t, copied.Object)

				// Underlying Object should be copied, not shared
				require.NotEqual(t,
					reflect.ValueOf(original.Object).Pointer(),
					reflect.ValueOf(copied.Object).Pointer(),
				)

				// Data should match
				require.Equal(t, original.Object.data, copied.Object.data)
				require.Equal(t, original.Object.id, copied.Object.id)
				require.Equal(t, original.Object.accType, copied.Object.accType)
			},
		},
		{
			name: "should handle empty dirty fields and indices",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()

				return createTestSystemObject(t, nil)
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				require.NotNil(t, copied.dirtyFields)
				require.NotNil(t, copied.dirtyIndices)
				require.Equal(t, 0, len(copied.dirtyFields))
				require.Equal(t, 0, len(copied.dirtyIndices))
			},
		},
		{
			name: "should handle multiple copies with reference counting",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()

				return createTestSystemObject(t, nil)
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				require.Equal(t, uint(2), original.sharedFieldReferences[fieldValidators].Refs())

				copy2 := original.Copy()
				require.Equal(t, uint(3), original.sharedFieldReferences[fieldValidators].Refs())

				copy3 := copied.Copy()
				require.Equal(t, uint(4), original.sharedFieldReferences[fieldValidators].Refs())

				// All should point to the same reference object
				require.Equal(t,
					reflect.ValueOf(original.sharedFieldReferences[fieldValidators]).Pointer(),
					reflect.ValueOf(copied.sharedFieldReferences[fieldValidators]).Pointer(),
				)
				require.Equal(t,
					reflect.ValueOf(original.sharedFieldReferences[fieldValidators]).Pointer(),
					reflect.ValueOf(copy2.sharedFieldReferences[fieldValidators]).Pointer(),
				)
				require.Equal(t,
					reflect.ValueOf(original.sharedFieldReferences[fieldValidators]).Pointer(),
					reflect.ValueOf(copy3.sharedFieldReferences[fieldValidators]).Pointer(),
				)
			},
		},
		{
			name: "should copy with storage trees in underlying Object",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()
				logicIDs := tests.GetLogicIDs(t, 2)
				storageTrees := getStorageTreesWithDefaultEntries(t, 2, 3)

				return createTestSystemObject(t, &createStateObjectParams{
					soCallback: func(obj *Object) {
						obj.storageTrees = storageTrees
						obj.storageTreeTxns = map[identifiers.Identifier]*iradix.Txn{
							logicIDs[0]: iradix.New().Txn(),
							logicIDs[1]: iradix.New().Txn(),
						}
					},
				})
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				require.NotNil(t, copied.Object)
				require.NotNil(t, copied.Object.storageTrees)
				require.Equal(t, len(original.Object.storageTrees), len(copied.Object.storageTrees))
			},
		},
		{
			name: "should preserve all field states after copy",
			setup: func(t *testing.T) *SystemObject {
				t.Helper()
				so := createTestSystemObject(t, nil)

				// Set up complex state
				so.totalValidators = 5
				so.genesisTime = time.Unix(9999999, 0)
				so.vals = []*common.Validator{
					{ID: 0, KramaID: tests.RandomKramaID(t, 0)},
					{ID: 1, KramaID: tests.RandomKramaID(t, 1)},
				}
				so.dirtyFields[fieldTotalValidators] = true
				so.dirtyFields[fieldValidators] = true
				so.dirtyIndices[fieldValidators] = []uint64{0, 1, 2, 3, 4}

				return so
			},
			validate: func(t *testing.T, original, copied *SystemObject) {
				t.Helper()
				// Verify all state is preserved
				require.Equal(t, original.totalValidators, copied.totalValidators)
				require.Equal(t, original.genesisTime, copied.genesisTime)
				require.Equal(t, len(original.vals), len(copied.vals))
				require.Equal(t, len(original.dirtyFields), len(copied.dirtyFields))
				require.Equal(t, len(original.dirtyIndices), len(copied.dirtyIndices))
				require.Equal(t, original.dirtyIndices[fieldValidators], copied.dirtyIndices[fieldValidators])
			},
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			original := test.setup(t)
			copied := original.Copy()

			test.validate(t, original, copied)
		})
	}
}
