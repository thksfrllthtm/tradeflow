package decisions

// Перцептивный хеш фото (dHash, 64 бита): одно и то же фото после пересжатия или уменьшения даёт
// почти тот же хеш. По нему база решений узнаёт товар, фото которого поставщик уже присылал.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/bits"
	"strconv"
	"strings"
)

// Фото с отличием хеша не больше этого числа бит считаются одним и тем же снимком.
const samePhotoMaxDistance = 6

// decodeDataURL возвращает байты картинки из «data:image/...;base64,...».
func decodeDataURL(value string) ([]byte, string, bool) {
	header, data, ok := strings.Cut(value, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
		return nil, "", false
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", false
	}
	return raw, strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64"), true
}

// PhotoHash — dHash фото в виде 16 hex-символов; "" — картинку не удалось прочитать.
func PhotoHash(dataURL string) string {
	raw, _, ok := decodeDataURL(dataURL)
	if !ok {
		return ""
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%016x", dHash(img))
}

// dHash: картинка сжимается до 9×8 оттенков серого, каждый бит — светлее ли пиксель соседа справа.
func dHash(img image.Image) uint64 {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return 0
	}
	var gray [8][9]float64
	for y := 0; y < 8; y++ {
		for x := 0; x < 9; x++ {
			// Среднее по клетке исходной картинки — устойчивее, чем один пиксель.
			x0, x1 := bounds.Min.X+x*width/9, bounds.Min.X+(x+1)*width/9
			y0, y1 := bounds.Min.Y+y*height/8, bounds.Min.Y+(y+1)*height/8
			if x1 <= x0 {
				x1 = x0 + 1
			}
			if y1 <= y0 {
				y1 = y0 + 1
			}
			stepX, stepY := max(1, (x1-x0)/8), max(1, (y1-y0)/8)
			var sum float64
			var count int
			for py := y0; py < y1; py += stepY {
				for px := x0; px < x1; px += stepX {
					r, g, b, _ := img.At(px, py).RGBA()
					sum += 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
					count++
				}
			}
			gray[y][x] = sum / float64(count)
		}
	}
	var hash uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			hash <<= 1
			if gray[y][x] > gray[y][x+1] {
				hash |= 1
			}
		}
	}
	return hash
}

// photoHashDistance — число различающихся бит; -1 — хеш не читается.
func photoHashDistance(a, b string) int {
	left, err1 := strconv.ParseUint(a, 16, 64)
	right, err2 := strconv.ParseUint(b, 16, 64)
	if err1 != nil || err2 != nil || a == "" || b == "" {
		return -1
	}
	// Почти однотонная картинка (пустой фон, заглушка) даёт хеш из нулей или единиц — такие
	// снимки совпадали бы между собой, хотя товары разные.
	for _, value := range []uint64{left, right} {
		if ones := bits.OnesCount64(value); ones <= 3 || ones >= 61 {
			return -1
		}
	}
	return bits.OnesCount64(left ^ right)
}

// samePhoto — хотя бы одна пара фото совпадает.
func samePhoto(hashes, other []string) bool {
	for _, left := range hashes {
		for _, right := range other {
			if distance := photoHashDistance(left, right); distance >= 0 && distance <= samePhotoMaxDistance {
				return true
			}
		}
	}
	return false
}
