package state

import "github.com/sarvalabs/go-moi-identifiers"

type ObjectMap map[identifiers.Address]*Object

func (objects ObjectMap) GetObject(addr identifiers.Address) *Object {
	return objects[addr]
}

func (objects ObjectMap) SetObject(addr identifiers.Address, object *Object) {
	objects[addr] = object
}
