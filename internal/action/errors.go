package action

import "fmt"

type ErrorClass string

const (
	ErrorAction ErrorClass = "action"
	ErrorTarget ErrorClass = "target"
	ErrorScript ErrorClass = "script"
	ErrorMux    ErrorClass = "mux"
	ErrorInput  ErrorClass = "input"
)

// ExecutionError preserves action identity and diagnostic class while exposing
// the underlying cause through errors.Is/errors.As.
type ExecutionError struct {
	ActionID ID
	Class    ErrorClass
	Err      error
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return fmt.Sprintf("action %q (%s) failed", e.ActionID, e.Class)
	}
	return fmt.Sprintf("action %q (%s): %v", e.ActionID, e.Class, e.Err)
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
