package trademarks

// Чтение старого Excel (.xls, BIFF8): реестр КГД публикуется в нём. «Сохранить как .xlsx» в Excel
// сжимает картинки примерно в 3,5 раза (замер 2026-10-29), а логотипы нужны в исходном качестве — их
// читает ИИ. Читается только нужное реестру: значения ячеек первого листа и картинки с привязкой к ячейке.
// grate (internal/thirdparty, импорт смет из .xls) картинок не отдаёт, поэтому книга читается здесь за один проход.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"strconv"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
)

// sheetPicture — картинка листа: левый верхний угол в ячейке (строка, колонка с 0).
type sheetPicture struct {
	Row, Col int
	// Ext — ".png", ".jpeg", ".bmp", ".tiff", ".emf", ".wmf"; Data — файл картинки.
	Ext  string
	Data []byte
	// Crop — обрезка в Excel, доля сверху, снизу, слева, справа (0 — нет): в ячейке видна только эта часть.
	Crop [4]float64
	// Area — видимый размер в Excel (строки × 256 на колонки × 1024, по привязке); 0 — неизвестен (.xlsx).
	Area int
}

// sheetData — лист: значения ячеек (строки и колонки с 0) и картинки. Числа — как есть (даты — числом Excel).
type sheetData struct {
	Rows     [][]string
	Pictures []sheetPicture
}

const maxColumns = 32

func (s *sheetData) set(row, col int, value string) {
	if row < 0 || row >= maxRows || col < 0 || col >= maxColumns {
		return
	}
	for len(s.Rows) <= row {
		s.Rows = append(s.Rows, nil)
	}
	for len(s.Rows[row]) <= col {
		s.Rows[row] = append(s.Rows[row], "")
	}
	s.Rows[row][col] = value
}

var errNotXLS = errors.New("not an xls workbook")

const (
	recBOF         = 0x0809
	recEOF         = 0x000A
	recContinue    = 0x003C
	recSheet       = 0x0085
	recSST         = 0x00FC
	recLabelSST    = 0x00FD
	recLabel       = 0x0204
	recNumber      = 0x0203
	recRK          = 0x027E
	recMulRK       = 0x00BD
	recFormula     = 0x0006
	recString      = 0x0207
	recDrawGroup   = 0x00EB
	recDrawing     = 0x00EC
	recObj         = 0x005D
	artDgg         = 0xF000
	artBStore      = 0xF001
	artDg          = 0xF002
	artSpgr        = 0xF003
	artSp          = 0xF004
	artFBSE        = 0xF007
	artOPT         = 0xF00B
	artAnchor      = 0xF010
	propPib        = 0x0104
	propCropTop    = 0x0100
	propCropRight  = 0x0103
	artTertiaryOPT = 0xF122
	emrStretchDIB  = 81
	// maxUnpackedBytes — распакованных метафайлов на книгу; maxDepth — вложенность групп фигур.
	maxUnpackedBytes = 300 << 20
	maxDepth         = 32
)

type biffRecord struct {
	id   uint16
	data []byte
}

