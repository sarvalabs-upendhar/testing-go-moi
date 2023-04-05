package engineio

import (
	"encoding/json"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/moichain/types"
)

// Callsite represents a callable point in a Logic.
// It can be resolved from a string by looking it up on the LogicDriver
type Callsite struct {
	Ptr  ElementPtr
	Kind CallsiteKind
}

// CallsiteKind represents the type of callable point in a Logic.
type CallsiteKind int

const (
	InvokableCallsite CallsiteKind = iota
	InteractableCallsite
	DeployerCallsite
	EnlisterCallsite
)

var callsiteKindToString = map[CallsiteKind]string{
	InvokableCallsite:    "invokable",
	InteractableCallsite: "interactable",
	DeployerCallsite:     "deployer",
	EnlisterCallsite:     "enlister",
}

var callsiteKindFromString = map[string]CallsiteKind{
	"invokable":    InvokableCallsite,
	"interactable": InteractableCallsite,
	"deployer":     DeployerCallsite,
	"enlister":     EnlisterCallsite,
}

// String implements the Stringer interface for CallsiteKind
func (callsite CallsiteKind) String() string {
	str, ok := callsiteKindToString[callsite]
	if !ok {
		panic("unknown CallsiteKind variant")
	}

	return str
}

// IxnType returns the appropriate types.IxType variant for the CallsiteKind
func (callsite CallsiteKind) IxnType() types.IxType {
	switch callsite {
	case InvokableCallsite:
		return types.IxLogicInvoke
	case DeployerCallsite:
		return types.IxLogicDeploy
	case InteractableCallsite:
		return types.IxLogicInteract
	case EnlisterCallsite:
		return types.IxLogicEnlist
	default:
		panic("unknown CallsiteKind variant")
	}
}

// Polorize implements the polo.Polorizable interface for CallsiteKind
func (callsite CallsiteKind) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	polorizer.PolorizeString(callsite.String())

	return polorizer, nil
}

// Depolorize implements the polo.Depolorizable interface for CallsiteKind
func (callsite *CallsiteKind) Depolorize(depolorizer *polo.Depolorizer) error {
	raw, err := depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	kind, ok := callsiteKindFromString[raw]
	if !ok {
		return errors.New("invalid CallsiteKind value")
	}

	*callsite = kind

	return nil
}

// MarshalJSON implements the json.Marshaller interface for CallsiteKind
func (callsite CallsiteKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(callsite.String())
}

// UnmarshalJSON implements the json.Unmarshaller interface for CallsiteKind
func (callsite *CallsiteKind) UnmarshalJSON(data []byte) error {
	raw := new(string)
	if err := json.Unmarshal(data, raw); err != nil {
		return err
	}

	kind, ok := callsiteKindFromString[*raw]
	if !ok {
		return errors.New("invalid CallsiteKind value")
	}

	*callsite = kind

	return nil
}

// MarshalYAML implements the yaml.Marshaller interface for CallsiteKind
func (callsite CallsiteKind) MarshalYAML() (interface{}, error) {
	return callsite.String(), nil
}

// UnmarshalYAML implements the yaml.Unmarshaller interface for CallsiteKind
func (callsite *CallsiteKind) UnmarshalYAML(node *yaml.Node) error {
	raw := new(string)
	if err := node.Decode(raw); err != nil {
		return err
	}

	kind, ok := callsiteKindFromString[*raw]
	if !ok {
		return errors.New("invalid CallsiteKind value")
	}

	*callsite = kind

	return nil
}

// CallEncoder is an interface with capabilities to encode
// inputs and decode outputs for a specific callable site.
// It can be generated from either a Manifest or LogicDriver.
type CallEncoder interface {
	EncodeInputs(map[string]any) (polo.Document, error)
	DecodeOutputs(polo.Document) (map[string]any, error)
}

// CallResult is the output emitted by EngineDriver when making function calls.
// It contains any output values returned along with fuel expended, logs emitted.
// It may also contain a non-Ok error code (any value that is not zero) with some error message.
type CallResult struct {
	Outputs polo.Document

	Fuel Fuel
	Logs []string

	ErrCode    uint64
	ErrMessage string
}

// Ok returns whether the CallResult has an Ok error code (0)
func (result CallResult) Ok() bool {
	return result.ErrCode == 0
}
