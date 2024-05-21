package pisa

import (
	"github.com/sarvalabs/go-moi/compute/engineio"
	pisalogic "github.com/sarvalabs/go-pisa/logic"
)

type Logic struct {
	driver engineio.LogicDriver
}

func (logic Logic) Callsite(site string) (uint64, bool) {
	callsite, ok := logic.driver.GetCallsite(site)
	if !ok {
		return 0, false
	}

	return callsite.Ptr, true
}

func (logic Logic) Classdef(class string) (uint64, bool) {
	classdef, ok := logic.driver.GetClassdef(class)
	if !ok {
		return 0, false
	}

	return classdef.Ptr, true
}

func (logic Logic) PersistentState() (*pisalogic.Element, bool) {
	ptr, ok := logic.driver.PersistentState()
	if !ok {
		return nil, false
	}

	element, ok := logic.driver.GetElement(ptr)
	if !ok {
		return nil, false
	}

	if element.Kind != StateElement {
		return nil, false
	}

	return &pisalogic.Element{
		Kind: pisalogic.StateElement,
		Deps: element.Deps,
		Data: element.Data,
	}, true
}

func (logic Logic) EphemeralState() (*pisalogic.Element, bool) {
	ptr, ok := logic.driver.EphemeralState()
	if !ok {
		return nil, false
	}

	element, ok := logic.driver.GetElement(ptr)
	if !ok {
		return nil, false
	}

	if element.Kind != StateElement {
		return nil, false
	}

	return &pisalogic.Element{
		Kind: pisalogic.StateElement,
		Deps: element.Deps,
		Data: element.Data,
	}, true
}

func (logic Logic) Element(ptr uint64) (*pisalogic.Element, bool) {
	element, ok := logic.driver.GetElement(ptr)
	if !ok {
		return nil, false
	}

	metadata, ok := ElementMetadata[element.Kind]
	if !ok {
		return nil, false
	}

	return &pisalogic.Element{
		Kind: metadata.pisakind,
		Deps: element.Deps,
		Data: element.Data,
	}, true
}

func (logic Logic) Dependencies(ptr uint64) []uint64 {
	return logic.driver.GetElementDeps(ptr)
}
