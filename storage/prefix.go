package storage

// Prefix represents the DB Key Prefix
// MSB is not set for account based keys - 0x00 ...
// MSB is set for non-account based keys - 0x80 ...
type Prefix byte

// Account Based Key Prefixes
const (
	Account         Prefix = 0x00
	Context         Prefix = 0x01
	Logic           Prefix = 0x02
	File            Prefix = 0x03
	Storage         Prefix = 0x04
	Balance         Prefix = 0x05
	Registry        Prefix = 0x06
	Approvals       Prefix = 0x07
	PreImage        Prefix = 0x08
	TesseractHeight Prefix = 0x09
)

// Non-Account Based Key Prefixes
const (
	Interaction         Prefix = 0x80
	NTQ                 Prefix = 0x81
	Tesseract           Prefix = 0x82
	TSGridLookup        Prefix = 0x83
	Receipt             Prefix = 0x84
	AccountSyncJob      Prefix = 0x85
	AccountSyncStatus   Prefix = 0x86
	PrincipalSyncStatus Prefix = 0x87
	Bucket              Prefix = 0x88
	BucketCount         Prefix = 0x89
)

func (p Prefix) Byte() byte {
	return byte(p)
}

func (p Prefix) IsAccountBasedKey() bool {
	return !(p&0x80 == 0x80)
}
