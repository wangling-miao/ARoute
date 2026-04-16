package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strings"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	_ "image/gif"
	_ "image/png"
)

var supportedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/bmp":  true,
	"image/webp": true,
}

func isImageMIME(mimeType string) bool {
	if !strings.HasPrefix(mimeType, "image/") {
		return false
	}
	return supportedImageTypes[mimeType]
}

func decodeImageConfig(data []byte) (int, int, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("decode image config: %w", err)
	}
	return cfg.Width, cfg.Height, nil
}

func generateThumbnail(data []byte, targetWidth, targetHeight int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image for thumbnail: %w", err)
	}

	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	if srcW <= 0 || srcH <= 0 {
		return nil, fmt.Errorf("invalid image dimensions: %dx%d", srcW, srcH)
	}

	scaleW := float64(targetWidth) / float64(srcW)
	scaleH := float64(targetHeight) / float64(srcH)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)
	if newW <= 0 {
		newW = 1
	}
	if newH <= 0 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, srcBounds, draw.Src, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode thumbnail jpeg: %w", err)
	}

	return buf.Bytes(), nil
}
