package pisa

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/sarvalabs/go-polo"

	"github.com/sarvalabs/moichain/jug/engineio"
)

const (
	StateElement    engineio.ElementKind = "state"
	ConstantElement engineio.ElementKind = "constant"
	RoutineElement  engineio.ElementKind = "routine"
	TypedefElement  engineio.ElementKind = "typedef"
	ClassElement    engineio.ElementKind = "class"
)

var elementGenerators = map[engineio.ElementKind]engineio.ManifestElementGenerator{
	StateElement:    func() engineio.ManifestElementObject { return new(StateSchema) },
	TypedefElement:  func() engineio.ManifestElementObject { return new(TypedefSchema) },
	ConstantElement: func() engineio.ManifestElementObject { return new(ConstantSchema) },
	RoutineElement:  func() engineio.ManifestElementObject { return new(RoutineSchema) },
	ClassElement:    func() engineio.ManifestElementObject { return new(ClassSchema) },
}

type StateSchema struct {
	Kind   engineio.ContextStateKind `yaml:"kind" json:"kind"`
	Fields []TypefieldSchema         `yaml:"fields" json:"fields"`
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

type ClassSchema struct {
	Name   string            `yaml:"name" json:"name"`
	Fields []TypefieldSchema `yaml:"fields" json:"fields"`
}

func (class ClassSchema) Polorize() (*polo.Polorizer, error) {
	polorizer := polo.NewPolorizer()

	if err := polorizer.Polorize(class.Name); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(class.Fields); err != nil {
		return nil, err
	}

	return polorizer, nil
}

func (class *ClassSchema) Depolorize(depolorizer *polo.Depolorizer) (err error) {
	depolorizer, err = depolorizer.DepolorizePacked()
	if errors.Is(err, polo.ErrNullPack) {
		return nil
	} else if err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&class.Name); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&class.Fields); err != nil {
		return err
	}

	return nil
}

type RoutineSchema struct {
	Name string                `yaml:"name" json:"name"`
	Kind engineio.CallsiteKind `yaml:"kind" json:"kind"`

	Accepts []TypefieldSchema `yaml:"accepts" json:"accepts"`
	Returns []TypefieldSchema `yaml:"returns" json:"returns"`

	Executes InstructionsSchema `yaml:"executes" json:"executes"`
	Catches  []string           `yaml:"catches" json:"catches"`
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

	if err := polorizer.Polorize(routine.Executes); err != nil {
		return nil, err
	}

	if err := polorizer.Polorize(routine.Catches); err != nil {
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

	if err = depolorizer.Depolorize(&routine.Executes); err != nil {
		return err
	}

	if err = depolorizer.Depolorize(&routine.Catches); err != nil {
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
	Type  string `yaml:"type" json:"type"`
	Value string `yaml:"value" json:"value"`
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
	Bin BinInstructs `yaml:"bin" json:"bin"`
	Hex string       `yaml:"hex" json:"hex"`
	Asm []string     `yaml:"asm" json:"asm"`
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
