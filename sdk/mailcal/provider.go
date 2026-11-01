package mailcal

import (
	"errors"

	"github.com/actonos/plugin-sdk/sdk/mailcal/abi"
)

// Provider is the Go view of the mail/calendar ABI. It has exactly the method set of Backend,
// so anything that implements one implements the other.
type Provider = Backend

// ErrFunctionNotFound is what a Caller returns when the plugin does not export the function
// that was asked for. The bridge turns it into an "unsupported" provider error.
var ErrFunctionNotFound = errors.New("plugin function not found")

// AsError extracts the coded provider error from err, or returns nil when err is not one.
func AsError(err error) *abi.Error {
	var e *abi.Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

// IsCode reports whether err is a provider error with the given code.
func IsCode(err error, code abi.ErrorCode) bool {
	e := AsError(err)
	return e != nil && e.Code == code
}
