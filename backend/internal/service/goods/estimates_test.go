package goods

import (
	"encoding/base64"
	"testing"
)

func TestValidateGoodsPhotoData(t *testing.T) {
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))
	if err := validateGoodsPhotoData(valid); err != nil {
		t.Fatalf("valid photo rejected: %v", err)
	}
	if err := validateGoodsPhotoData("data:image/gif;base64," + base64.StdEncoding.EncodeToString([]byte("gif"))); err == nil {
		t.Fatal("unsupported image type should be rejected")
	}
	if err := validateGoodsPhotoData("data:image/png;base64,not-base64"); err == nil {
		t.Fatal("invalid base64 should be rejected")
	}
}
