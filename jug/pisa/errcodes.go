package pisa

// ErrorCode defines a PISA error code
// 0 -> represents no error (exception)
type ErrorCode uint64

const (
	ErrorCodeOk ErrorCode = iota

	ErrorCodeRanOutOfFuel
	ErrorCodeInvalidManifest
	ErrorCodeStorageCompile
	ErrorCodeConstantCompile
	ErrorCodeTypedefCompile
	ErrorCodeRoutineCompile
	ErrorCodeBindingCompile

	ErrorCodeInvalidCallsite
	ErrorCodeExecutionSetupFail
	ErrorCodeExecutionRuntimeFail
	ErrorCodeStorageBuildFail
)
