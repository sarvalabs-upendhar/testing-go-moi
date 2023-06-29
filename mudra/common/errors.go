package common

import "errors"

var (
	ErrInDecryption         = errors.New("could not decrypt key with given password")
	ErrUnsupportedNodeType  = errors.New("invalid node option")
	ErrMOIIDBaseURLNotFound = errors.New(
		"MOI_ID_BASE_URL NOT FOUND: MOI id Base URL not found in given environment")
	ErrAuthFailed                      = errors.New("authentication failed with given credentials")
	ErrInvalidKramaID                  = errors.New("invalid Krama id")
	ErrInvalidKramaIDVersion           = errors.New("invalid Krama id Version")
	ErrNoKeystore                      = errors.New("no keystore at given datadir")
	ErrUnsupportedSigTypeForPrivateKey = errors.New("unsupported Signature type for given private key type")
	ErrUnsupportedSigType              = errors.New("unsupported Signature type")
	ErrInvalidUsername                 = errors.New("invalid username")
	ErrZeroKey                         = errors.New("received secret key is zero")
	ErrInvalidBLSPublicKeyLength       = errors.New("invalid length for BLS public key")
	ErrEmpty                           = errors.New("given data is empty")
	ErrInsufficientSigLength           = errors.New("signature length is insufficient")
	ErrInvalidBLSSignature             = errors.New("invalid BLS Signature")
	ErrUnsupportedAggSignature         = errors.New("invalid signature type for aggregation")
	ErrUnsupportedSig                  = errors.New("currently this Signature type is not supported")
	ErrUnIntialized                    = errors.New("uninitialized")
	ErrParsingKramaID                  = errors.New("error parsing kramaID")
	ErrNotPairingFriendlyKey           = errors.New("not a pairing friendly private key")
	ErrInvalidPrivKeyLength            = errors.New("secret key must be 32 bytes")
	ErrSigningFailed                   = errors.New("error in signing")
	ErrDerivingPrivKeyFromSRP          = errors.New("error deriving the private bytes from SRP")
	ErrMnemonicMandatory               = errors.New("seedPhrase/mnemonic must be passed through config in Register Mode")
	ErrSignOptionsNotPassed            = errors.New("pass IgcPath through signOptions inorder to sign using ECDSA")
)
