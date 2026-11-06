package trademarks

import (
	"encoding/binary"
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
