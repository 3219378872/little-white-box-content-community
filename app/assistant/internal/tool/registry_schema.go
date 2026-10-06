package tool

import (
	"encoding/json"
	"esx/app/assistant/internal/canonical"
	"esx/pkg/errx"
	"fmt"
	"io"
	"strings"
)

// decodeStrictValue 解码单个 JSON 值并保留数字原文，拒绝尾随内容。
func decodeStrictValue(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

// validateSchemaValue 按工具 schema 的子集（类型、属性、必填、枚举）校验参数，未声明的字段一律拒绝。
func validateSchemaValue(value any, schema map[string]any, path string) error {
	if schema == nil {
		return nil
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties := schemaProperties(schema["properties"])
		for key, item := range object {
			property, exists := properties[key]
			if !exists {
				return fmt.Errorf("%s.%s is not allowed", path, key)
			}
			propertySchema, ok := property.(map[string]any)
			if !ok {
				return fmt.Errorf("%s.%s has an invalid schema", path, key)
			}
			if err := validateSchemaValue(item, propertySchema, path+"."+key); err != nil {
				return err
			}
		}
		for _, required := range schemaRequired(schema["required"]) {
			if _, exists := object[required]; !exists {
				return fmt.Errorf("%s.%s is required", path, required)
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for index, item := range array {
			if err := validateSchemaValue(item, itemSchema, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
	case "integer":
		if !isJSONInteger(value) {
			return fmt.Errorf("%s must be an integer", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "number":
		if !isJSONNumber(value) {
			return fmt.Errorf("%s must be a number", path)
		}
	}
	if enum, ok := schema["enum"]; ok && !enumContains(enum, value) {
		return fmt.Errorf("%s has an invalid value", path)
	}
	return nil
}

// schemaProperties 读取 schema 的 properties。
func schemaProperties(raw any) map[string]any {
	if properties, ok := raw.(map[string]any); ok {
		return properties
	}
	return map[string]any{}
}

// schemaRequired 读取 schema 的 required，兼容 []string 与解码后的 []any。
func schemaRequired(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

// isJSONInteger 判断值是否为整数。
func isJSONInteger(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	_, err := number.Int64()
	return err == nil
}

// isJSONNumber 判断值是否为数字。
func isJSONNumber(value any) bool {
	if number, ok := value.(json.Number); ok {
		_, err := number.Float64()
		return err == nil
	}
	return false
}

// enumContains 判断值是否在枚举内。
func enumContains(raw, value any) bool {
	switch values := raw.(type) {
	case []string:
		text, ok := value.(string)
		if !ok {
			return false
		}
		for _, candidate := range values {
			if candidate == text {
				return true
			}
		}
	case []any:
		for _, candidate := range values {
			if fmt.Sprint(candidate) == fmt.Sprint(value) {
				return true
			}
		}
	}
	return false
}

// strictUnmarshal 把参数解码到具体结构，拒绝未知字段与尾随内容。
func strictUnmarshal(raw string, target any) error {
	raw = canonical.UnwrapArgsJSON(raw)
	if raw == "" {
		raw = "{}"
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("tool arguments contain a trailing JSON value")
		}
		return err
	}
	return nil
}

// truncateRunes 按字符截断并在截断时追加省略号。
func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

// excerptRunes 按字符截取前缀，不加省略号。
func excerptRunes(value string, limit int) string {
	runes := []rune(value)
	return string(runes[:min(len(runes), limit)])
}

// sessionUserID 取调用者用户 ID，未登录时返回 LoginRequired。
func sessionUserID(session *Session) (int64, error) {
	if session == nil || session.UserID <= 0 {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	return session.UserID, nil
}

// CanonicalDigest 计算参数的规范化摘要。
func CanonicalDigest(argsJSON string) (string, error) {
	return canonical.DigestArgs(argsJSON)
}

// objectSchema 构造对象类型的参数 schema。
func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
