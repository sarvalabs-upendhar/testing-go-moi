package test

import (
	"github.com/sarvalabs/go-moi/common"
	"github.com/sarvalabs/go-moi/common/identifiers"
	"github.com/sarvalabs/go-moi/compute/engineio"
	"github.com/sarvalabs/go-polo"
	"github.com/stretchr/testify/suite"
)

type MultiLogicSuite struct {
	suite.Suite
	TestLogicInstance
}

func (suite *MultiLogicSuite) Initialise(
	kind engineio.EngineKind,
	logic ...Logic,
) (*engineio.FuelGauge, error) {
	suite.ti = suite.T()

	return suite.initialise(kind, logic...)
}

func (suite *MultiLogicSuite) Deploy(
	logicID identifiers.LogicID,
	callsite string, input polo.Document,
	output polo.Document, failure *engineio.ErrorResult,
	access map[[32]byte]bool,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(logicID, common.IxLogicDeploy, callsite, input, access, output, failure, opts...)
}

func (suite *MultiLogicSuite) Enlist(
	logicID identifiers.LogicID,
	callsite string, input polo.Document,
	output polo.Document, failure *engineio.ErrorResult,
	access map[[32]byte]bool,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(logicID, common.IxLogicEnlist, callsite, input, access, output, failure, opts...)
}

func (suite *MultiLogicSuite) Invoke(
	logicID identifiers.LogicID,
	callsite string, input polo.Document,
	output polo.Document, failure *engineio.ErrorResult,
	access map[[32]byte]bool,
	opts ...TestSuiteOption,
) {
	suite.callAndCheck(logicID, common.IxLogicInvoke, callsite, input, access, output, failure, opts...)
}

func (suite *MultiLogicSuite) CheckActorStorage(
	logicID identifiers.LogicID,
	id identifiers.Identifier,
	key [32]byte, val any,
) {
	suite.checkActorStorage(id, logicID, key, val)
}

func (suite *MultiLogicSuite) CheckLogicStorage(logicID identifiers.LogicID, key [32]byte, val any) {
	suite.checkStorage(suite.logic[logicID], logicID, key, val)
}

func (suite *MultiLogicSuite) DocGen(values map[string]any) polo.Document {
	return DocGen(suite.T(), values)
}
