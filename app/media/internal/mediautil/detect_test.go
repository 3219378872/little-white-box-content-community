package mediautil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	require.NoError(t, os.WriteFile(p, data, 0o600))
	return p
}

var (
	jpegHeader = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	pngHeader  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	webpHeader = []byte{0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x20}
	mp4Header  = []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x6D, 0x70, 0x34, 0x32, 0x00, 0x00, 0x00, 0x00}
	webmHeader = []byte{
		0x1A, 0x45, 0xDF, 0xA3, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x20,
		0x42, 0x82, 0x88, 0x77, 0x65, 0x62, 0x6D,
	}
	movHeader = []byte{0x00, 0x00, 0x00, 0x14, 0x66, 0x74, 0x79, 0x70, 0x71, 0x74, 0x20, 0x20, 0x00, 0x00, 0x00, 0x00}
	pdfHeader  = []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}
)

func TestDetect_TableDriven(t *testing.T) {
	cases := []struct {
		name        string
		data        []byte
		allowImage  bool
		allowVideo  bool
		wantKind    MediaKind
		wantMIME    string
		expectError bool
	}{
		{"JPEG → image", jpegHeader, true, true, KindImage, "image/jpeg", false},
		{"PNG → image", pngHeader, true, true, KindImage, "image/png", false},
		{"WebP → image", webpHeader, true, true, KindImage, "image/webp", false},
		{"MP4 → video", mp4Header, true, true, KindVideo, "video/mp4", false},
		{"WebM → video", webmHeader, true, true, KindVideo, "video/webm", false},
		{"MOV → video", movHeader, true, true, KindVideo, "video/quicktime", false},
		{"PDF 拒绝", pdfHeader, true, true, KindUnknown, "", true},
		{"空文件拒绝", []byte{}, true, true, KindUnknown, "", true},
		{"仅允许图片收到 MP4 → 拒绝", mp4Header, true, false, KindUnknown, "", true},
		{"仅允许视频收到 JPEG → 拒绝", jpegHeader, false, true, KindUnknown, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFile(t, tc.data)
			got, err := Detect(path, tc.allowImage, tc.allowVideo)
			if tc.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantKind, got.Kind)
			assert.Equal(t, tc.wantMIME, got.MIME)
			assert.NotEmpty(t, got.Ext)
		})
	}
}
