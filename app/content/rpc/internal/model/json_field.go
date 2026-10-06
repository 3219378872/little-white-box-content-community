package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"esx/pkg/errx"
)

// JSONField serializes a typed value as a JSON column.
type JSONField[T any] struct {
	Data T
}

// Scan decodes a JSON column into Data.
func (j *JSONField[T]) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan type %T into JSONField", value)
	}
	return json.Unmarshal(bytes, &j.Data)
}

// Value encodes Data as a JSON string for the driver.
func (j *JSONField[T]) Value() (driver.Value, error) {
	payload, err := json.Marshal(j.Data)
	if err != nil {
		return nil, err
	}
	return string(payload), nil
}

// ToJSONObject wraps a value so it can be written to a JSON column.
func ToJSONObject[T any](t T) *JSONField[T] {
	return &JSONField[T]{Data: t}
}

// JSONString returns the encoded JSON text.
func (j *JSONField[T]) JSONString() (string, error) {
	value, err := j.Value()
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("json format conversion failed: %w", errx.NewWithCode(errx.SystemError))
	}
	return text, nil
}
