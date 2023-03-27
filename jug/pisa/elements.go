package pisa

import (
	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

type ElementKind string

const (
	StateElement    ElementKind = "state"
	ConstantElement ElementKind = "constant"
	RoutineElement  ElementKind = "routine"
	TypedefElement  ElementKind = "typedef"
	ClassElement    ElementKind = "class"
)

func ElementRegistry() engineio.ManifestElementRegistry {
	return map[string]engineio.ManifestElementGenerator{
		string(StateElement):    func() engineio.ManifestElementObject { return new(StateSchema) },
		string(TypedefElement):  func() engineio.ManifestElementObject { return new(TypedefSchema) },
		string(ConstantElement): func() engineio.ManifestElementObject { return new(ConstantSchema) },
		string(RoutineElement):  func() engineio.ManifestElementObject { return new(RoutineSchema) },
	}
}

type StateSchema struct {
	Kind   engineio.ContextStateKind `json:"kind"`
	Fields []TypefieldSchema         `json:"fields"`
}

func (state StateSchema) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(state.Kind); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(state.Fields); err != nil {
		return nil, err
	}

	return polorizer, nil
}

func (state *StateSchema) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil
	} else if err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&state.Kind); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&state.Fields); err != nil {
		return err
	}

	return nil
}

type RoutineSchema struct {
	Name string `yaml:"name" json:"name"`

	Kind    engineio.CallsiteKind `yaml:"kind" json:"kind"`
	Accepts []TypefieldSchema     `yaml:"accepts" json:"accepts"`
	Returns []TypefieldSchema     `yaml:"returns" json:"returns"`
	Catches []string              `yaml:"catches" json:"catches"`

	Executes InstructionsSchema `yaml:"executes" json:"executes"`
}

func (routine RoutineSchema) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	polorizer.PolorizeString(routine.Name)

	if err := polorizer.Polorize(routine.Kind); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(routine.Accepts); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(routine.Returns); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(routine.Catches); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(routine.Executes); err != nil {
		return nil, err
	}

	return polorizer, nil
}

func (routine *RoutineSchema) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil
	} else if err != nil {
		return err
	}

	routine.Name, err = depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&routine.Kind); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&routine.Accepts); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&routine.Returns); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&routine.Catches); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&routine.Executes); err != nil {
		return err
	}

	return nil
}

type TypedefSchema string

func (symbolic TypedefSchema) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	polorizer.PolorizeString(string(symbolic))

	return polorizer, nil
}

func (symbolic *TypedefSchema) Depolorize(depolorizer *polo.Depolorizer) error {
	data, err := depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	*symbolic = TypedefSchema(data)

	return nil
}

type ConstantSchema struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func (constant ConstantSchema) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()
	polorizer.PolorizeString(constant.Type)
	polorizer.PolorizeString(constant.Value)

	return polorizer, nil
}

func (constant *ConstantSchema) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil
	} else if err != nil {
		return err
	}

	constant.Type, err = depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	constant.Value, err = depolorizer.DepolorizeString()
	if err != nil {
		return err
	}

	return nil
}

type TypefieldSchema struct {
	Slot  uint8  `yaml:"slot" json:"slot"`
	Label string `yaml:"label" json:"label"`
	Type  string `yaml:"type" json:"type"`
}

type InstructionsSchema struct {
	Bin []byte   `yaml:"bin" json:"bin"`
	Hex string   `yaml:"hex" json:"hex"`
	Asm []string `yaml:"asm" json:"asm"`
}
