// Package configx loads application YAML with explicit defaults and validation.
package configx

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Option customizes Load.
type Option func(*options)

// options holds the Load switches.
type options struct{ env bool }

// UseEnv expands ${VAR} references from the environment before decoding.
func UseEnv() Option { return func(o *options) { o.env = true } }

// MustLoad panics when the configuration cannot be loaded (startup only).
func MustLoad(path string, target any, opts ...Option) {
	if err := Load(path, target, opts...); err != nil {
		panic(err)
	}
}

// Load fills tag defaults, decodes the YAML file over them (expanding ${VAR} with UseEnv)
// and validates required fields, ranges and options.
func Load(path string, target any, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw any
	if err = yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode configuration %s: invalid YAML", path)
	}
	if o.env {
		raw = expand(raw)
	}
	if err = FillDefault(target); err != nil {
		return err
	}
	raw, err = normalize(reflect.TypeOf(target), raw)
	if err != nil {
		return err
	}
	data, err = json.Marshal(raw)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode configuration %s: invalid field type", path)
	}
	if err = validate(reflect.ValueOf(target)); err != nil {
		return err
	}
	return nil
}

// expand substitutes environment variables in every string of the decoded tree.
func expand(v any) any {
	switch x := v.(type) {
	case string:
		if !strings.Contains(x, "${") {
			return x
		}
		return os.ExpandEnv(x)
	case map[string]any:
		for k, item := range x {
			x[k] = expand(item)
		}
	case []any:
		for i, item := range x {
			x[i] = expand(item)
		}
	}
	return v
}

// FillDefault applies `default=` tags to zero-valued fields of a struct pointer.
func FillDefault(target any) error {
	v := reflect.ValueOf(target)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return fmt.Errorf("configuration target must be a pointer")
	}
	return defaults(v.Elem())
}

// defaults walks nested structs and fills zero values from their `default=` tags.
func defaults(v reflect.Value) error {
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		sf := v.Type().Field(i)
		if f.Kind() == reflect.Struct {
			if err := defaults(f); err != nil {
				return err
			}
		}
		for _, option := range strings.Split(sf.Tag.Get("json"), ",") {
			if !strings.HasPrefix(option, "default=") || !f.IsZero() {
				continue
			}
			value := strings.TrimPrefix(option, "default=")
			switch f.Kind() {
			case reflect.String:
				f.SetString(value)
			case reflect.Bool:
				n, e := strconv.ParseBool(value)
				if e != nil {
					return e
				}
				f.SetBool(n)
			case reflect.Int, reflect.Int32, reflect.Int64:
				if f.Type() == reflect.TypeFor[time.Duration]() {
					d, e := time.ParseDuration(value)
					if e != nil {
						return e
					}
					f.SetInt(int64(d))
					continue
				}
				n, e := strconv.ParseInt(value, 10, 64)
				if e != nil {
					return e
				}
				f.SetInt(n)
			case reflect.Float32, reflect.Float64:
				n, e := strconv.ParseFloat(value, 64)
				if e != nil {
					return e
				}
				f.SetFloat(n)
			}
		}
	}
	return nil
}

// validate enforces required fields (fields without optional/default), `options=` and `range=` tags.
func validate(v reflect.Value) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanInterface() {
			continue
		}
		sf := v.Type().Field(i)
		if f.Kind() == reflect.Struct {
			if err := validate(f); err != nil {
				return err
			}
		}
		for _, option := range strings.Split(sf.Tag.Get("json"), ",") {
			if strings.HasPrefix(option, "options=") && !f.IsZero() {
				allowed := strings.Split(strings.TrimPrefix(option, "options="), "|")
				ok := false
				for _, a := range allowed {
					if fmt.Sprint(f.Interface()) == a {
						ok = true
					}
				}
				if !ok {
					return fmt.Errorf("%s has an unsupported value", sf.Name)
				}
			}
			if !strings.HasPrefix(option, "range=") {
				continue
			}
			bound := strings.TrimPrefix(option, "range=")
			if len(bound) < 3 {
				continue
			}
			parts := strings.Split(bound[1:len(bound)-1], ":")
			if len(parts) != 2 {
				continue
			}
			n, err := strconv.ParseFloat(fmt.Sprint(f.Interface()), 64)
			if err != nil {
				return fmt.Errorf("%s must be numeric", sf.Name)
			}
			if min, e := strconv.ParseFloat(parts[0], 64); e == nil && (n < min || (bound[0] == '(' && n == min)) {
				return fmt.Errorf("%s is below its permitted range", sf.Name)
			}
			if max, e := strconv.ParseFloat(parts[1], 64); e == nil && (n > max || (bound[len(bound)-1] == ')' && n == max)) {
				return fmt.Errorf("%s is above its permitted range", sf.Name)
			}
		}
	}
	if x, ok := v.Interface().(interface{ Validate() error }); ok {
		return x.Validate()
	}
	return nil
}

// fieldType finds the struct field matching a YAML key by json tag or name, case-insensitively.
func fieldType(t reflect.Type, key string) reflect.Type {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" {
			name = f.Name
		}
		if strings.EqualFold(name, key) {
			return f.Type
		}
		if f.Anonymous {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				if found := fieldType(ft, key); found != nil {
					return found
				}
			}
		}
	}
	return nil
}

// normalize converts decoded YAML values to the shapes the target fields expect before JSON decoding.
func normalize(t reflect.Type, raw any) (any, error) {
	if t == nil || raw == nil {
		return raw, nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct {
		if m, ok := raw.(map[string]any); ok {
			for k, v := range m {
				n, e := normalize(fieldType(t, k), v)
				if e != nil {
					return nil, fmt.Errorf("invalid configuration field %s", k)
				}
				m[k] = n
			}
			return m, nil
		}
	}
	if t.Kind() == reflect.Slice {
		if a, ok := raw.([]any); ok {
			for i, v := range a {
				n, e := normalize(t.Elem(), v)
				if e != nil {
					return nil, e
				}
				a[i] = n
			}
			return a, nil
		}
	}
	if text, ok := raw.(string); ok {
		switch t.Kind() {
		case reflect.Bool:
			return strconv.ParseBool(text)
		case reflect.Int, reflect.Int32, reflect.Int64:
			if t == reflect.TypeFor[time.Duration]() {
				d, e := time.ParseDuration(text)
				return int64(d), e
			}
			return strconv.ParseInt(text, 10, 64)
		case reflect.Float32, reflect.Float64:
			return strconv.ParseFloat(text, 64)
		}
	}
	return raw, nil
}
