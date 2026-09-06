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

func (j *JSONField[T]) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan type %T into JSONField", value)
	}
	return json.Unmarshal(bytes, &j.Data)
}

func (j *JSONField[T]) Value() (driver.Value, error) {
	payload, err := json.Marshal(j.Data)
	if err != nil {
		return nil, err
	}
	return string(payload), nil
}

func ToJSONObject[T any](t T) *JSONField[T] {
	return &JSONField[T]{Data: t}
}

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
