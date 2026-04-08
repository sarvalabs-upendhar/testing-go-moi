package state

import (
	"github.com/sarvalabs/go-moi/common/identifiers"
)

type ObjectMap map[identifiers.Identifier]*Object

func (objects ObjectMap) GetObject(id identifiers.Identifier) *Object {
	return objects[id]
}

func (objects ObjectMap) SetObject(id identifiers.Identifier, object *Object) {
	objects[id] = object
}
