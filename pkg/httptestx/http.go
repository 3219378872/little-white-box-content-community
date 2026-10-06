// Package httptestx adapts native Hertz handlers to existing protocol assertions.
// Production traffic never passes through this test adapter.
package httptestx

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/test/mock"
	"github.com/cloudwego/hertz/pkg/route/param"
)

// varsKey carries path parameters in a net/http request context.
type varsKey struct{}

// WithVars attaches path parameters to a test request.
func WithVars(r *http.Request, vars map[string]string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), varsKey{}, vars))
}

// Context converts a net/http test request into a Hertz request context.
func Context(r *http.Request) *app.RequestContext {
	c := app.NewContext(0)
	c.SetConn(mock.NewConn(""))
	c.Request.SetRequestURI(r.URL.String())
	c.Request.Header.SetMethod(r.Method)
	for name, values := range r.Header {
		for _, value := range values {
			c.Request.Header.Add(name, value)
		}
	}
	if r.Body != nil {
		c.Request.SetBodyStream(r.Body, int(r.ContentLength))
	}
	if vars, ok := r.Context().Value(varsKey{}).(map[string]string); ok {
		for k, v := range vars {
			c.Params = append(c.Params, param.Param{Key: k, Value: v})
		}
	}
	return c
}

// Adapt lets httptest drive a Hertz handler, copying hijacked (streaming) output when present.
func Adapt(handler app.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := Context(r)
		handler(r.Context(), c)
		if c.Response.GetHijackWriter() != nil {
			recorder := c.GetConn().(*mock.Conn).WriterRecorder()
			raw, err := recorder.Peek(recorder.WroteLen())
			if err != nil {
				panic(err)
			}
			response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), r)
			if err != nil {
				panic(err)
			}
			defer func() { _ = response.Body.Close() }()
			for k, v := range response.Header {
				w.Header()[k] = v
			}
			w.WriteHeader(response.StatusCode)
			_, _ = io.Copy(w, response.Body)
			return
		}
		c.Response.Header.VisitAll(func(k, v []byte) { w.Header().Add(string(k), string(v)) })
		w.WriteHeader(c.Response.StatusCode())
		_, _ = w.Write(c.Response.Body())
	}
}

// Chain runs handlers as a Hertz middleware chain.
func Chain(handlers ...app.HandlerFunc) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) { c.SetHandlers(handlers); c.SetIndex(-1); c.Next(ctx) }
}
