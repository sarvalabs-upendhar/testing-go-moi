package engineio

type Storage interface {
	Identifier() [32]byte
	Root() [32]byte
	ReadPersistentStorage(logicID [32]byte, key [32]byte) ([]byte, error)
	ReadTransientStorage(logicID [32]byte, key [32]byte) ([]byte, error)
	WritePersistentStorage(logicID [32]byte, key [32]byte, value []byte) error
	WriteTransientStorage(logicID [32]byte, key [32]byte, value []byte) error
	DeletePersistentStorage(logicID [32]byte, key [32]byte) (uint64, error)
	DeleteTransientStorage(logicID [32]byte, key [32]byte) (uint64, error)
}

type StorageReader interface {
	Identifier() [32]byte
	Root() [32]byte
	ReadPersistentStorage(logicID [32]byte, key [32]byte) ([]byte, error)
}
