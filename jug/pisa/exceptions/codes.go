package exceptions

// ExceptionCode defines a PISA error code
// 0 -> represents no error (exception)
type ExceptionCode uint64

const (
	ExceptionOk ExceptionCode = iota
	ExceptionFuelExhausted

	ExceptionExecutionSetup
	ExceptionInvalidCallsite

	ExceptionInvalidManifest
	ExceptionElementCompile
	ExceptionBindingCompile

	ExceptionInvalidJump
	ExceptionInputValidate
	ExceptionOutputValidate

	ExceptionNotFound
	ExceptionPointerOverflow
	ExceptionValueInit
	ExceptionInvalidTypeID
	ExceptionInvalidRegisterType
	ExceptionMethodNotFound

	ExceptionArithmeticOverflow
	ExceptionArithmeticDivideByZero

	ExceptionNilCollection
	ExceptionCollectionAccess

	ExceptionStorageRead
	ExceptionStorageWrite
)

var ExceptionToString = map[ExceptionCode]string{
	ExceptionOk:            "Ok",
	ExceptionFuelExhausted: "FuelExhausted",

	ExceptionExecutionSetup:  "ExecutionSetup",
	ExceptionInvalidCallsite: "InvalidCallsite",

	ExceptionInvalidManifest: "InvalidManifest",
	ExceptionElementCompile:  "ElementCompile",
	ExceptionBindingCompile:  "BindingCompile",

	ExceptionInvalidJump:    "InvalidJump",
	ExceptionInputValidate:  "InputValidate",
	ExceptionOutputValidate: "OutputValidate",

	ExceptionNotFound:            "NotFound",
	ExceptionPointerOverflow:     "PointerOverflow",
	ExceptionValueInit:           "ValueInit",
	ExceptionInvalidTypeID:       "InvalidTypeID",
	ExceptionInvalidRegisterType: "InvalidRegisterType",
	ExceptionMethodNotFound:      "MethodNotFound",

	ExceptionArithmeticOverflow:     "ArithmeticOverflow",
	ExceptionArithmeticDivideByZero: "ArithmeticDivideByZero",

	ExceptionNilCollection:    "NilCollection",
	ExceptionCollectionAccess: "CollectionAccess",

	ExceptionStorageRead:  "StorageRead",
	ExceptionStorageWrite: "StorageWrite",
}
