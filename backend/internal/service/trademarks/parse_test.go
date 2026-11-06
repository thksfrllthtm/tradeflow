package trademarks

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// troiss_sample.xls — настоящие строки реестра КГД (Excel сохранил их в .xls): заголовок, нумерация
// колонок, АРАРАТ (текст), 00118 (логотип PNG), исключённая запись, 00565 Gulf (логотип EMF),
// EARPODS и пустая запись 01790.
func TestParseRealRegistrySample(t *testing.T) {
	data, err := os.ReadFile("testdata/troiss_sample.xls")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Excluded != 1 || len(parsed.Skipped) != 1 || !strings.Contains(parsed.Skipped[0], "01790/ТЗ-271123") {
		t.Fatalf("excluded %d, skipped %v", parsed.Excluded, parsed.Skipped)
	}
	byReg := map[string]Entry{}
	for _, entry := range parsed.Entries {
		byReg[entry.RegNo] = entry
	}
	if len(byReg) != 4 {
		t.Fatalf("entries %v", byReg)
	}
	ararat := byReg["00001/ТЗ-260905"]
	if ararat.Name != "АРАРАТ" || strings.Join(ararat.Codes, ",") != "220820" || ararat.ProtectedUntil == nil || ararat.ProtectedUntil.Format("2006-01-02") != "2027-04-19" || ararat.Logo != "" {
		t.Fatalf("АРАРАТ %+v", ararat)
	}
	if !strings.Contains(byReg["01595/ТЗ-260822"].Holder, "Apple Inc.") || byReg["01595/ТЗ-260822"].Name != "EARPODS" {
		t.Fatalf("EARPODS %+v", byReg["01595/ТЗ-260822"])
	}
	for _, regNo := range []string{"00118/ТЗ-031210", "00565/ТЗ-251115"} {
		entry := byReg[regNo]
		if entry.Name != "" || !strings.HasPrefix(entry.Logo, "data:image/png;base64,") {
			t.Fatalf("%s: name %q, logo %.40q", regNo, entry.Name, entry.Logo)
		}
	}
	if parsed.LogoEntries != 2 || len(parsed.BadLogos) != 0 {
		t.Fatalf("logos %d, bad %v", parsed.LogoEntries, parsed.BadLogos)
	}
}

func TestParseXLSXWithLogo(t *testing.T) {
	book := excelize.NewFile()
	sheet := book.GetSheetName(0)
	rows := [][]any{
		{"Таможенный реестр объектов интеллектуальной собственности"},
		{"Регистрационный номер по таможенному реестру ОИС", "Наименование (вид, описание, изображение) объекта", "Наименование товаров, класс товаров по МКТУ/ код товаров по ТН ВЭД", "Сведения о правообладателе", "Название, номер и дата охранного документа", "Срок защиты", "Сведения о доверенных лицах"},
		{"00010/ТЗ-010101", "", "Наушники / 9 / 851830, 851810", "Sony Group Corporation", "Свидетельство №1", "19.07.2028", "ТОО «Пример»"},
		{"Исключён приказом № 1 от 01.01.2020 г.", ""},
	}
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := book.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	logo := image.NewRGBA(image.Rect(0, 0, 40, 20))
	logo.Set(5, 5, color.Black)
	var picture bytes.Buffer
	_ = png.Encode(&picture, logo)
	if err := book.AddPictureFromBytes(sheet, "B3", &excelize.Picture{Extension: ".png", File: picture.Bytes()}); err != nil {
		t.Fatal(err)
	}
	var file bytes.Buffer
	if err := book.Write(&file); err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(&file, int64(file.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entries) != 1 || parsed.Excluded != 1 {
		t.Fatalf("%+v", parsed)
	}
	entry := parsed.Entries[0]
	if entry.Logo == "" || strings.Join(entry.Codes, ",") != "851830,851810" || entry.ProtectedUntil.Format("2006-01-02") != "2028-07-19" || entry.Representatives != "ТОО «Пример»" {
		t.Fatalf("%+v", entry)
	}
}

// Обрезанный или испорченный файл — ошибка проверки, а не паника сервера.
func TestParseBrokenFileDoesNotPanic(t *testing.T) {
	data, err := os.ReadFile("testdata/troiss_sample.xls")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 512, len(data) / 3, len(data) / 2, len(data) - 100} {
		broken := append([]byte{}, data[:size]...)
		if _, err := Parse(bytes.NewReader(broken), int64(len(broken))); err == nil {
			t.Errorf("size %d: expected error", size)
		}
	}
	for step := 0; step < 200; step++ {
		broken := append([]byte{}, data...)
		for i := 600 + step*7; i < len(broken); i += 997 {
			broken[i] ^= byte(step*31 + 7)
		}
		_, _ = Parse(bytes.NewReader(broken), int64(len(broken)))
	}
}

// troiss_sample2.xls — настоящие строки реестра 2026-10: BIC (в ячейке ещё сплющенная в линию копия),
// Levante (у фигуры поворот 180° с двумя отражениями — на экране как в файле), DENSO (обрезана в Excel).
func TestParseRealSampleCropAndSeveralPictures(t *testing.T) {
	data, err := os.ReadFile("testdata/troiss_sample2.xls")
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := readXLS(data)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(parsed.Entries) != 3 || len(parsed.BadLogos) != 0 {
		t.Fatalf("%v %+v", err, parsed)
	}
	decode := func(logo string) image.Image {
		raw, _ := base64.StdEncoding.DecodeString(logo[strings.Index(logo, ",")+1:])
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	byRow := map[int][]sheetPicture{}
	for _, picture := range sheet.Pictures {
		byRow[picture.Row] = append(byRow[picture.Row], picture)
	}
	// BIC: из двух картинок — логотип 250×78, а не линия.
	if bic := decode(parsed.Entries[0].Logo).Bounds(); bic.Dx() != 250 || bic.Dy() != 78 || len(byRow[2]) != 2 {
		t.Fatalf("BIC %v, pictures %d", bic, len(byRow[2]))
	}
	// Levante: у фигуры поворот 180° и два отражения — Excel показывает картинку как в файле, и логотип тот же.
	ext, raw := emfBitmap(byRow[3][0].Data)
	source, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil || ext == "" {
		t.Fatal(err)
	}
	logo, b := decode(parsed.Entries[1].Logo), source.Bounds()
	for _, point := range [][2]int{{0, 0}, {b.Dx() / 3, b.Dy() / 2}, {83, 96}, {b.Dx() - 5, 7}} {
		r1, g1, b1, _ := logo.At(point[0], point[1]).RGBA()
		r2, g2, b2, _ := source.At(point[0], point[1]).RGBA()
		if r1 != r2 || g1 != g2 || b1 != b2 {
			t.Fatalf("pixel %v changed", point)
		}
	}
	// DENSO: в файле квадрат с полями, в Excel видна только красная полоса.
	if denso := decode(parsed.Entries[2].Logo).Bounds(); float64(denso.Dx())/float64(denso.Dy()) < 2.5 {
		t.Fatalf("DENSO not cropped: %v", denso)
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
