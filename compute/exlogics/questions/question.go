package questions

import "github.com/sarvalabs/go-moi/common/identifiers"

type InputExternAnswer struct {
	LogicID identifiers.Identifier `polo:"answerLogicId"`
	Answer  uint64                 `polo:"new_answer"`
}
