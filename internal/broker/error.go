package broker

import "fmt"

type Error struct {
	message    string
	statusCode int
}

func newError(message string, args ...any) *Error {
	return &Error{
		message:    fmt.Sprintf(message, args...),
		statusCode: 500,
	}
}

func (e *Error) WithStatusCode(statusCode int) *Error {
	e.statusCode = statusCode
	return e
}

func (e *Error) StatusCode() int {
	return e.statusCode
}

func (e *Error) Error() string {
	return e.message
}
