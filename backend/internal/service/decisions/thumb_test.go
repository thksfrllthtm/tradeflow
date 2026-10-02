package decisions

import (
	"bytes"
	"image"
	"testing"
)

func TestMakeThumbShrinksPhotoForList(t *testing.T) {
	for _, asJPEG := range []bool{true, false} {
		thumb := MakeThumb(encodeTestImage(t, 900, 450, asJPEG, false))
		if !ValidPhoto(thumb, maxDecisionThumbBytes) {
			t.Fatalf("thumb is not a valid small photo (jpeg source %v): %d bytes", asJPEG, len(thumb))
		}
		raw, _, _ := decodeDataURL(thumb)
		config, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil || config.Width != thumbSize || config.Height != thumbSize/2 {
			t.Fatalf("thumb size %dx%d, %v", config.Width, config.Height, err)
		}
	}
	if MakeThumb("data:image/png;base64,???") != "" {
		t.Fatal("broken photo gives no thumb")
	}
}
