package errors

import (
	"fmt"
)

type ErrorType string

const (
	ResourceNotFound ErrorType = "ResourceNotFound"
	ServerError      ErrorType = "CommonServerError"
)

// Error is an implementation of the 'error' interface, which represents an
// error of server.
type Error struct {
	Type         ErrorType
	Message      string
	ResourceType string
	Action       string
	ResouceName  string
}

// Error is method of error interface
func (e *Error) Error() string {
	return fmt.Sprintf("[%s] happened when [%s] type: [%s] name: [%s], msg: [%s]", e.Type, e.Action, e.ResourceType, e.ResouceName, e.Message)
}

func NewResourceNotFoundError(resource, name string, message ...string) error {
	e := &Error{
		Type:         ResourceNotFound,
		ResourceType: resource,
		Action:       "GetResource",
		ResouceName:  name,
	}
	if len(message) > 0 {
		e.Message = message[0]
	}
	return e
}

func IsResourceNotFound(e error) bool {
	er, ok := e.(*Error)
	if ok && er.Type == ResourceNotFound {
		return true
	}
	return false
}

func NewCommonServerError(resource, name, action, message string) error {
	return &Error{
		Type:         ServerError,
		ResourceType: resource,
		Message:      message,
		Action:       action,
		ResouceName:  name,
	}
}

func IsCommonServerError(e error) bool {
	er, ok := e.(*Error)
	if ok && er.Type == ServerError {
		return true
	}
	return false
}
