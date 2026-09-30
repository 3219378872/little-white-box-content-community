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

func MapError(err error) (int, any) {
	var b *errx.BizError
	if !errors.As(err, &b) {
		b = errx.FromHTTPError(err)
	}
	return b.HTTPStatus(), map[string]any{"code": b.Code, "message": b.Message}
}
func ErrorCtx(_ context.Context, c *app.RequestContext, err error) {
	code, body := MapError(err)
	c.JSON(code, body)
}
func OkJsonCtx(_ context.Context, c *app.RequestContext, body any) { c.JSON(http.StatusOK, body) }
func ParamError() error                                            { return errx.NewWithCode(errx.ParamError) }
func Parse(c *app.RequestContext, out any) error {
	v := reflect.ValueOf(out)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return ParamError()
	}
	v = v.Elem()
	var body map[string]json.RawMessage
	if len(c.Request.Header.ContentType()) > 0 && strings.HasPrefix(strings.ToLower(string(c.Request.Header.ContentType())), "application/json") {
		var data []byte
		var err error
		limit := int64(10 << 20)
		if raw, ok := c.Get("maxRequestBody"); ok {
			limit = raw.(int64)
		}
		if reader := c.Request.BodyStream(); reader != nil {
			if limit > 0 {
				reader = io.LimitReader(reader, limit+1)
			}
			data, err = io.ReadAll(reader)
		} else {
			data, err = c.Request.BodyE()
		}
		if limit > 0 && int64(len(data)) > limit {
			return errx.NewWithCode(errx.FileTooLarge)
		}
		if err != nil {
			return ParamError()
		}
		if len(data) > 0 {
			if json.Unmarshal(data, &body) != nil || validateJSON(v.Type(), data) != nil || json.Unmarshal(data, out) != nil {
				return ParamError()
			}
		}
	}
	for i := 0; i < v.NumField(); i++ {
		f, sf := v.Field(i), v.Type().Field(i)
		if !f.CanSet() {
			continue
		}
		location, tag := "json", sf.Tag.Get("json")
		if x := sf.Tag.Get("path"); x != "" {
			location, tag = "path", x
		} else if x := sf.Tag.Get("form"); x != "" {
			location, tag = "query", x
		}
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "" {
			continue
		}
		optional := false
		fallback := ""
		for _, opt := range parts[1:] {
			if opt == "optional" {
				optional = true
			}
			if strings.HasPrefix(opt, "default=") {
				fallback = strings.TrimPrefix(opt, "default=")
				optional = true
			}
		}
		if location == "json" {
			if _, found := body[name]; !found && !optional {
				return ParamError()
			}
			continue
		}
		text, present := "", false
		if location == "path" {
			text = c.Param(name)
			present = text != ""
		} else {
			// The public form contract ignores empty values, including empty
			// optional numbers. Scalar fields use the first nonempty value.
			for _, value := range c.QueryArgs().PeekAll(name) {
				if len(value) > 0 {
					text, present = string(value), true
					break
				}
			}
		}
		if !present {
			if fallback != "" {
				text = fallback
			} else if optional {
				continue
			} else {
				return ParamError()
			}
		}
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
