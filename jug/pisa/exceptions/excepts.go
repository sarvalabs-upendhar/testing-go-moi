package exceptions

import "fmt"

type ExceptionObject struct {
	Code ExceptionCode
	Data string
}

func Exception(kind ExceptionCode, data string) *ExceptionObject {
	return &ExceptionObject{kind, data}
}

func Exceptionf(kind ExceptionCode, format string, a ...any) *ExceptionObject {
	return Exception(kind, fmt.Sprintf(format, a...))
}

func (exception ExceptionObject) String() string {
	return fmt.Sprintf("%v: %v", ExceptionToString[exception.Code], exception.Data)
}
