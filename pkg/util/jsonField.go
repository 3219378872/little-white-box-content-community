package util

import (
	"database/sql/driver"
	"encoding/json"
	"errx"
	"fmt"
)

// JSONField 泛型 JSON 类型，可复用 ["", "", ""]
type JSONField[T any] struct {
	Data T
}

// Scan 从数据库读取时自动反序列化
func (j *JSONField[T]) Scan(value any) error {
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan type %T into JSONField", value)
	}
	return json.Unmarshal(bytes, &j.Data)
}

// Value 写入数据库时自动序列化
func (j *JSONField[T]) Value() (driver.Value, error) {
	marshal, err := json.Marshal(j.Data)
	if err != nil {
		return nil, err
	}
	return string(marshal), err
}

func ToJsonObject[T any](t T) *JSONField[T] {
	return &JSONField[T]{
		Data: t,
	}
}

func (j *JSONField[T]) JsonString() (string, error) {
	jsonValue, err := j.Value()
	if err != nil {
		return "", err
	}
	jsonString, ok := jsonValue.(string)
	if !ok {
		return "", fmt.Errorf("json格式转换错误:%w", errx.NewWithCode(errx.SystemError))
	}
	return jsonString, nil
}
