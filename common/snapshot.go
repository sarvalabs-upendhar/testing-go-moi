package common

type Snapshot struct {
	CreatedAt int64
	Prefix    []byte
	Entries   []byte
	SinceTS   uint64
	Size      uint64
}
