package mqx

import (
	"errors"
	"fmt"
)

type permanentEventError struct {
	reason string
}

func (e permanentEventError) Error() string {
	return fmt.Sprintf("permanent event: %s", e.reason)
}

func ErrPermanentEvent(reason string) error {
	return permanentEventError{reason: reason}
}

func IsPermanentEvent(err error) bool {
	var target permanentEventError
	return errors.As(err, &target)
}
