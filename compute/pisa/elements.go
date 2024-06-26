package pisa

import (
	"fmt"
	"strings"

	"github.com/sarvalabs/go-pisa/logic"
	"github.com/sarvalabs/go-pisa/state"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

var (
	ConstantElement = logic.ConstantElement.String()
	TypedefElement  = logic.TypedefElement.String()
	RoutineElement  = logic.RoutineElement.String()
	ClassElement    = logic.ClassElement.String()
	MethodElement   = logic.MethodElement.String()
	StateElement    = logic.StateElement.String()
	EventElement    = logic.EventElement.String()
)

var ElementMetadata = map[engineio.ElementKind]struct {
	pisakind  logic.ElementKind
	generator func() any
}{
	ConstantElement: {logic.ConstantElement, func() any { return new(ConstantSchema) }},
	TypedefElement:  {logic.TypedefElement, func() any { return new(TypedefSchema) }},
	RoutineElement:  {logic.RoutineElement, func() any { return new(RoutineSchema) }},
	ClassElement:    {logic.ClassElement, func() any { return new(ClassSchema) }},
	MethodElement:   {logic.MethodElement, func() any { return new(MethodSchema) }},
	StateElement:    {logic.StateElement, func() any { return new(StateSchema) }},
	EventElement:    {logic.EventElement, func() any { return new(EventSchema) }},
}

type ConstantSchema struct {
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

type TypedefSchema string

type CatchExpressionSchema string

type RoutineSchema struct {
	Name string                `json:"name" yaml:"name"`
	Mode state.Mode            `json:"mode" yaml:"mode"`
	Kind engineio.CallsiteKind `json:"kind" yaml:"kind"`

	Accepts []TypefieldSchema `json:"accepts" yaml:"accepts"`
	Returns []TypefieldSchema `json:"returns" yaml:"returns"`

	Executes InstructionsSchema      `json:"executes" yaml:"executes"`
	Catches  []CatchExpressionSchema `json:"catches" yaml:"catches"`
}

type ClassSchema struct {
	Name string `json:"name" yaml:"name"`

	Fields  []TypefieldSchema   `json:"fields" yaml:"fields"`
	Methods []MethodFieldSchema `json:"methods" yaml:"methods"`
}

type MethodSchema struct {
	Name  string `json:"name" yaml:"name"`
	Class string `json:"class" yaml:"class"`

	Mutable bool              `json:"mutable" yaml:"mutable"`
	Accepts []TypefieldSchema `json:"accepts" yaml:"accepts"`
	Returns []TypefieldSchema `json:"returns" yaml:"returns"`

	Executes InstructionsSchema      `json:"executes" yaml:"executes"`
	Catches  []CatchExpressionSchema `json:"catches" yaml:"catches"`
}

type StateSchema struct {
	Mode   state.Mode        `json:"mode" yaml:"mode"`
	Fields []TypefieldSchema `json:"fields" yaml:"fields"`
}

type EventSchema struct {
	Name   string            `json:"name" yaml:"name"`
	Topics uint8             `json:"topics" yaml:"topics"`
	Fields []TypefieldSchema `json:"fields" yaml:"fields"`
}

type TypefieldSchema struct {
	Slot  uint8  `json:"slot" yaml:"slot"`
	Label string `json:"label" yaml:"label"`
	Type  string `json:"type" yaml:"type"`
}

type MethodFieldSchema struct {
	Ptr  uint64 `json:"ptr" yaml:"ptr"`
	Code uint64 `json:"code" yaml:"code"`
}

type InstructionsSchema struct {
	Bin BinInstructs `json:"bin" yaml:"bin"`
	Hex string       `json:"hex" yaml:"hex"`
	Asm []string     `json:"asm" yaml:"asm"`
}

type BinInstructs []uint8

func (bin BinInstructs) MarshalJSON() ([]byte, error) {
	var result string
	if bin == nil {
		result = "null"
	} else {
		result = strings.Join(strings.Fields(fmt.Sprintf("%d", bin)), ",")
	}

	return []byte(result), nil
}