// readXLS — первый рабочий лист книги .xls.
func readXLS(file []byte) (sheetData, error) {
	doc, err := mscfb.New(bytes.NewReader(file))
	if err != nil {
		return sheetData{}, errNotXLS
	}
	var stream []byte
	for entry, nextErr := doc.Next(); nextErr == nil; entry, nextErr = doc.Next() {
		if entry.Name == "Workbook" || entry.Name == "Book" {
			if stream, err = io.ReadAll(entry); err != nil {
				return sheetData{}, errNotXLS
			}
			break
		}
	}
	if len(stream) == 0 {
		return sheetData{}, errNotXLS
	}
	globals := biffRecords(stream, 0)
	if len(globals) == 0 || globals[0].id != recBOF {
		return sheetData{}, errNotXLS
	}
	sheetAt := -1
	var sst, group [][]byte
	inSST, inGroup := false, false
	for _, record := range globals {
		switch {
		case record.id == recSheet && len(record.data) >= 6 && sheetAt < 0 && record.data[5] == 0:
			sheetAt = int(binary.LittleEndian.Uint32(record.data))
		case record.id == recSST:
			sst, inSST, inGroup = [][]byte{record.data}, true, false
			continue
		case record.id == recDrawGroup:
			group, inGroup, inSST = append(group, record.data), true, false
			continue
		case record.id == recContinue && inSST:
			sst = append(sst, record.data)
			continue
		case record.id == recContinue && inGroup:
			group = append(group, record.data)
			continue
		}
		inSST, inGroup = false, false
	}
	if sheetAt < 0 || sheetAt >= len(stream) {
		return sheetData{}, errNotXLS
	}
	strings := sharedStrings(sst)
	blips := blipStore(bytes.Join(group, nil))

	var sheet sheetData
	var drawing [][]byte
	// Данные рисунков листа идут в MSODRAWING и в CONTINUE после него и после OBJ (фигура за фигурой);
	// CONTINUE после TXO — текст надписи, не рисунок.
	lastMain := uint16(0)
	pendingRow, pendingCol := -1, -1
	depth := 0
	for _, record := range biffRecords(stream, sheetAt) {
		switch record.id {
		case recBOF:
			depth++
		case recEOF:
			depth--
		}
		if depth > 1 {
			continue // вложенные диаграммы
		}
		data := record.data
		if record.id != recContinue {
			lastMain = record.id
		}
		if record.id == recDrawing || (record.id == recContinue && (lastMain == recDrawing || lastMain == recObj)) {
			drawing = append(drawing, data)
			continue
		}
		if len(data) < 6 && record.id != recString {
			continue
		}
		var row, col int
		if len(data) >= 4 {
			row, col = int(binary.LittleEndian.Uint16(data)), int(binary.LittleEndian.Uint16(data[2:]))
		}
		switch record.id {
		case recLabelSST:
			if len(data) >= 10 {
				if index := int(binary.LittleEndian.Uint32(data[6:])); index < len(strings) {
					sheet.set(row, col, strings[index])
				}
			}
		case recLabel:
			reader := &segmentReader{segments: [][]byte{data[6:]}}
			sheet.set(row, col, reader.unicodeString(false))
		case recNumber:
			if len(data) >= 14 {
				sheet.set(row, col, formatNumber(math.Float64frombits(binary.LittleEndian.Uint64(data[6:]))))
			}
		case recRK:
			if len(data) >= 10 {
				sheet.set(row, col, formatNumber(rkValue(binary.LittleEndian.Uint32(data[6:]))))
			}
		case recMulRK:
			for offset, column := 4, col; offset+6 <= len(data)-2; offset, column = offset+6, column+1 {
				sheet.set(row, column, formatNumber(rkValue(binary.LittleEndian.Uint32(data[offset+2:]))))
			}
		case recFormula:
			if len(data) >= 14 {
				value := data[6:14]
				if value[6] == 0xFF && value[7] == 0xFF {
					if value[0] == 0 {
						pendingRow, pendingCol = row, col
					}
				} else {
					sheet.set(row, col, formatNumber(math.Float64frombits(binary.LittleEndian.Uint64(value))))
				}
			}
		case recString:
			if pendingRow >= 0 {
				reader := &segmentReader{segments: [][]byte{data}}
				sheet.set(pendingRow, pendingCol, reader.unicodeString(false))
				pendingRow = -1
			}
		}
	}
	sheet.Pictures = sheetPictures(bytes.Join(drawing, nil), blips)
	return sheet, nil
}

func biffRecords(stream []byte, offset int) []biffRecord {
	records := []biffRecord{}
	depth := 0
	for offset+4 <= len(stream) {
		id, size := binary.LittleEndian.Uint16(stream[offset:]), int(binary.LittleEndian.Uint16(stream[offset+2:]))
		if offset+4+size > len(stream) {
			break
		}
		records = append(records, biffRecord{id: id, data: stream[offset+4 : offset+4+size]})
		offset += 4 + size
		switch id {
		case recBOF:
			depth++
		case recEOF:
			if depth--; depth <= 0 {
				return records
			}
		}
	}
	return records
}

func rkValue(rk uint32) float64 {
	var value float64
	if rk&2 != 0 {
		value = float64(int32(rk) >> 2)
	} else {
		value = math.Float64frombits(uint64(rk&0xFFFFFFFC) << 32)
	}
	if rk&1 != 0 {
		value /= 100
	}
	return value
}

