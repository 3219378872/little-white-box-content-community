package httpx_test

import (
	"encoding/json"
	"esx/pkg/errx"
	"esx/pkg/httptestx"
	"esx/pkg/httpx"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBindingPresenceNestedObjectsAndInt64(t *testing.T) {
	type nested struct {
		Text string `json:"text"`
	}
	type request struct {
		ID     int64     `path:"id"`
		Before int64     `form:"before,optional"`
		Size   int64     `form:"size,default=20"`
		Name   string    `json:"name"`
		Events []nested  `json:"events,optional"`
		Images *[]string `json:"images,optional"`
	}
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{}`, false}, {`{"name":null}`, false}, {`{"name":"","events":[{}]}`, false},
		{`{"name":"","events":[{"text":"ok"}]}`, true},
		{`{"name":"","images":null}`, true}, {`{"name":"","images":[]}`, true},
		{`{"name":""} {}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/items/9007199254740993?before=9007199254740995", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			r = httptestx.WithVars(r, map[string]string{"id": "9007199254740993"})
			var out request
			err := httpx.Parse(httptestx.Context(r), &out)
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(9007199254740993), out.ID)
			require.Equal(t, int64(9007199254740995), out.Before)
			require.Equal(t, int64(20), out.Size)
			if strings.Contains(tc.body, `"images":[]`) {
				require.NotNil(t, out.Images)
				require.Empty(t, *out.Images)
			} else {
				require.Nil(t, out.Images)
			}
		})
	}
}
func TestUnknownLengthBodyIsBounded(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"`+strings.Repeat("x", 65)+`"}`))
	r.ContentLength = -1
	r.Header.Set("Content-Type", "application/json")
	c := httptestx.Context(r)
	c.Set("maxRequestBody", int64(64))
	var out struct {
		Name string `json:"name"`
	}
	err := httpx.Parse(c, &out)
	var biz *errx.BizError
	require.ErrorAs(t, err, &biz)
	require.Equal(t, errx.FileTooLarge, biz.Code)
	_, body := httpx.MapError(err)
	_, encodeErr := json.Marshal(body)
	require.NoError(t, encodeErr)
}
