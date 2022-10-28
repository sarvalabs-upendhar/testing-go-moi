package ktypes

type GroupID uint8

const (
	InteractionGID GroupID = iota
	NtqGID
	TesseractGID
	AccountGID
	ContextGID
	LogicsGID
	FilesGID
	StorageGID
	BalanceGID
	ApprovalsGID
)

func (groupID GroupID) Byte() byte {
	return []byte{0x01, 0x02, 0x3, 0x01, 0x02, 0x3, 0x04, 0x05, 0x06, 0x07}[groupID]
}

func GetDBKey(address Address, groupID GroupID, hash Hash) []byte {
	if address != NilAddress {
		return append(address.Bytes(), append([]byte{groupID.Byte()}, hash.Bytes()...)...)
	}

	return append([]byte{groupID.Byte()}, hash.Bytes()...)
}
