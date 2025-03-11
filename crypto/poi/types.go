package poi

// poi constants
const (
	ALPHABET = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdef" +
		"ghijkmnopqrstuvwxyz" // Alphabet set for base 58 encoding
	ZeroAddress  = "0x0000000000000000000000000000000000000000" // ZERO ADDRESS STRING
	keyHeaderKDF = "scrypt"                                     // Algorithm used in KDF
	SrpdkLen     = 64                                           // KeyLength in KDF
)

var NodeIGCPath = [3]uint32{6174, 5020, 0}

// WalletKeystoreJSON is combination of privateKey and mnemonic keystore
type WalletKeystoreJSON struct {
	Address string         `json:"address"`
	ID      string         `json:"id"`
	Version int            `json:"version"`
	Crypto  cryptoParams   `json:"Crypto"`
	SRP     MnemonicParams `json:"x-ethers"`
}

// MnemonicParams is cryptographic params of mnemonic keystore
type MnemonicParams struct {
	Client     string `json:"client"`
	Filename   string `json:"gethFilename"`
	Counter    string `json:"mnemonicCounter"`
	CipherText string `json:"mnemonicCiphertext"`
	IGCPath    string `json:"path"`
	Version    string `json:"version"`
}

// cryptoParams is cryptographic params for wallet keystore
type cryptoParams struct {
	Cipher       string                 `json:"cipher"`
	CipherText   string                 `json:"ciphertext"`
	CipherParams map[string]string      `json:"cipherparams"`
	KDF          string                 `json:"kdf"`
	KDFParams    map[string]interface{} `json:"kdfparams"`
	MAC          string                 `json:"mac"`
}

// cryptoParams is cryptographic params for node keystore
type nodeKeystore struct {
	NodeAddress string       `json:"address"`
	KramaID     string       `json:"kramaID"`
	NodeType    string       `json:"nodeType"`
	Crypto      cryptoParams `json:"signature"`
	UUID        string       `json:"id"`
	Version     int          `json:"version"`
	IGCPath     [4]uint32    `json:"igcPath"`
}

type Mnemonic [12]string
