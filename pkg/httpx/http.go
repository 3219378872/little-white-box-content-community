// Package httpx implements the public HTTP binding and error boundary for Hertz.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"esx/pkg/errx"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

// MapError converts any error into the public status code and {code, message} body;
// errors that are not BizError are classified by errx.FromHTTPError.
func MapError(err error) (int, any) {
	var b *errx.BizError
	if !errors.As(err, &b) {
		b = errx.FromHTTPError(err)
	}
	return b.HTTPStatus(), map[string]any{"code": b.Code, "message": b.Message}
}

// ErrorCtx writes err through the public error boundary.
func ErrorCtx(_ context.Context, c *app.RequestContext, err error) {
	code, body := MapError(err)
	c.JSON(code, body)
}

// OkJsonCtx writes a successful JSON response.
func OkJsonCtx(_ context.Context, c *app.RequestContext, body any) { c.JSON(http.StatusOK, body) }

// ParamError is the single error returned for any malformed request.
func ParamError() error { return errx.NewWithCode(errx.ParamError) }

// defaultMaxRequestBody applies when no RoutePolicy set a per-route limit.
const defaultMaxRequestBody = int64(10 << 20)

// Parse binds a request into the struct pointed to by out. Fields are bound by tag:
// `path` from route params, `form` from the query string and `json` from a JSON body.
// Required fields that are missing, and values that do not parse, yield ParamError.
func Parse(c *app.RequestContext, out any) error {
	v := reflect.ValueOf(out)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return ParamError()
	}
	v = v.Elem()
	body, err := decodeJSONBody(c, v.Type(), out)
	if err != nil {
		return err
	}
	for i := 0; i < v.NumField(); i++ {
		f, sf := v.Field(i), v.Type().Field(i)
		if !f.CanSet() {
			continue
		}
		binding := fieldBindingOf(sf)
		if binding.name == "" {
			continue
		}
		// JSON fields were already decoded with the body; only presence is checked here.
		if binding.location == "json" {
			if _, found := body[binding.name]; !found && !binding.optional {
				return ParamError()
			}
			continue
		}
		text, present := binding.lookup(c)
		if !present {
			if binding.fallback != "" {
				text = binding.fallback
			} else if binding.optional {
				continue
			} else {
				return ParamError()
			}
		}
		if err := setScalar(f, text); err != nil {
			return err
		}
	}
	return nil
}

