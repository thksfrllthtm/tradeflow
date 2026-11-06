package trademarks

// Картинки листа .xls: хранилище картинок книги (OfficeArt), фигуры листа с привязкой к ячейке, BMP и EMF.

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"io"
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
