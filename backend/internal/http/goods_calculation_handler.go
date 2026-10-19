package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/goods"
	"github.com/gin-gonic/gin"
)

func (s *Server) parseGoodsXlsx(c *gin.Context) {
	// Multipart headers and boundaries add overhead on top of the documented
	// file limit; leave a small fixed envelope without making the endpoint
	// unbounded.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, goods.MaxGoodsXLSXBytes+(1<<20))
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, fmt.Sprintf("Выберите XLSX-файл размером до %d МБ", goods.MaxGoodsXLSXBytes>>20), "file")
		return
	}
	defer file.Close()

	if header.Size > goods.MaxGoodsXLSXBytes {
		badRequest(c, fmt.Sprintf("XLSX-файл больше %d МБ не поддерживается", goods.MaxGoodsXLSXBytes>>20), "file")
		return
	}
	if name := strings.ToLower(header.Filename); !strings.HasSuffix(name, ".xlsx") && !strings.HasSuffix(name, ".xls") {
		badRequest(c, "Поддерживаются только файлы Excel .xlsx и .xls", "file")
		return
	}

	sheet := c.Query("sheet")
	headerRow, _ := strconv.Atoi(c.DefaultQuery("headerRow", "1"))
	dataStartRow, _ := strconv.Atoi(c.DefaultQuery("dataStartRow", "2"))

	var result goods.GoodsParseResult
	if c.Query("shareImages") == "true" {
		result, err = s.goods.ParseXlsxWithSharedImages(c.Request.Context(), file, header.Size, header.Filename, sheet, headerRow, dataStartRow)
	} else {
		result, err = s.goods.ParseXlsx(c.Request.Context(), file, header.Size, header.Filename, sheet, headerRow, dataStartRow)
	}
	if err != nil {
		var valErr *service.ValidationError
		if errors.As(err, &valErr) {
			badRequest(c, valErr.Message, valErr.Field)
		} else {
			badRequest(c, err.Error(), "file")
		}
		return
	}

	// Photos are streamed into the response instead of c.JSON building the
	// whole (possibly 100+ MB) body in memory.
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(http.StatusOK)
	if err := result.WriteJSON(c.Writer); err != nil {
		_ = c.Error(err)
	}
}

func (s *Server) calculateGoods(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	var input goods.GoodsCalculationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные расчёта", "")
		return
	}

	result, err := s.goods.Calculate(c.Request.Context(), input)
	if err != nil {
		var valErr *service.ValidationError
		if errors.As(err, &valErr) {
			badRequest(c, valErr.Message, valErr.Field)
		} else {
			internalError(c)
		}
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) exportGoods(c *gin.Context) {
	// Rows carry their first photo (compressed JPG/PNG) for the XLSX file.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<20)
	var input goods.GoodsCalculationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные расчёта", "")
		return
	}

	buffer, err := s.goods.Export(c.Request.Context(), input)
	if err != nil {
		var valErr *service.ValidationError
		if errors.As(err, &valErr) {
			badRequest(c, valErr.Message, valErr.Field)
		} else {
			internalError(c)
		}
		return
	}

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="goods-calculation.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}
