package httpx_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"esx/pkg/httpx"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/network/standard"
	"github.com/stretchr/testify/require"
)

// Real loopback connections exercise the reader that a canceled context alone
// cannot unblock. No mock HTTP adapter or external services are involved.
func TestRouteDeadlineInterruptsSlowStreamingBodies(t *testing.T) {
	for _, transport := range []string{"default", "standard"} {
		for _, contentType := range []string{"application/json", "multipart/form-data; boundary=test"} {
			t.Run(transport+"/"+contentType, func(t *testing.T) {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				h := server.New(server.WithListener(ln), server.WithStreamBody(true), server.WithDisablePreParseMultipartForm(true))
				if transport == "standard" {
					h = server.New(server.WithListener(ln), server.WithStreamBody(true), server.WithDisablePreParseMultipartForm(true), server.WithTransport(standard.NewTransporter))
				}
				entered, finished := make(chan struct{}), make(chan error, 1)
				h.POST("/body", httpx.RoutePolicy(100, 1024*1024, false), func(ctx context.Context, c *app.RequestContext) {
					close(entered)
					var err error
					if strings.HasPrefix(contentType, "application/json") {
						var out struct {
							Value string `json:"value"`
						}
						err = httpx.Parse(c, &out)
					} else {
						_, err = c.MultipartForm()
					}
					finished <- err
				})
				go func() { _ = h.Run() }()
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					_ = h.Shutdown(ctx)
					_ = ln.Close()
				})
				conn, err := net.Dial("tcp", ln.Addr().String())
				require.NoError(t, err)
				defer conn.Close()
				_, err = fmt.Fprintf(conn, "POST /body HTTP/1.1\r\nHost: test\r\nContent-Type: %s\r\nTransfer-Encoding: chunked\r\n\r\n", contentType)
				require.NoError(t, err)
				// Start a body but never send the final chunk. A drip stays active enough
				// to evade an inactivity-only timeout.
				_, err = io.WriteString(conn, "1\r\n{\r\n")
				require.NoError(t, err)
				select {
				case <-entered:
				case <-time.After(time.Second):
					t.Fatal("route never entered")
				}
				started := time.Now()
				stop := make(chan struct{})
				defer close(stop)
				go func() {
					ticker := time.NewTicker(15 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-stop:
							return
						case <-ticker.C:
							if _, err := io.WriteString(conn, "1\r\n \r\n"); err != nil {
								return
							}
						}
					}
				}()
				select {
				case err := <-finished:
					require.Error(t, err)
					require.Less(t, time.Since(started), 500*time.Millisecond)
				case <-time.After(time.Second):
					t.Fatal("stream read survived absolute route deadline")
				}
			})
		}
	}
}

func TestRouteDeadlinePreservesSuccessKeepAliveAndSSE(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	h := server.New(server.WithListener(ln), server.WithStreamBody(true))
	h.POST("/ok", httpx.RoutePolicy(30, 1024, false), func(ctx context.Context, c *app.RequestContext) {
		var out struct {
			Value string `json:"value"`
		}
		require.NoError(t, httpx.Parse(c, &out))
		c.String(200, "ok")
	})
	h.GET("/sse", httpx.RoutePolicy(20, 1024, true), func(ctx context.Context, c *app.RequestContext) {
		time.Sleep(70 * time.Millisecond)
		require.NoError(t, ctx.Err())
		c.String(200, "event: done\n\n")
	})
	go func() { _ = h.Run() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = h.Shutdown(ctx)
		_ = ln.Close()
	})
	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for i := 0; i < 2; i++ {
		_, err = io.WriteString(conn, "POST /ok HTTP/1.1\r\nHost: test\r\nContent-Type: application/json\r\nContent-Length: 13\r\n\r\n{\"value\":\"x\"}")
		require.NoError(t, err)
		resp, err := http.ReadResponse(reader, nil)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, "ok", string(body))
		time.Sleep(50 * time.Millisecond)
	}
	resp, err := http.Get("http://" + ln.Addr().String() + "/sse")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "event: done")
}
