package trademarks

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"testing"
)

// Испорченная длина форматирования (4 ГБ) не выделяет память, CONTINUE из одного байта флагов не паникует.
func TestSharedStringsSurviveBrokenLengths(t *testing.T) {
	header := make([]byte, 8)
	binary.LittleEndian.PutUint32(header[4:], 2)
	// Строка «AB» с флагом ext и длиной ext 0xFFFFFFFF.
	first := append(header, 2, 0, 0x04, 0xFF, 0xFF, 0xFF, 0xFF, 'A', 'B')
	if got := sharedStrings([][]byte{first}); len(got) != 1 || got[0] != "AB" {
		t.Fatalf("%q", got)
	}
	// «ABC»: «A» в SST, продолжение — только байт флагов, затем «BC».
	second := append(append([]byte{}, header...), 3, 0, 0, 'A')
	if got := sharedStrings([][]byte{second, {0}, {0, 'B', 'C'}}); len(got) == 0 || got[0] != "ABC" {
		t.Fatalf("%q", got)
	}
}

// Картинка с заголовком «60000×60000» не распаковывается.
func TestLogoThumbRejectsHugeDimensions(t *testing.T) {
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, image.NewGray(image.Rect(0, 0, 1, 1)))
	raw := buffer.Bytes()
	binary.BigEndian.PutUint32(raw[16:], 60000) // ширина и высота в IHDR
	binary.BigEndian.PutUint32(raw[20:], 60000)
	if logo := logoThumb(sheetPicture{Ext: ".png", Data: raw}); logo != "" {
		t.Fatal("huge image accepted")
	}
}

func TestParseUntilAndSpaces(t *testing.T) {
	if date := parseUntil("с 01.01.2018 по 19.07.2028"); date == nil || date.Format("2006-01-02") != "2028-07-19" {
		t.Fatalf("%v", date)
	}
	if date := parseUntil("31.06.2027"); date != nil {
		t.Fatalf("31 июня: %v", date)
	}
	if got := spaces.ReplaceAllString("00001 /ТЗ-260905", ""); got != "00001/ТЗ-260905" {
		t.Fatalf("%q", got)
	}
}
