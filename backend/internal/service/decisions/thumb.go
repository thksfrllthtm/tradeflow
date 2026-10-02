package decisions

// Миниатюра записи для списка базы решений. Импорт и ручное добавление присылают её из браузера;
// если её нет (запись из подбора ИИ, старые записи), сервер уменьшает первое фото сам.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"log"
)

const (
	thumbSize       = 160
	thumbQuality    = 70
	thumbFillBatch  = 50
	thumbDataPrefix = "data:image/jpeg;base64,"
)

// MakeThumb — JPEG не больше thumbSize по большей стороне; "" — фото не удалось прочитать.
func MakeThumb(dataURL string) string {
	raw, _, ok := decodeDataURL(dataURL)
	if !ok {
		return ""
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return ""
	}
	scale := min(1, float64(thumbSize)/float64(max(width, height)))
	outW, outH := max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))
	out := image.NewRGBA(image.Rect(0, 0, outW, outH))
	for y := 0; y < outH; y++ {
		y0, y1 := bounds.Min.Y+y*height/outH, bounds.Min.Y+(y+1)*height/outH
		for x := 0; x < outW; x++ {
			x0, x1 := bounds.Min.X+x*width/outW, bounds.Min.X+(x+1)*width/outW
			// Среднее по клетке исходного фото; прозрачное — на белом фоне.
			var r, g, b, n uint64
			for sy := y0; sy < max(y1, y0+1); sy++ {
				for sx := x0; sx < max(x1, x0+1); sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					white := uint64(0xffff - ca)
					r, g, b, n = r+uint64(cr)+white, g+uint64(cg)+white, b+uint64(cb)+white, n+1
				}
			}
			out.Set(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(b / n >> 8), 0xff})
		}
	}
	var buffer bytes.Buffer
	if jpeg.Encode(&buffer, out, &jpeg.Options{Quality: thumbQuality}) != nil {
		return ""
	}
	return thumbDataPrefix + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

// FillMissingThumbs проставляет миниатюры записям с фото, у которых её нет (один проход при старте).
func (s *CodeDecisionService) FillMissingThumbs(ctx context.Context) {
	var after int64
	filled := 0
	for ctx.Err() == nil {
		rows, err := s.repository.DecisionsMissingThumb(ctx, after, thumbFillBatch)
		if err != nil || len(rows) == 0 {
			break
		}
		for _, row := range rows {
			after = row.ID
			var photos []string
			if json.Unmarshal(row.Photos, &photos) != nil || len(photos) == 0 {
				continue
			}
			if thumb := MakeThumb(photos[0]); thumb != "" && s.repository.SaveDecisionThumb(ctx, row.ID, thumb) == nil {
				filled++
			}
		}
	}
	if filled > 0 {
		log.Printf("decision thumbs: filled %d", filled)
	}
}
