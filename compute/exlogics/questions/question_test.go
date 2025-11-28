package questions

import (
	"testing"

	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-moi/compute/engineio/test"
	"github.com/sarvalabs/go-moi/compute/exlogics/questions/answer"
	"github.com/sarvalabs/go-moi/compute/pisa"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"
)

var (
	QuestionLogicID = identifiers.RandomLogicIDv0().AsIdentifier()
	AnswerLogicID   = identifiers.RandomLogicIDv0().AsIdentifier()
)

func TestLogicInterfaceTestSuite(t *testing.T) {
	engineio.RegisterEngine(pisa.NewEngine())
	suite.Run(t, new(LogicInterfaceTestSuite))
}

type LogicInterfaceTestSuite struct {
	test.MultiLogicSuite
}

var (
	SeederID   = identifiers.RandomParticipantIDv0().AsIdentifier()
	ReceiverID = identifiers.RandomParticipantIDv0().AsIdentifier()

	InitialSeed uint64 = 100000000
)

/*
To test logic interfaces, we need to deploy two logics,
A caller logic, In this case, the `questions` logic, and a callee logic, which is the `answer` logic.
We will modify the logic and actor states of the callee logic by invoking endpoints from the caller logic.
*/

func (suite *LogicInterfaceTestSuite) SetupSuite() {
	callerManifest, err := engineio.NewManifestFromFile("./question.yaml")
	suite.Require().NoError(err, "could not read caller manifest file")

	calleeManifest, err := engineio.NewManifestFromFile("./answer/answer.yaml")
	suite.Require().NoError(err, "could not read callee manifest file")

	_, err = suite.Initialise(engineio.PISA, nil, []test.Logic{
		{
			LogicID:  QuestionLogicID,
			Manifest: callerManifest,
			Actors:   []identifiers.Identifier{SeederID},
		},
		{
			LogicID:  AnswerLogicID,
			Manifest: calleeManifest,
		},
	}...)
	suite.Require().NoError(err, "could not initialise test")

	suite.Deploy(AnswerLogicID, "Init", polo.Document{}, nil, nil, nil)

	// Read Answer value to check if the logic state is initialized
	suite.CheckLogicStorage(AnswerLogicID, pisa.GenerateStorageKey(answer.LogicAnswerSlot), 42)
}

func (suite *LogicInterfaceTestSuite) TestSetActorAnswer() {
	// Case 1: Check if the actor state (Sender) is updated correctly
	suite.Invoke(
		QuestionLogicID,
		"SetActorAnswer",
		must(polo.PolorizeDocument(InputExternAnswer{
			LogicID: AnswerLogicID,
			Answer:  50,
		})), nil, nil, nil,
	)
	suite.CheckActorStorage(AnswerLogicID, SeederID, pisa.GenerateStorageKey(answer.ActorAnswerSlot), 50)
}

func (suite *LogicInterfaceTestSuite) TestSetLogicAnswer() {
	// Case 2: Check if the logic state (Callee) is updated correctly
	suite.Invoke(
		QuestionLogicID,
		"SetMyAnswer",
		must(polo.PolorizeDocument(InputExternAnswer{
			LogicID: AnswerLogicID,
			Answer:  30,
		})), nil, nil, nil,
	)

	suite.CheckActorStorage(AnswerLogicID, QuestionLogicID, pisa.GenerateStorageKey(answer.ActorAnswerSlot), 30)
}

func (suite *LogicInterfaceTestSuite) TestAccessConstraints() {
	testcases := []struct {
		name   string
		access map[[32]byte]int
		error  string
	}{
		{
			name: "Access not granted for caller logic",
			access: map[[32]byte]int{
				SeederID:        int(common.MutateLock),
				QuestionLogicID: int(common.NoLock), // Answer logic is not accessible
			},
			error: "actor not accessible",
		},
		{
			name: "Access not granted for callee logic",
			access: map[[32]byte]int{
				SeederID:      int(common.MutateLock),
				AnswerLogicID: int(common.NoLock), // Question logic is not accessible
			},
			// Unfortunately, this is the error message we get from the PISA engine.
			error: "actor not accessible",
		},
		{
			name: "Dynamic access not granted for actor",
			access: map[[32]byte]int{
				SeederID:        int(common.ReadLock), // false here means the actor is not mutable (Static Access)
				AnswerLogicID:   int(common.ReadLock),
				QuestionLogicID: int(common.ReadLock),
			},
			// Unfortunately, this is the error message we get from the PISA engine.
			error: "slot is read-only",
		},
	}
	for _, tc := range testcases {
		suite.Run(tc.name, func() {
			suite.Invoke(
				QuestionLogicID,
				"SetActorAnswer",
				must(polo.PolorizeDocument(InputExternAnswer{
					LogicID: AnswerLogicID,
					Answer:  50,
				})), nil, &engineio.ErrorResult{Error: tc.error}, tc.access,
			)
		})
	}
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}

	return t
}
