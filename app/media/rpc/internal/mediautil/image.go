package mediautil

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

const (
	MaxImagePixels       int64 = 25_000_000
	MaxImageSide         int   = 8192
	MaxConcurrentDecodes int   = 2
)

var (
	ErrImageDimensionsExceeded = errors.New("media: image dimensions exceed processing budget")
	imageDecodeSlots           = make(chan struct{}, MaxConcurrentDecodes)
)

// ValidateImageDimensions parses only the image header. It must run before any
// full decode so compressed images cannot force an unbounded pixel allocation.
func ValidateImageDimensions(srcPath string) (int, int, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return 0, 0, fmt.Errorf("media: open image config: %w", err)
	}
	defer func() { _ = file.Close() }()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return 0, 0, fmt.Errorf("media: decode image config: %w", err)
	}
	width, height := config.Width, config.Height
	if width <= 0 || height <= 0 || width > MaxImageSide || height > MaxImageSide ||
		int64(width)*int64(height) > MaxImagePixels {
		return width, height, ErrImageDimensionsExceeded
	}
	return width, height, nil
}

func withImageDecodePermit(ctx context.Context, decode func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case imageDecodeSlots <- struct{}{}:
		defer func() { <-imageDecodeSlots }()
		return decode()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CompressImage 将 srcPath 指向的图片按 maxW × maxH 等比缩放（不放大），
// 以 JPEG 格式输出到一个新临时文件。maxW / maxH 为 0 表示该维度不限。
// 返回输出路径、最终宽高、error。
func CompressImage(ctx context.Context, srcPath string, maxW, maxH, quality int) (string, int, int, error) {
	var outPath string
	var targetW, targetH int
	err := withImageDecodePermit(ctx, func() error {
		img, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
		if err != nil {
			return fmt.Errorf("media: open for compress: %w", err)
		}

		origW, origH := img.Bounds().Dx(), img.Bounds().Dy()
		targetW, targetH = origW, origH
		if maxW > 0 && targetW > maxW {
			ratio := float64(maxW) / float64(targetW)
			targetW = maxW
			targetH = int(float64(targetH) * ratio)
		}
		if maxH > 0 && targetH > maxH {
			ratio := float64(maxH) / float64(targetH)
			targetH = maxH
			targetW = int(float64(targetW) * ratio)
		}
		if targetW != origW || targetH != origH {
			img = imaging.Resize(img, targetW, targetH, imaging.Lanczos)
		}

		out, err := os.CreateTemp("", "media-compress-*.jpg")
		if err != nil {
			return fmt.Errorf("media: create compress temp: %w", err)
		}
		outPath = out.Name()
		if err = imaging.Encode(out, img, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
			_ = out.Close()
			_ = os.Remove(outPath)
			outPath = ""
			return fmt.Errorf("media: encode jpeg: %w", err)
		}
		if err = out.Close(); err != nil {
			_ = os.Remove(outPath)
			outPath = ""
			return fmt.Errorf("media: close compress temp: %w", err)
		}
		return nil
	})
	return outPath, targetW, targetH, err
}

const thumbLongSide = 256

// MakeThumbnail 生成长边为 thumbLongSide 的 JPEG 缩略图到新临时文件。
func MakeThumbnail(ctx context.Context, srcPath string) (string, error) {
	var outPath string
	err := withImageDecodePermit(ctx, func() error {
		img, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
		if err != nil {
			return fmt.Errorf("media: open for thumb: %w", err)
		}

		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		var targetW, targetH int
		if w >= h {
			targetW = thumbLongSide
			targetH = int(float64(h) * float64(thumbLongSide) / float64(w))
		} else {
			targetH = thumbLongSide
			targetW = int(float64(w) * float64(thumbLongSide) / float64(h))
		}
		thumb := imaging.Resize(img, targetW, targetH, imaging.Lanczos)

		out, err := os.CreateTemp("", "media-thumb-*.jpg")
		if err != nil {
			return fmt.Errorf("media: create thumb temp: %w", err)
		}
		outPath = out.Name()
		if err = imaging.Encode(out, thumb, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
			_ = out.Close()
			_ = os.Remove(outPath)
			outPath = ""
			return fmt.Errorf("media: encode thumb: %w", err)
		}
		if err = out.Close(); err != nil {
			_ = os.Remove(outPath)
			outPath = ""
			return fmt.Errorf("media: close thumb temp: %w", err)
		}
		return nil
	})
	return outPath, err
}
