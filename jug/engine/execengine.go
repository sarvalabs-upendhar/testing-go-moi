package engine

// Kind is an enum with its variants
// representing the execution engine
type Kind string

const (
	PISA Kind = "PISA"
	MERU Kind = "MERU"
)

// ExecutionEngine is an interface that defines an execution environment
// for compiling LogicObjects and executing its routines. It must be able to
// execute without needing to be initialized.
type ExecutionEngine interface {
	// Kind must return the type of engine for ExecutionEngine
	Kind() Kind
	// Fuel must return the fuel tank of the engine
	Fuel() *FuelTank

	// Init must initialize a logic and its storage by invoking its constructor with the given input data.
	// Must return error for invalid input data form or execution failures.
	Init(*LogicObject, StorageObject, Values) error
	// Run must perform a logic routine with the given input data.
	// Must return error for invalid input data for or execution failures.
	Run(string, *LogicObject, StorageObject, Values) (Values, error)

	// CompileLogic must compile a logic manifest into a LogicObject
	// and return an error for any compilation failures.
	CompileLogic([]byte) (*LogicObject, error)
	// CompileManifest must compile a LogicObject into a logic manifest
	// and return an error for any compilation failures.
	CompileManifest(*LogicObject) ([]byte, error)
}

// Factory is an interface that defines an engine generator for different runtimes.
// This allows each engine implementation to define some base generation logic and
// reduce the overhead of creating multiple engine instances.
type Factory interface {
	// Kind must return the type of the engine factory
	Kind() Kind

	// NewExecutionEngine must construct and return a new
	// ExecutionEngine with the specified fuel capacity
	NewExecutionEngine(fuelCap uint64) ExecutionEngine
	//// NewAssetEngine must construct and return a new AssetEngine
	// NewAssetEngine(fuelCap uint64) AssetEngine
}

// Values defines an interface for accessing string indexed data.
// It represents the form for engine input/output data.
// Implemented by polo.Document.
type Values interface {
	Size() int
	Bytes() []byte

	Get(string) []byte
	Set(string, []byte)

	GetObject(string, any) error
	SetObject(string, any) error
}
