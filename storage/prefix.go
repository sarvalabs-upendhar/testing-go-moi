package storage

import (
	"github.com/sarvalabs/go-moi-identifiers"
)

// PrefixTag represents the tag used for DB Key prefix
// MSB is not set for account based keys - 0x00 ...
// MSB is set for non-account based keys - 0x80 ...
type PrefixTag byte

// Account Based Key Prefixes
const (
	Account         PrefixTag = 0x00
	Context         PrefixTag = 0x01
	Asset           PrefixTag = 0x02
	Logic           PrefixTag = 0x03
	File            PrefixTag = 0x04
	Storage         PrefixTag = 0x05
	Deeds           PrefixTag = 0x06
	PreImage        PrefixTag = 0x07
	TesseractHeight PrefixTag = 0x08
	LogicManifest   PrefixTag = 0x09
)

// Non-Account Based Key Prefixes
const (
	Interaction         PrefixTag = 0x80
	Senatus             PrefixTag = 0x81
	SenatusPeerCount    PrefixTag = 0x82
	Tesseract           PrefixTag = 0x83
	TesseractCommitInfo PrefixTag = 0x84
	Receipt             PrefixTag = 0x85
	AccountSyncJob      PrefixTag = 0x86
	AccountSyncStatus   PrefixTag = 0x87
	PrincipalSyncStatus PrefixTag = 0x88
	Bucket              PrefixTag = 0x89
	BucketCount         PrefixTag = 0x8A
	ConsensusProposals  PrefixTag = 0x8B
	ConsensusSafetyInfo PrefixTag = 0x8C
)

const (
	NonAccountPrefix = "nonacc_"
)

func (p PrefixTag) Byte() byte {
	return byte(p)
}

func (p PrefixTag) IsAccountBasedKey() bool {
	return !(p&0x80 == 0x80)
}

func SenatusPrefix() []byte {
	return dbKey(identifiers.NilAddress, Senatus, nil)
}

func AccountSyncPrefix() []byte {
	return dbKey(identifiers.NilAddress, AccountSyncJob, nil)
}
