package decisions

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func encodeTestImage(t *testing.T, width, height int, asJPEG bool, invert bool) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Узор из полос и градиента — у снимка есть «содержание».
			value := uint8((x*255/width + (y/(height/6+1))*40) % 256)
			if (x/(width/5+1))%2 == 0 {
				value = 255 - value
			}
			if invert {
				value = 255 - value
			}
			img.Set(x, y, color.RGBA{value, value / 2, 255 - value, 255})
		}
	}
	var buffer bytes.Buffer
	mime := "image/png"
	if asJPEG {
		mime = "image/jpeg"
		_ = jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 60})
	} else {
		_ = png.Encode(&buffer, img)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func TestPhotoHashFindsSamePhotoAfterResize(t *testing.T) {
	original := PhotoHash(encodeTestImage(t, 640, 480, false, false))
	resized := PhotoHash(encodeTestImage(t, 320, 240, true, false))
	other := PhotoHash(encodeTestImage(t, 640, 480, false, true))
	if original == "" || resized == "" || other == "" {
		t.Fatalf("hashes: %q %q %q", original, resized, other)
	}
	if !samePhoto([]string{original}, []string{resized}) {
		t.Fatalf("resized JPEG must match: distance %d", photoHashDistance(original, resized))
	}
	if samePhoto([]string{original}, []string{other}) {
		t.Fatalf("different photo must not match: distance %d", photoHashDistance(original, other))
	}
	if PhotoHash("data:image/png;base64,???") != "" || photoHashDistance("0000000000000000", "0000000000000000") != -1 {
		t.Fatal("broken image or blank hash must not match")
	}
}
