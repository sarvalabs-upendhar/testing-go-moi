package exception

// Code defines a PISA error code
// 0 -> represents no error (exception)
type Code uint64

const (
	Ok Code = iota
	FuelExhausted
	EngineNotBootstrapped

	InvalidCallsite
	InvalidIxnCtx
	InputNotFound
	InvalidInputs
	InvalidOutputs
	InvalidJumpsite

	ElementNotFound
	ElementMalformed

	RegisterNotFound
	PointerOverflow
	ValueInit
	InvalidTypeID
	InvalidRegisterType
	MethodNotFound

	ArithmeticOverflow
	ArithmeticDivideByZero

	NilCollection
	CollectionAccess

	StorageRead
	StorageWrite
)

var CodeToString = map[Code]string{
	Ok:                    "Ok",
	FuelExhausted:         "FuelExhausted",
	EngineNotBootstrapped: "EngineNotBootstrapped",

	InvalidCallsite: "InvalidCallsite",
	InvalidIxnCtx:   "InvalidIxnCtx",
	InputNotFound:   "InputNotFound",
	InvalidInputs:   "InvalidInputs",
	InvalidOutputs:  "InvalidOutputs",
	InvalidJumpsite: "InvalidJumpsite",

	ElementNotFound:  "ElementNotFound",
	ElementMalformed: "ElementMalformed",

	PointerOverflow:     "PointerOverflow",
	ValueInit:           "ValueInit",
	InvalidTypeID:       "InvalidTypeID",
	InvalidRegisterType: "InvalidRegisterType",
	MethodNotFound:      "MethodNotFound",

	ArithmeticOverflow:     "ArithmeticOverflow",
	ArithmeticDivideByZero: "ArithmeticDivideByZero",

	NilCollection:    "NilCollection",
	CollectionAccess: "CollectionAccess",

	StorageRead:  "StorageRead",
	StorageWrite: "StorageWrite",
}