func formatNumber(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

// segmentReader читает данные записи с продолжениями (CONTINUE). Символы строки, разорванные границей
// записи, продолжаются с нового байта флагов (1 или 2 байта на символ).
type segmentReader struct {
	segments [][]byte
	index    int
	pos      int
}

func (r *segmentReader) skipEmpty() bool {
	for r.index < len(r.segments) && r.pos >= len(r.segments[r.index]) {
		r.index++
		r.pos = 0
	}
	return r.index < len(r.segments)
}

// read — n байт (n из заголовков, до 4). Длина не доверяется: память — по мере чтения.
func (r *segmentReader) read(n int) ([]byte, bool) {
	out := make([]byte, 0, min(n, 16))
	for len(out) < n {
		if !r.skipEmpty() {
			return out, false
		}
		segment := r.segments[r.index]
		take := min(n-len(out), len(segment)-r.pos)
		out = append(out, segment[r.pos:r.pos+take]...)
		r.pos += take
	}
	return out, true
}

// skip пропускает n байт без копирования (длины форматирования из файла бывают испорчены — до 4 ГБ).
func (r *segmentReader) skip(n int) {
	for n > 0 && r.skipEmpty() {
		take := min(n, len(r.segments[r.index])-r.pos)
		r.pos += take
		n -= take
	}
}

func (r *segmentReader) uint16() (int, bool) {
	b, ok := r.read(2)
	if !ok {
		return 0, false
	}
	return int(binary.LittleEndian.Uint16(b)), true
}

// unicodeString — XLUnicodeString (rich: XLUnicodeRichExtendedString таблицы строк).
func (r *segmentReader) unicodeString(rich bool) string {
	count, ok := r.uint16()
	if !ok {
		return ""
	}
	flags, ok := r.read(1)
	if !ok {
		return ""
	}
	runs, extra := 0, 0
	if rich && flags[0]&0x08 != 0 {
		if runs, ok = r.uint16(); !ok {
			return ""
		}
	}
	if rich && flags[0]&0x04 != 0 {
		b, ok := r.read(4)
		if !ok {
			return ""
		}
		extra = int(binary.LittleEndian.Uint32(b))
	}
	wide := flags[0]&0x01 != 0
	units := make([]uint16, 0, count)
	for len(units) < count {
		if r.index < len(r.segments) && r.pos >= len(r.segments[r.index]) {
			// Символы продолжаются в следующей записи — с новым байтом флагов.
			b, ok := r.read(1)
			if !ok {
				break
			}
			wide = b[0]&0x01 != 0
			continue // запись могла кончиться на байте флагов — проверить снова
		}
		if r.index >= len(r.segments) {
			break
		}
		segment := r.segments[r.index]
		if wide {
			if r.pos+2 > len(segment) {
				break
			}
			units = append(units, binary.LittleEndian.Uint16(segment[r.pos:]))
			r.pos += 2
		} else {
			units = append(units, uint16(segment[r.pos]))
			r.pos++
		}
	}
	r.skip(runs*4 + extra)
	return string(utf16.Decode(units))
}

// sharedStrings — таблица строк книги (SST и её продолжения).
func sharedStrings(segments [][]byte) []string {
	if len(segments) == 0 || len(segments[0]) < 8 {
		return nil
	}
	unique := int(binary.LittleEndian.Uint32(segments[0][4:]))
	segments[0] = segments[0][8:]
	reader := &segmentReader{segments: segments}
	out := make([]string, 0, min(unique, 1<<20))
	for len(out) < unique && reader.skipEmpty() {
		out = append(out, reader.unicodeString(true))
	}
	return out
}

type artRecord struct {
	inst uint16
	typ  uint16
	data []byte
}

// artRecords — записи OfficeArt одного уровня.
func artRecords(data []byte) []artRecord {
	records := []artRecord{}
	for offset := 0; offset+8 <= len(data); {
		verInst, typ, size := binary.LittleEndian.Uint16(data[offset:]), binary.LittleEndian.Uint16(data[offset+2:]), int(binary.LittleEndian.Uint32(data[offset+4:]))
		end := offset + 8 + size
		if size < 0 || end > len(data) {
			end = len(data)
		}
		records = append(records, artRecord{inst: verInst >> 4, typ: typ, data: data[offset+8 : end]})
		offset = end
	}
	return records
}

type blip struct {
	ext  string
	data []byte
	// unpacked — data распакована из zlib (считается в общий лимит книги).
	unpacked bool
}

// blipStore — картинки книги по порядку (номер картинки у фигуры — с 1).
func blipStore(group []byte) []blip {
	store := []blip{}
	// Распакованные метафайлы — не больше maxUnpackedBytes на всю книгу (размер в заголовке не доверяется).
	budget := maxUnpackedBytes
	for _, dgg := range artRecords(group) {
		if dgg.typ != artDgg {
			continue
		}
		for _, container := range artRecords(dgg.data) {
			if container.typ != artBStore {
				continue
			}
			for _, entry := range artRecords(container.data) {
				item := blip{}
				if entry.typ == artFBSE && len(entry.data) >= 36 {
					if start := 36 + int(entry.data[33]); start < len(entry.data) {
						if records := artRecords(entry.data[start:]); len(records) > 0 {
							item = decodeBlip(records[0], budget)
							if item.unpacked {
								budget -= len(item.data)
							}
						}
					}
				}
				store = append(store, item)
			}
		}
	}
	return store
}

func decodeBlip(record artRecord, budget int) blip {
	uid := 16
	if record.inst&1 == 1 {
		uid = 32
	}
	if len(record.data) < uid+1 {
		return blip{}
	}
	switch record.typ {
	case 0xF01A, 0xF01B: // EMF, WMF: заголовок метафайла 34 байта, тело обычно сжато (zlib)
		header := record.data[uid:]
		if len(header) < 34 {
			return blip{}
		}
		size, saved := int(binary.LittleEndian.Uint32(header)), int(binary.LittleEndian.Uint32(header[28:]))
		body := header[34:min(len(header), 34+max(saved, 0))]
		unpacked := false
		if header[32] == 0 {
			reader, err := zlib.NewReader(bytes.NewReader(body))
			if err != nil || budget <= 0 {
				return blip{}
			}
			if body, err = io.ReadAll(io.LimitReader(reader, int64(min(max(size, 0), maxFileBytes, budget)))); err != nil && len(body) == 0 {
				return blip{}
			}
			unpacked = true
		}
		ext := ".emf"
		if record.typ == 0xF01B {
			ext = ".wmf"
		}
		return blip{ext: ext, data: body, unpacked: unpacked}
	case 0xF01D, 0xF02A:
		return blip{ext: ".jpeg", data: record.data[uid+1:]}
	case 0xF01E:
		return blip{ext: ".png", data: record.data[uid+1:]}
	case 0xF01F:
		return blip{ext: ".bmp", data: dibToBMP(record.data[uid+1:], nil)}
	case 0xF029:
		return blip{ext: ".tiff", data: record.data[uid+1:]}
	}
	return blip{}
}

// sheetPictures — фигуры листа с картинкой и привязкой к ячейке.
func sheetPictures(drawing []byte, blips []blip) []sheetPicture {
	pictures := []sheetPicture{}
	var walk func(data []byte, depth int)
	walk = func(data []byte, depth int) {
		if depth > maxDepth {
			return
		}
		for _, record := range artRecords(data) {
			switch record.typ {
			case artDg, artSpgr:
				walk(record.data, depth+1)
			case artSp:
				pib, row, col, area := 0, -1, -1, 0
				var crop [4]float64
				// Поворот и отражение фигуры не применяются: на реестре 2026-10 Excel показывает картинку так,
				// как она лежит в файле (поворот 90° без отражений и 180° с двумя отражениями — сверено снимком листа).
				for _, part := range artRecords(record.data) {
					switch {
					case part.typ == artOPT || part.typ == artTertiaryOPT:
						for offset := 0; offset+6 <= len(part.data) && offset < int(part.inst)*6; offset += 6 {
							id, value := binary.LittleEndian.Uint16(part.data[offset:])&0x3FFF, binary.LittleEndian.Uint32(part.data[offset+2:])
							switch {
							case id == propPib:
								pib = int(value)
							case id >= propCropTop && id <= propCropRight:
								// 16.16 — доля картинки; отрицательная (поле вокруг) не обрезает.
								crop[id-propCropTop] = min(max(float64(int32(value))/65536, 0), 0.95)
							}
						}
					case part.typ == artAnchor && len(part.data) >= 18:
						col, row = int(binary.LittleEndian.Uint16(part.data[2:])), int(binary.LittleEndian.Uint16(part.data[6:]))
						area = anchorArea(part.data)
					}
				}
				if pib >= 1 && pib <= len(blips) && row >= 0 && len(blips[pib-1].data) > 0 {
					pictures = append(pictures, sheetPicture{Row: row, Col: col, Ext: blips[pib-1].ext, Data: blips[pib-1].data, Crop: crop, Area: area})
				}
			}
		}
	}
	walk(drawing, 0)
	return pictures
}

// dibToBMP — файл BMP из BITMAPINFO и пикселей (bits nil — пиксели сразу за заголовком).
func dibToBMP(info, bits []byte) []byte {
	if len(info) < 16 {
		return nil
	}
	headerSize := int(binary.LittleEndian.Uint32(info))
	if headerSize < 12 || headerSize > len(info) {
		return nil
	}
	pixelsAt := len(info)
	if bits == nil {
		pixelsAt = headerSize
		if headerSize >= 40 {
			bitCount, compression, used := int(binary.LittleEndian.Uint16(info[14:])), binary.LittleEndian.Uint32(info[16:]), int(binary.LittleEndian.Uint32(info[32:]))
			if bitCount <= 8 {
				if used == 0 {
					used = 1 << bitCount
				}
				pixelsAt += 4 * used
			}
			if compression == 3 && headerSize == 40 {
				pixelsAt += 12
			}
		} else if bitCount := int(binary.LittleEndian.Uint16(info[10:])); bitCount <= 8 {
			pixelsAt += 3 << bitCount // BITMAPCOREHEADER: палитра по 3 байта
		}
		pixelsAt = min(pixelsAt, len(info))
	}
	body := append(append([]byte{}, info...), bits...)
	out := make([]byte, 14, 14+len(body))
	out[0], out[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(out[2:], uint32(14+len(body)))
	binary.LittleEndian.PutUint32(out[10:], uint32(14+pixelsAt))
	return append(out, body...)
}

// emfBitmap — растровая картинка из EMF: Office хранит вставленный логотип как один EMR_STRETCHDIBITS.
// Ext "" — в EMF нет растровой картинки (векторный рисунок не читается).
func emfBitmap(data []byte) (string, []byte) {
	for offset := 0; offset+8 <= len(data); {
		typ, size := binary.LittleEndian.Uint32(data[offset:]), int(binary.LittleEndian.Uint32(data[offset+4:]))
		if size < 8 || offset+size > len(data) {
			break
		}
		record := data[offset : offset+size]
		if typ == emrStretchDIB && len(record) >= 64 {
			infoAt, infoSize := int(binary.LittleEndian.Uint32(record[48:])), int(binary.LittleEndian.Uint32(record[52:]))
			bitsAt, bitsSize := int(binary.LittleEndian.Uint32(record[56:])), int(binary.LittleEndian.Uint32(record[60:]))
			if infoAt+infoSize <= len(record) && bitsAt+bitsSize <= len(record) && infoSize >= 40 {
				info, bits := record[infoAt:infoAt+infoSize], record[bitsAt:bitsAt+bitsSize]
				switch binary.LittleEndian.Uint32(info[16:]) {
				case 4:
					return ".jpeg", bits
				case 5:
					return ".png", bits
				}
				return ".bmp", dibToBMP(info, bits)
			}
		}
		offset += size
	}
	return "", nil
}

// anchorArea — видимый размер фигуры по привязке ClientAnchor: (строка × 256 + dy) и (колонка × 1024 + dx)
// от угла до угла. Линия высотой в доли пункта — почти 0.
func anchorArea(anchor []byte) int {
	value := func(at int) int { return int(binary.LittleEndian.Uint16(anchor[at:])) }
	width := value(10)*1024 + value(12) - value(2)*1024 - value(4)
	height := value(14)*256 + value(16) - value(6)*256 - value(8)
	return max(width, 0) * max(height, 0)
}
