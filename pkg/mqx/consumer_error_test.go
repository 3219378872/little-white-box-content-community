package mqx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrPermanentEvent_IsDetectableAfterWrapping(t *testing.T) {
	err := ErrPermanentEvent("missing user_id")

	assert.ErrorContains(t, err, "missing user_id")
	assert.True(t, IsPermanentEvent(err))
	assert.True(t, IsPermanentEvent(errors.Join(errors.New("outer"), err)))
	assert.False(t, IsPermanentEvent(errors.New("temporary storage failure")))
}
