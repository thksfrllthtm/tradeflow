package xls

import (
	"fmt"
	"math"
)

type header struct {
	Version  uint16 // An unsigned integer that specifies the BIFF version of the file. The value MUST be 0x0600.
	DocType  uint16 //An unsigned integer that specifies the document type of the substream of records following this record. For more information about the layout of the sub-streams in the workbook stream see File Structure.
	RupBuild uint16 // An unsigned integer that specifies the build identifier.
	RupYear  uint16 // An unsigned integer that specifies the year when this BIFF version was first created. The value MUST be 0x07CC or 0x07CD.
	MiscBits uint64 // lots of miscellaneous bits and flags we're not going to check
}

// 2.1.4
type rec struct {
	RecType recordType //
	RecSize uint16     // must be between 0 and 8224
	Data    []byte     // len(rec.data) = rec.recsize
}

type boundSheet struct {
	Position    uint32 // A FilePointer as specified in [MS-OSHARED] section 2.2.1.5 that specifies the stream position of the start of the BOF record for the sheet.
	HiddenState byte   // (2 bits) An unsigned integer that specifies the hidden state of the sheet. MUST be a value from the following table:
	SheetType   byte   // An unsigned integer that specifies the sheet type. 00=worksheet
	Name        string
}

// /////
type shRow struct {
	RowIndex uint16 // 0-based
	FirstCol uint16 // 0-based
	LastCol  uint16 // 1-based!
	Height   uint16
	Reserved uint32
	Flags    uint32
}

type shRef8 struct {
	FirstRow uint16 // 0-based
	LastRow  uint16 // 0-based
	FirstCol uint16 // 0-based
	LastCol  uint16 // 0-based
}
type shMulRK struct {
	RowIndex uint16 // 0-based
	FirstCol uint16 // 0-based
	Values   []RkRec
	LastCol  uint16 // 0-based?
}
type RkRec struct {
	IXFCell uint16
	Value   RKNumber
}

type shRK struct {
	RowIndex uint16 // 0-based
	Col      uint16 // 0-based
	IXFCell  uint16
	Value    RKNumber
}

type RKNumber uint32

func (r RKNumber) IsInteger() bool {
	if (r & 1) != 0 {
		// has 2 decimals
		return false
	}
	if (r & 2) == 0 {
		// is part of a float
		return false
	}
	return true
}

func (r RKNumber) Int() int {
	val := int32(r) >> 2
	if (r&1) == 0 && (r&2) != 0 {
		return int(val)
	}
	if (r&1) != 0 && (r&2) != 0 {
		return int(val / 100)
	}
	return 0
}

// Float64 — значение RK любого вида (MS-XLS 2.5.217): бит 0 — число хранится умноженным на 100,
// бит 1 — в старших 30 битах целое, иначе старшие 30 бит IEEE double. Раньше для целого ×100
// (так Excel хранит, например, 11568.42 = 1156842 / 100) возвращался 0.
func (r RKNumber) Float64() float64 {
	val := int32(r) >> 2
	var value float64
	if (r & 2) != 0 {
		value = float64(val)
	} else {
		value = math.Float64frombits(uint64(val) << 34)
	}
	if (r & 1) != 0 {
		return value / 100.0
	}
	return value
}

func (r RKNumber) String() string {
	if r.IsInteger() {
		return fmt.Sprint(r.Int())
	}
	return fmt.Sprint(r.Float64())
}
