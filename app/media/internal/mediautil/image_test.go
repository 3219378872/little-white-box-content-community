package mediautil

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestJPEG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 128, A: 255})
		}
	}
	p := filepath.Join(t.TempDir(), "in.jpg")
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, jpeg.Encode(f, img, &jpeg.Options{Quality: 95}))
	return p
}

func writeTestPNG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	p := filepath.Join(t.TempDir(), "in.png")
	f, err := os.Create(p)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, png.Encode(f, img))
	return p
}

func TestCompressImage_ShrinksOversized(t *testing.T) {
	src := writeTestJPEG(t, 3000, 2000)
	out, w, h, err := CompressImage(src, 1000, 1000, 80)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(out) })

	assert.LessOrEqual(t, w, 1000)
	assert.LessOrEqual(t, h, 1000)
	assert.Greater(t, w, 500)
}

func TestCompressImage_DoesNotUpscale(t *testing.T) {
	src := writeTestJPEG(t, 100, 100)
	out, w, h, err := CompressImage(src, 1000, 1000, 85)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(out) })

	assert.Equal(t, 100, w)
	assert.Equal(t, 100, h)
}

func TestCompressImage_ConvertsPNGToJPEG(t *testing.T) {
	src := writeTestPNG(t, 500, 500)
	out, _, _, err := CompressImage(src, 1000, 1000, 85)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(out) })

	f, err := os.Open(out)
	require.NoError(t, err)
	defer f.Close()
	head := make([]byte, 3)
	_, err = f.Read(head)
	require.NoError(t, err)
	assert.Equal(t, byte(0xFF), head[0])
	assert.Equal(t, byte(0xD8), head[1])
	assert.Equal(t, byte(0xFF), head[2])
}

func TestMakeThumbnail_LongSideIs256(t *testing.T) {
	src := writeTestJPEG(t, 1000, 500)
	out, err := MakeThumbnail(src)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(out) })

	f, err := os.Open(out)
	require.NoError(t, err)
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	require.NoError(t, err)
	assert.Equal(t, 256, cfg.Width)
	assert.Equal(t, 128, cfg.Height)
}

func TestMakeThumbnail_PortraitLongSideIs256(t *testing.T) {
	src := writeTestJPEG(t, 500, 1000)
	out, err := MakeThumbnail(src)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(out) })

	f, err := os.Open(out)
	require.NoError(t, err)
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	require.NoError(t, err)
	assert.Equal(t, 128, cfg.Width)
	assert.Equal(t, 256, cfg.Height)
}
