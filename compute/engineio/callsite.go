package engineio

import (
	"encoding/json"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/sarvalabs/go-polo"
)

// Callsite represents a callable point in a Logic.
// It can be resolved from a string identifier with the GetCallsite method on Logic
type Callsite struct {
	Ptr  ElementPtr   `json:"ptr" yaml:"ptr"`
	Name string       `json:"name" yaml:"name"`
	Kind CallsiteKind `json:"kind" yaml:"kind"`
}

// CallsiteKind represents the type of callable point in a Logic.
type CallsiteKind int

const (
	CallsiteInternal CallsiteKind = iota
	CallsiteInvoke
	CallsiteDeploy
	CallsiteEnlist
	CallsiteInteract
)

func (kind CallsiteKind) String() string {
	str, ok := callsiteKindToString[kind]
	if !ok {
		panic("unknown CallsiteKind variant")
	}

	return str
}

// Polorize implements the polo.Polorizable interface for CallsiteKind
func (kind CallsiteKind) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	polorizer.PolorizeString(kind.String())

	return polorizer, nil
}

// Depolorize implements the polo.Depolorizable interface for CallsiteKind
func (kind *CallsiteKind) Depolorize(depolorizer *polo.Depolorizer) error {
	str, err := depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	*kind, err = NewCallsiteKindFromString(str)
	if err != nil {
		return err
	}

	return nil
}

func (kind CallsiteKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(kind.String())
}

func (kind *CallsiteKind) UnmarshalJSON(data []byte) (err error) {
	str := new(string)
	if err = json.Unmarshal(data, str); err != nil {
		return err
	}

	*kind, err = NewCallsiteKindFromString(*str)
	if err != nil {
		return err
	}

	return nil
}

func (kind CallsiteKind) MarshalYAML() (interface{}, error) {
	return kind.String(), nil
}

func (kind *CallsiteKind) UnmarshalYAML(node *yaml.Node) (err error) {
	str := new(string)
	if err = node.Decode(str); err != nil {
		return err
	}

	*kind, err = NewCallsiteKindFromString(*str)
	if err != nil {
		return err
	}

	return nil
}

func NewCallsiteKindFromString(str string) (CallsiteKind, error) {
	kind, ok := callsiteKindFromString[str]
	if !ok {
		return -1, errors.Errorf("invalid callsite kind: %v", str)
	}

	return kind, nil
}

var callsiteKindToString = map[CallsiteKind]string{
	CallsiteInternal: "internal",
	CallsiteInvoke:   "invoke",
	CallsiteInteract: "interact",
	CallsiteDeploy:   "deploy",
	CallsiteEnlist:   "enlist",
}

var callsiteKindFromString = map[string]CallsiteKind{
	"internal": CallsiteInternal,
	"invoke":   CallsiteInvoke,
	"interact": CallsiteInteract,
	"deploy":   CallsiteDeploy,
	"enlist":   CallsiteEnlist,
}
