package e2e

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/sarvalabs/go-moi/common/hexutil"
	"github.com/sarvalabs/go-moi/common/identifiers"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/exlogics/lockledger"
	"github.com/sarvalabs/go-moi/compute/exlogics/questions"
	"github.com/sarvalabs/go-moi/compute/exlogics/toggler"
	"github.com/sarvalabs/go-moi/compute/exlogics/tokenledger"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-moi/jsonrpc/args"
	"github.com/sarvalabs/go-moi/moiclient"
	"github.com/sarvalabs/go-polo"
)

func (te *TestEnvironment) TestEphemeralLogic() {
	sender := te.chooseRandomAccount()
	manifest := func() []byte {
		engineio.RegisterEngine(pisa.NewEngine())

		file, err := engineio.NewManifestFromFile("./../../compute/exlogics/toggler/toggler.yaml")
		if err != nil {
			panic(err)
		}

		encoded, err := file.Encode(common.POLO)
		if err != nil {
			panic(err)
		}

		return encoded
	}()

	// Deploy the Toggler Logic
	te.CallAndCheckReceipt(te.logicDeploy(sender, &common.LogicPayload{
		Manifest: manifest, Callsite: "", Calldata: nil,
	}))

	// Create a storage reader
	logicID := te.GetLogicID(sender.ID)
	reader := te.moiClient.NewStorageReader(sender.ID, logicID)

	// Invoke the Toggle Callsite
	te.CallAndCheckReceipt(te.logicInvoke(sender, &common.LogicPayload{
		Logic: logicID, Callsite: "Toggle", Calldata: nil,
	}))

	// Check State for SenderID
	value, err := toggler.GetValue(reader)
	require.NoError(te.T(), err)
	require.Equal(te.T(), true, value)
}

func (te *TestEnvironment) TestHybridStateLogic() {
	accounts, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender, _ := accounts[0], accounts[1]

	manifest := func() []byte {
		engineio.RegisterEngine(pisa.NewEngine())

		file, err := engineio.NewManifestFromFile("./../../compute/exlogics/lockledger/lockledger.yaml")
		if err != nil {
			panic(err)
		}

		encoded, err := file.Encode(common.POLO)
		if err != nil {
			panic(err)
		}

		return encoded
	}()

	// Deploy the LockLedger Logic
	te.CallAndCheckReceipt(te.logicDeploy(sender, &common.LogicPayload{
		Manifest: manifest, Callsite: "Seed", Calldata: func() []byte {
			inputs := lockledger.InputSeed{
				Name: "MOI", Symbol: "MOI",
				Supply: 1000000000,
			}

			encoded, err := polo.Polorize(inputs, polo.DocStructs())
			if err != nil {
				require.NoError(te.T(), err)
			}

			return encoded
		}(),
	}))

	logicID := te.GetLogicID(sender.ID)
	persistent := te.moiClient.NewStorageReader(logicID.AsIdentifier(), logicID)

	// Check supply [1000000000]
	supply, err := lockledger.GetPersistentSupply(persistent)
	require.NoError(te.T(), err)
	require.Equal(te.T(), big.NewInt(1000000000), supply)

	// Check symbol [MOI]
	symbol, err := lockledger.GetPersistentSymbol(persistent)
	require.NoError(te.T(), err)
	require.Equal(te.T(), "MOI", symbol)

	te.T().Run("lockup", func(t *testing.T) {
		// Create ephemeral state reader for sender
		senderState := te.moiClient.NewStorageReader(sender.ID, logicID)

		// Check spendable balance for sender
		spendable, err := lockledger.GetEphemeralSpendable(senderState)
		require.NoError(te.T(), err)
		require.Equal(te.T(), uint64(1000000000), spendable)

		// Invoke the Lockup Callsite
		te.CallAndCheckReceipt(te.logicInvoke(sender, &common.LogicPayload{
			Logic: logicID, Callsite: "Lockup", Calldata: func() []byte {
				inputs := lockledger.InputLockup{Amount: 1000}

				encoded, err := polo.Polorize(inputs, polo.DocStructs())
				if err != nil {
					require.NoError(te.T(), err)
				}

				return encoded
			}(),
		}))

		// Check spendable balance for sender
		spendable, err = lockledger.GetEphemeralSpendable(senderState)
		require.NoError(te.T(), err)
		require.Equal(te.T(), uint64(1000000000-1000), spendable)

		// Check lockedup balance for sender
		lockedup, err := lockledger.GetEphemeralLockedup(senderState)
		require.NoError(te.T(), err)
		require.Equal(te.T(), uint64(1000), lockedup)
	})
}

