package exception

import "fmt"

type Object struct {
	Code Code
	Data string
}

func Exception(code Code, data string) *Object {
	return &Object{code, data}
}

func Exceptionf(code Code, format string, a ...any) *Object {
	return Exception(code, fmt.Sprintf(format, a...))
}

func (exception Object) String() string {
	return fmt.Sprintf("%v: %v", CodeToString[exception.Code], exception.Data)
}
