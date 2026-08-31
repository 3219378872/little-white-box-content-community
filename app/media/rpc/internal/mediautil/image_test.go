package mediautil

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePNGHeader(t *testing.T, width, height uint32) string {
	t.Helper()
	var raw bytes.Buffer
	raw.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2
	_ = binary.Write(&raw, binary.BigEndian, uint32(len(ihdr)))
	raw.WriteString("IHDR")
	raw.Write(ihdr)
	crcPayload := append([]byte("IHDR"), ihdr...)
	_ = binary.Write(&raw, binary.BigEndian, crc32.ChecksumIEEE(crcPayload))
	path := filepath.Join(t.TempDir(), "header.png")
	require.NoError(t, os.WriteFile(path, raw.Bytes(), 0o600))
	return path
}

func writeTestJPEG(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
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
	out, w, h, err := CompressImage(context.Background(), src, 1000, 1000, 80)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(out) })

	assert.LessOrEqual(t, w, 1000)
	assert.LessOrEqual(t, h, 1000)
	assert.Greater(t, w, 500)
}

func TestCompressImage_DoesNotUpscale(t *testing.T) {
	src := writeTestJPEG(t, 100, 100)
	out, w, h, err := CompressImage(context.Background(), src, 1000, 1000, 85)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(out) })

	assert.Equal(t, 100, w)
	assert.Equal(t, 100, h)
}

func TestCompressImage_ConvertsPNGToJPEG(t *testing.T) {
	src := writeTestPNG(t, 500, 500)
	out, _, _, err := CompressImage(context.Background(), src, 1000, 1000, 85)
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
	out, err := MakeThumbnail(context.Background(), src)
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
	out, err := MakeThumbnail(context.Background(), src)
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

func TestValidateImageDimensionsRejectsPixelAndSideBudgets(t *testing.T) {
	tests := []struct {
		name          string
		width, height uint32
	}{
		{name: "pixel budget", width: 5001, height: 5000},
		{name: "side budget", width: uint32(MaxImageSide + 1), height: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writePNGHeader(t, tc.width, tc.height)
			_, _, err := ValidateImageDimensions(path)
			require.ErrorIs(t, err, ErrImageDimensionsExceeded)
		})
	}
}

func TestImageDecodePermitCapsConcurrency(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := withImageDecodePermit(context.Background(), func() error {
				current := active.Add(1)
				for {
					previous := maximum.Load()
					if current <= previous || maximum.CompareAndSwap(previous, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				active.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("decode permit: %v", err)
			}
		}()
	}
	<-started
	<-started
	select {
	case <-started:
		t.Fatal("more than two decodes started before a permit was released")
	default:
	}
	close(release)
	wg.Wait()
	if got := maximum.Load(); got != int32(MaxConcurrentDecodes) {
		t.Fatalf("maximum concurrent decodes = %d, want %d", got, MaxConcurrentDecodes)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := withImageDecodePermit(cancelled, func() error { return errors.New("must not run") }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquire = %v", err)
	}
}
