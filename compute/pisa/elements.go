package pisa

import (
	"fmt"
	"strings"

	"github.com/sarvalabs/go-pisa"

	"github.com/sarvalabs/go-moi/compute/engineio"
)

const (
	DynamicState = "dynamic"
)

var (
	LiteralElement = pisa.ArtifactElementLiteral.String()
	TypedefElement = pisa.ArtifactElementType.String()
	RoutineElement = pisa.ArtifactElementCallable.String()
	ClassElement   = pisa.ArtifactElementClass.String()
	MethodElement  = pisa.ArtifactElementMethod.String()
	StateElement   = pisa.ArtifactElementState.String()
	EventElement   = pisa.ArtifactElementEvent.String()
	ExternElement  = pisa.ArtifactElementExtern.String()
	AssetElement   = pisa.ArtifactElementAsset.String()
)

var ElementMetadata = map[engineio.ElementKind]struct {
	pisakind  pisa.ArtifactElementKind
	generator func() any
}{
	LiteralElement: {pisa.ArtifactElementLiteral, func() any { return new(ConstantSchema) }},
	TypedefElement: {pisa.ArtifactElementType, func() any { return new(TypedefSchema) }},
	RoutineElement: {pisa.ArtifactElementCallable, func() any { return new(RoutineSchema) }},
	ClassElement:   {pisa.ArtifactElementClass, func() any { return new(ClassSchema) }},
	MethodElement:  {pisa.ArtifactElementMethod, func() any { return new(MethodSchema) }},
	StateElement:   {pisa.ArtifactElementState, func() any { return new(StateSchema) }},
	EventElement:   {pisa.ArtifactElementEvent, func() any { return new(EventSchema) }},
	ExternElement:  {pisakind: pisa.ArtifactElementExtern, generator: func() any { return new(ExternSchema) }},
	AssetElement:   {pisakind: pisa.ArtifactElementAsset, generator: func() any { return new(AssetSchema) }},
}

type ConstantSchema struct {
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value" yaml:"value"`
}

type TypedefSchema string

type CatchExpressionSchema string

type RoutineSchema struct {
	Name string                `json:"name" yaml:"name"`
	Mode string                `json:"mode" yaml:"mode"`
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

type ExternalRoutineSchema struct {
	Name    string            `json:"name" yaml:"name"`
	Accepts []TypefieldSchema `json:"accepts" yaml:"accepts"`
	Returns []TypefieldSchema `json:"returns" yaml:"returns"`
}

type StateSchema struct {
	Mode   string            `json:"mode" yaml:"mode"`
	Fields []TypefieldSchema `json:"fields" yaml:"fields"`
}

func StateModeToKind(mode string) pisa.StateKind {
	switch mode {
	case "logic":
		return pisa.LogicState
	case "actor":
		return pisa.ActorState
	default:
		return -1
	}
}

type AssetSchema struct {
	Engine string `json:"engine" yaml:"engine"`
}

type ExternSchema struct {
	Name      string                  `json:"name" yaml:"name"`
	Logic     *StateSchema            `json:"logic" yaml:"logic"`
	Actor     *StateSchema            `json:"actor" yaml:"actor"`
	Endpoints []ExternalRoutineSchema `json:"endpoint" yaml:"endpoint"`
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
