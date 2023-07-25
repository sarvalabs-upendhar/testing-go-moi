package state

import "github.com/sarvalabs/go-moi/common"

type ObjectMap map[common.Address]*Object

func (objects ObjectMap) GetObject(addr common.Address) *Object {
	return objects[addr]
}

func (objects ObjectMap) SetObject(addr common.Address, object *Object) {
	objects[addr] = object
}
