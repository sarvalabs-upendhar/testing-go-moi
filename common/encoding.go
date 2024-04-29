package common

// Encoding is an enum with variants that describe
// encoding schemes supported for file objects.
type Encoding int

const (
	POLO Encoding = iota
	JSON
	YAML
)

func EncodingFromString(encoding string) Encoding {
	// todo: return error for invalid option
	switch encoding {
	case "POLO":
		return POLO
	case "JSON":
		return JSON
	case "YAML":
		return YAML
	default:
		return POLO
	}
}