// decodeJSONBody reads a JSON body (bounded by the route's maxRequestBody) into out and
// returns its top-level keys for presence checks. Non-JSON requests and empty bodies
// return a nil map. An oversized body is FileTooLarge rather than a parameter error.
func decodeJSONBody(c *app.RequestContext, t reflect.Type, out any) (map[string]json.RawMessage, error) {
	contentType := c.Request.Header.ContentType()
	if len(contentType) == 0 || !strings.HasPrefix(strings.ToLower(string(contentType)), "application/json") {
		return nil, nil
	}
	limit := defaultMaxRequestBody
	if raw, ok := c.Get("maxRequestBody"); ok {
		limit = raw.(int64)
	}
	var data []byte
	var err error
	// Streamed bodies are read with one extra byte so an over-limit body is detectable.
	if reader := c.Request.BodyStream(); reader != nil {
		if limit > 0 {
			reader = io.LimitReader(reader, limit+1)
		}
		data, err = io.ReadAll(reader)
	} else {
		data, err = c.Request.BodyE()
	}
	if limit > 0 && int64(len(data)) > limit {
		return nil, errx.NewWithCode(errx.FileTooLarge)
	}
	if err != nil {
		return nil, ParamError()
	}
	if len(data) == 0 {
		return nil, nil
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(data, &body) != nil || validateJSON(t, data) != nil || json.Unmarshal(data, out) != nil {
		return nil, ParamError()
	}
	return body, nil
}

// fieldBinding describes where a struct field is read from and whether it may be absent.
type fieldBinding struct {
	location string // "path", "query" or "json"
	name     string
	optional bool
	fallback string // value used when absent; implies optional
}

// fieldBindingOf reads the path/form/json tags; path wins over form, form over json.
func fieldBindingOf(sf reflect.StructField) fieldBinding {
	location, tag := "json", sf.Tag.Get("json")
	if x := sf.Tag.Get("path"); x != "" {
		location, tag = "path", x
	} else if x := sf.Tag.Get("form"); x != "" {
		location, tag = "query", x
	}
	parts := strings.Split(tag, ",")
	binding := fieldBinding{location: location, name: parts[0]}
	for _, opt := range parts[1:] {
		if opt == "optional" {
			binding.optional = true
		}
		if strings.HasPrefix(opt, "default=") {
			binding.fallback = strings.TrimPrefix(opt, "default=")
			binding.optional = true
		}
	}
	return binding
}

// lookup returns the raw path or query value and whether it was supplied.
func (b fieldBinding) lookup(c *app.RequestContext) (string, bool) {
	if b.location == "path" {
		text := c.Param(b.name)
		return text, text != ""
	}
	// The public form contract ignores empty values, including empty
	// optional numbers. Scalar fields use the first nonempty value.
	for _, value := range c.QueryArgs().PeekAll(b.name) {
		if len(value) > 0 {
			return string(value), true
		}
	}
	return "", false
}

// setScalar parses text into a string, integer or bool field, allocating pointer fields.
func setScalar(f reflect.Value, text string) error {
	target := f
	if target.Kind() == reflect.Pointer {
		target.Set(reflect.New(target.Type().Elem()))
		target = target.Elem()
	}
	switch target.Kind() {
	case reflect.String:
		target.SetString(text)
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, e := strconv.ParseInt(text, 10, target.Type().Bits())
		if e != nil {
			return ParamError()
		}
		target.SetInt(n)
	case reflect.Bool:
		n, e := strconv.ParseBool(text)
		if e != nil {
			return ParamError()
		}
		target.SetBool(n)
	default:
		return ParamError()
	}
	return nil
}

// RoutePolicy bounds request ingestion without buffering multipart files or SSE output.
func RoutePolicy(timeoutMS int64, maxBody int64, sse bool) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if maxBody > 0 && int64(c.Request.Header.ContentLength()) > maxBody {
			ErrorCtx(ctx, c, errx.NewWithCode(errx.FileTooLarge))
			c.Abort()
			return
		}
		c.Set("maxRequestBody", maxBody)
		if timeoutMS > 0 && !sse {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMS)*time.Millisecond)
			defer cancel()
			// A context deadline alone does not interrupt BodyStream.Read.
			// Bound actual socket reads, but leave writes available for the
			// established JSON error boundary after downstream cancellation.
			if conn := c.GetConn(); conn != nil {
				deadline, _ := ctx.Deadline()
				if err := conn.SetReadDeadline(deadline); err != nil {
					ErrorCtx(ctx, c, errx.NewWithCode(errx.ServiceUnavailable))
					c.Abort()
					return
				}
				defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
			}
		}
		c.Next(ctx)
	}
}

// validateJSON preserves required-field checks inside nested request objects.
// Optional null fields retain their distinction from explicitly empty arrays.
func validateJSON(t reflect.Type, data json.RawMessage) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if string(data) == "null" {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			return ParamError()
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			tag := field.Tag.Get("json")
			if tag == "" || tag == "-" {
				continue
			}
			options := strings.Split(tag, ",")
			optional := false
			for _, option := range options[1:] {
				if option == "optional" || strings.HasPrefix(option, "default=") {
					optional = true
				}
			}
			raw, found := object[options[0]]
			if !found || string(raw) == "null" {
				if !optional {
					return ParamError()
				}
				continue
			}
			if err := validateJSON(field.Type, raw); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		var elements []json.RawMessage
		if err := json.Unmarshal(data, &elements); err != nil {
			return ParamError()
		}
		for _, element := range elements {
			if err := validateJSON(t.Elem(), element); err != nil {
				return err
			}
		}
	}
	return nil
}
