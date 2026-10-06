// Package consent defines the current Assistant capability authorization gate.
package consent

import (
	"context"
	"esx/pkg/errx"
)

// CurrentVersion is the consent text version users must have granted for full Assistant access.
const CurrentVersion int32 = 3

// Reader reads the user's granted consent version.
type Reader interface {
	AgentConsent(context.Context, int64) (int32, bool, error)
}

// Require allows a request only when the user granted exactly the current consent version.
func Require(ctx context.Context, reader Reader, userID int64) error {
	if userID <= 0 {
		return errx.NewWithCode(errx.LoginRequired)
	}
	if reader == nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	version, granted, err := reader.AgentConsent(ctx, userID)
	if err != nil {
		return errx.NewWithCode(errx.ServiceUnavailable)
	}
	if !granted || version != CurrentVersion {
		return errx.NewWithCode(errx.AgentNotAuthorized)
	}
	return nil
}