func (te *TestEnvironment) TestLogicWithEvent() {
	accounts, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender, another := accounts[0], accounts[1]

	manifest := func() []byte {
		engineio.RegisterEngine(pisa.NewEngine())

		file, err := engineio.NewManifestFromFile("./../../compute/exlogics/tokenledger/tokenledger.yaml")
		if err != nil {
			panic(err)
		}

		encoded, err := file.Encode(common.POLO)
		if err != nil {
			panic(err)
		}

		return encoded
	}()

	// Deploy the TokenLedger Logic
	te.CallAndCheckReceipt(te.logicDeploy(sender, &common.LogicPayload{
		Manifest: manifest, Callsite: "Seed", Calldata: func() []byte {
			inputs := tokenledger.InputSeed{
				Symbol: "MOI",
				Supply: 1000000000,
			}

			encoded, err := polo.Polorize(inputs, polo.DocStructs())
			if err != nil {
				require.NoError(te.T(), err)
			}

			return encoded
		}(),
	}))

	// Create a storage reader (persistent)
	logicID := te.GetLogicID(sender.ID)
	reader := te.moiClient.NewStorageReader(logicID.AsIdentifier(), logicID)

	// Check supply
	supply, err := tokenledger.GetSupply(reader)
	require.NoError(te.T(), err)
	require.Equal(te.T(), big.NewInt(1000000000), supply)

	// Check symbol
	symbol, err := tokenledger.GetSymbol(reader)
	require.NoError(te.T(), err)
	require.Equal(te.T(), "MOI", symbol)

	// Check balance for sender
	balanceSender, err := tokenledger.GetBalance(reader, sender.ID)
	require.NoError(te.T(), err)
	require.Equal(te.T(), big.NewInt(1000000000), balanceSender)

	// Invoke the Transfer Callsite
	te.CallAndCheckReceipt(te.logicInvoke(sender, &common.LogicPayload{
		Logic: logicID, Callsite: "Transfer", Calldata: func() []byte {
			inputs := tokenledger.InputTransfer{
				Receiver: another.ID,
				Amount:   10000,
			}

			encoded, err := polo.Polorize(inputs, polo.DocStructs())
			if err != nil {
				require.NoError(te.T(), err)
			}

			return encoded
		}(),
	}))

	// Check balance for sender
	balanceSender, err = tokenledger.GetBalance(reader, sender.ID)
	require.NoError(te.T(), err)
	require.Equal(te.T(), big.NewInt(1000000000-10000), balanceSender)

	// Check balance for another
	balanceAnother, err := tokenledger.GetBalance(reader, another.ID)
	require.NoError(te.T(), err)
	require.Equal(te.T(), big.NewInt(10000), balanceAnother)

	// Get the logs from the latest tesseract
	logs, err := te.moiClient.GetLogs(context.Background(), &args.FilterQueryArgs{
		StartHeight: moiclient.NumPointer(-1),
		EndHeight:   moiclient.NumPointer(-1),
		ID:          sender.ID,
	})
	require.NoError(te.T(), err)
	require.Len(te.T(), logs, 1) // Expect 1 log in the latest tesseract

	log := logs[0]
	require.Equal(te.T(), logicID, log.LogicID)
	require.Equal(te.T(), logicID.AsIdentifier(), log.ID)
	require.Equal(te.T(), []common.Hash{
		blake2b.Sum256(must(polo.Polorize("Transfer"))),
		blake2b.Sum256(must(polo.Polorize(sender.ID))),
		blake2b.Sum256(must(polo.Polorize(another.ID))),
	}, log.Topics)
	require.Equal(te.T(), func() hexutil.Bytes {
		doc := make(polo.Document)

		_ = doc.Set("sender", sender.ID)
		_ = doc.Set("receiver", another.ID)
		_ = doc.Set("amount", 10000)

		return doc.Bytes()
	}(), log.Data)
}

