package mediaupload

import (
	"bytes"
	"esx/pkg/errx"
	"esx/pkg/httptestx"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileLimitExcludesMultipartOverhead(t *testing.T) {
	for _, size := range []int{15, 16, 17} {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		f, e := mw.CreateFormFile("file", "x.wav")
		require.NoError(t, e)
		_, e = f.Write(make([]byte, size))
		require.NoError(t, e)
		require.NoError(t, mw.WriteField("idempotencyKey", "stable"))
		require.NoError(t, mw.Close())
		r := httptest.NewRequest("POST", "/", &b)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		c := httptestx.Context(r)
		defer Cleanup(c)
		file, h, key, e := Bind(c, 16)
		if size > 16 {
			require.True(t, errx.Is(e, errx.FileTooLarge))
			continue
		}
		require.NoError(t, e)
		require.Equal(t, int64(size), h.Size)
		require.Equal(t, "stable", key)
		require.NoError(t, file.Close())
	}
}
