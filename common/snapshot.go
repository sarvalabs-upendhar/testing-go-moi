package common

type Snapshot struct {
	CreatedAt     int64
	Prefix        []byte
	Entries       []byte
	SinceTS       uint64
	TotalSnapSize uint64
}

type SnapResponse struct {
	MetaInfo   *SnapMetaInfo
	Data       []byte
	ChunkSize  uint64
	Start, End bool
}

type SnapMetaInfo struct {
	Hash          Hash
	CreatedAt     int64
	TotalSnapSize uint64
}