func (te *TestEnvironment) TestLogicInterfaces() {
	accounts, err := te.chooseRandomUniqueAccounts(2)
	require.NoError(te.T(), err)

	sender, _ := accounts[0], accounts[1]

	calleeManifest := func() []byte {
		engineio.RegisterEngine(pisa.NewEngine())

		file, err := engineio.NewManifestFromFile("./../../compute/exlogics/questions/answer.yaml")
		if err != nil {
			panic(err)
		}

		encoded, err := file.Encode(common.POLO)
		if err != nil {
			panic(err)
		}

		return encoded
	}()

	callerManifest := func() []byte {
		engineio.RegisterEngine(pisa.NewEngine())

		file, err := engineio.NewManifestFromFile("./../../compute/exlogics/questions/question.yaml")
		if err != nil {
			panic(err)
		}

		encoded, err := file.Encode(common.POLO)
		if err != nil {
			panic(err)
		}

		return encoded
	}()

	// Deploy the LockLedger Logic
	te.CallAndCheckReceipt(te.logicDeploy(sender, &common.LogicPayload{
		Manifest: calleeManifest, Callsite: "Init", Calldata: func() []byte {
			doc := polo.Document{}

			encoded, err := polo.Polorize(doc, polo.DocStructs())
			if err != nil {
				require.NoError(te.T(), err)
			}

			return encoded
		}(),
	}))

	AnswerLogicID := te.GetLogicID(sender.ID)

	fmt.Printf("Answer LogicID: %s\n", AnswerLogicID)

	te.CallAndCheckReceipt(te.logicDeploy(sender, &common.LogicPayload{
		Manifest: callerManifest, Callsite: "", Calldata: func() []byte {
			doc := polo.Document{}

			encoded, err := polo.Polorize(doc, polo.DocStructs())
			if err != nil {
				require.NoError(te.T(), err)
			}

			return encoded
		}(),
	}))

	QuestionLogicID := te.GetLogicID(sender.ID)

	te.CallAndCheckReceipt(te.logicInvoke(sender, &common.LogicPayload{
		Logic:    QuestionLogicID,
		Callsite: "SetActorAnswer",
		Calldata: must(polo.Polorize(questions.InputExternAnswer{
			LogicID: AnswerLogicID.AsIdentifier(),
			Answer:  50,
		}, polo.DocStructs())),
		Interfaces: map[string]identifiers.LogicID{
			"AnswerLogic": AnswerLogicID,
		},
	}))

	reader := te.moiClient.NewStorageReader(sender.ID, AnswerLogicID)

	output, err := questions.GetActorAnswer(reader)
	require.NoError(te.T(), err)
	require.Equal(te.T(), uint64(50), output)

	te.CallAndCheckReceipt(te.logicInvoke(sender, &common.LogicPayload{
		Logic:    QuestionLogicID,
		Callsite: "SetMyAnswer",
		Calldata: must(polo.Polorize(questions.InputExternAnswer{
			LogicID: AnswerLogicID.AsIdentifier(),
			Answer:  70,
		}, polo.DocStructs())),
		Interfaces: map[string]identifiers.LogicID{
			"AnswerLogic": AnswerLogicID,
		},
	}))

	reader = te.moiClient.NewStorageReader(QuestionLogicID.AsIdentifier(), AnswerLogicID)

	output, err = questions.GetActorAnswer(reader)
	require.NoError(te.T(), err)
	require.Equal(te.T(), uint64(70), output)
}

func (te *TestEnvironment) CallAndCheckReceipt(ixhash common.Hash, err error) {
	require.NoError(te.T(), err)
	checkForReceiptSuccess(te.T(), te.moiClient, ixhash)
}

func (te *TestEnvironment) GetLogicID(id identifiers.Identifier) identifiers.LogicID {
	height := moiclient.GetLatestHeight(te.T(), te.moiClient, id)

	return moiclient.GetLogicID(te.T(), te.moiClient, 0, id, int64(height))
}

func must[T any](object T, err error) T {
	if err != nil {
		panic(err)
	}

	return object
}
