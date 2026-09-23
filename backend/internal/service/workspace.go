package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/xuri/excelize/v2"
)

// StandardProductColumns is the single source of truth for the order used by
// the normalized table. Keep this list independent from source Excel order.
var StandardProductColumns = []string{
	"number", "name", "grossWeight", "netWeight", "quantity", "quantityUnit", "pairs", "places", "pairsPerPlace",
	"unitPrice", "currency", "lineTotal", "sku", "translation", "material",
	"characteristics", "photo", "additional",
}

var workspaceFields = map[string]bool{
	"unassigned": true, "unused": true, "name": true, "translation": true,
	"sku": true, "quantity": true, "quantityUnit": true, "unitPrice": true,
	"lineTotal": true, "currency": true, "grossWeight": true, "netWeight": true, "pairs": true, "places": true, "pairsPerPlace": true,
	"material": true, "characteristics": true, "photo": true, "additional": true,
}

type WorkspaceColumn struct {
	ID          string `json:"id"`
	SourceIndex int    `json:"sourceIndex"`
	SourceName  string `json:"sourceName"`
	Field       string `json:"field"`
	ExtraName   string `json:"extraName"`
	WeightUnit  string `json:"weightUnit"`
	WeightBasis string `json:"weightBasis"`
}

type WorkspaceRow struct {
	ID             string            `json:"id"`
	SourceRow      int               `json:"sourceRow"`
	Values         map[string]string `json:"values"`
	OriginalValues map[string]string `json:"originalValues"`
	Formulas       map[string]string `json:"formulas"`
	Included       bool              `json:"included"`
	Warnings       []string          `json:"warnings"`
}

type DeletedColumn struct {
	Column WorkspaceColumn `json:"column"`
	Index  int             `json:"index"`
}

type WorkspaceBatch struct {
	GrossWeight             string `json:"grossWeight"`
	GrossWeightUnit         string `json:"grossWeightUnit"`
	GrossWeightSourceRowID  string `json:"grossWeightSourceRowId"`
	GrossWeightSourceColumn string `json:"grossWeightSourceColumn"`
	Currency                string `json:"currency"`
}

type WorkspaceImage struct {
	AssetID  int64  `json:"assetId"`
	RowID    string `json:"rowId"`
	MIMEType string `json:"mimeType"`
	FileSize int64  `json:"fileSize"`
	Kind     string `json:"kind"`
}

type Workspace struct {
	ImportID        int64             `json:"importId"`
	Sheets          []SheetInfo       `json:"sheets"`
	Sheet           string            `json:"sheet"`
	HeaderRow       int               `json:"headerRow"`
	DataStartRow    int               `json:"dataStartRow"`
	Columns         []WorkspaceColumn `json:"columns"`
	Rows            []WorkspaceRow    `json:"rows"`
	DeletedRows     []WorkspaceRow    `json:"deletedRows"`
	DeletedColumns  []DeletedColumn   `json:"deletedColumns"`
	Batch           WorkspaceBatch    `json:"batch"`
	PriceBasis      string            `json:"priceBasis"`
	PriceUnitLabel  string            `json:"priceUnitLabel"`
	Pictures        []PictureHint     `json:"pictures"`
	Images          []WorkspaceImage  `json:"images"`
	Warnings        []string          `json:"warnings"`
	SavedAt         string            `json:"savedAt"`
	UpdatedBy       int64             `json:"updatedBy"`
	Revision        int               `json:"revision"`
	ReplaceExisting bool              `json:"replaceExisting"`
}

func (s *ImportService) GetWorkspace(ctx context.Context, importID, ownerID int64, sheet string, headerRow, dataStartRow int) (Workspace, error) {
	item, err := s.repository.ImportByIDForOwner(ctx, importID, ownerID)
	if err != nil {
		return Workspace{}, err
	}
	if saved, savedErr := s.repository.WorkspaceByImportForOwner(ctx, importID, ownerID); savedErr == nil {
		workspace, decodeErr := decodeWorkspace(saved, nil)
		if decodeErr != nil {
			return Workspace{}, decodeErr
		}
		if sheet == "" || sheet == workspace.Sheet {
			if (headerRow <= 0 || headerRow == workspace.HeaderRow) && (dataStartRow <= 0 || dataStartRow == workspace.DataStartRow) {
				workspace.Sheets, _ = s.inspectWorkbook(savedPathForWorkspace(item))
				workspace.Images = s.workspaceImages(ctx, importID)
				return workspace, nil
			}
		}
	} else if !errors.Is(savedErr, repository.ErrNotFound) {
		return Workspace{}, savedErr
	}
	return s.initializeWorkspace(item, sheet, headerRow, dataStartRow)
}

func savedPathForWorkspace(item repository.EstimateImport) string { return item.StoredPath }

func (s *ImportService) initializeWorkspace(item repository.EstimateImport, requestedSheet string, headerRow, dataStartRow int) (Workspace, error) {
	file, err := s.files.Open(item.StoredPath)
	if err != nil {
		return Workspace{}, err
	}
	defer file.Close()
	if err := inspectXLSXContainer(file); err != nil {
		return Workspace{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Workspace{}, err
	}
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return Workspace{}, &ValidationError{Field: "file", Message: "Не удалось прочитать сохранённый XLSX-файл"}
	}
	defer workbook.Close()
	sheets := make([]SheetInfo, 0)
	for _, sheet := range workbook.GetSheetList() {
		rows, rowsErr := workbook.GetRows(sheet)
		if rowsErr != nil {
			return Workspace{}, &ValidationError{Field: "file", Message: "Не удалось прочитать один из листов XLSX"}
		}
		sheets = append(sheets, SheetInfo{Name: sheet, Rows: len(rows)})
	}
	if requestedSheet == "" {
		requestedSheet = sheets[0].Name
	}
	rows, err := workbook.GetRows(requestedSheet)
	if err != nil {
		return Workspace{}, &ValidationError{Field: "sheet", Message: "Выбранный лист недоступен"}
	}
	if headerRow < 1 {
		headerRow = 1
	}
	if headerRow > len(rows) {
		return Workspace{}, &ValidationError{Field: "headerRow", Message: "Строка заголовков находится вне листа"}
	}
	if dataStartRow <= headerRow {
		dataStartRow = headerRow + 1
	}
	if dataStartRow > len(rows)+1 {
		return Workspace{}, &ValidationError{Field: "dataStartRow", Message: "Начало данных находится за пределами листа"}
	}
	maxColumns := len(rows[headerRow-1])
	for _, row := range rows {
		if len(row) > maxColumns {
			maxColumns = len(row)
		}
	}
	if maxColumns > MaxColumns {
		return Workspace{}, &ValidationError{Field: "columns", Message: "Лист содержит больше 100 колонок"}
	}
	headers := rows[headerRow-1]
	columns := make([]WorkspaceColumn, 0, maxColumns)
	for index := 1; index <= maxColumns; index++ {
		name := ""
		if index <= len(headers) {
			name = strings.TrimSpace(headers[index-1])
		}
		if name == "" {
			name = fmt.Sprintf("Колонка %d", index)
		}
		columns = append(columns, WorkspaceColumn{ID: fmt.Sprintf("c%d", index), SourceIndex: index, SourceName: name, Field: "unassigned"})
	}
	for field, sourceIndex := range suggestMapping(headers) {
		mapped := field
		if field == "weight" {
			mapped = "grossWeight"
		}
		if field == "photoURL" {
			mapped = "photo"
		}
		for index := range columns {
			if columns[index].SourceIndex == sourceIndex && columns[index].Field == "unassigned" {
				columns[index].Field = mapped
			}
		}
	}
	workspace := Workspace{ImportID: item.ID, Sheets: sheets, Sheet: requestedSheet, HeaderRow: headerRow, DataStartRow: dataStartRow, Columns: columns, PriceBasis: "unknown", Batch: WorkspaceBatch{Currency: ""}, Revision: 1}
	// Keep the complete visible source area. Rows before the selected data start
	// remain in the draft for inspection but are never normalized as goods.
	for sourceRow := 1; sourceRow <= len(rows); sourceRow++ {
		row := rows[sourceRow-1]
		values := make(map[string]string, len(columns))
		original := make(map[string]string, len(columns))
		formulas := make(map[string]string, len(columns))
		for index, column := range columns {
			value := ""
			if index < len(row) {
				value = strings.TrimSpace(row[index])
			}
			cell, _ := excelize.CoordinatesToCellName(column.SourceIndex, sourceRow)
			formula, _ := workbook.GetCellFormula(requestedSheet, cell)
			values[column.ID], original[column.ID], formulas[column.ID] = value, value, formula
		}
		included, reason := likelyProduct(row)
		if sourceRow <= headerRow {
			included = false
			reason = "Строка до начала данных (заголовок или служебная строка) не будет импортирована"
		} else if sourceRow < dataStartRow {
			included = false
			reason = "Строка до начала данных не будет импортирована"
		}
		warnings := rowFormulaWarnings(workbook, requestedSheet, sourceRow, row)
		if reason != "" {
			warnings = append(warnings, reason)
		}
		workspace.Rows = append(workspace.Rows, WorkspaceRow{ID: fmt.Sprintf("r%d", sourceRow), SourceRow: sourceRow, Values: values, OriginalValues: original, Formulas: formulas, Included: included, Warnings: warnings})
	}
	workspace.Pictures, workspace.Warnings = pictureHints(workbook, requestedSheet)
	workspace.Warnings = append(workspace.Warnings, "Рабочая таблица является копией: исходный XLSX остаётся неизменным")
	workspace.Warnings = append(workspace.Warnings, "URL фотографий сохраняются как текст и не скачиваются автоматически")
	return workspace, nil
}

func (s *ImportService) SaveWorkspace(ctx context.Context, importID, ownerID int64, workspace Workspace) (Workspace, error) {
	item, err := s.repository.ImportByIDForOwner(ctx, importID, ownerID)
	if err != nil {
		return Workspace{}, err
	}
	if err := validateWorkspace(workspace, item.ID); err != nil {
		return Workspace{}, err
	}
	workspace.ImportID = importID
	model := workspaceModel(workspace, ownerID)
	saved, err := s.repository.SaveWorkspace(ctx, model)
	if err != nil {
		return Workspace{}, err
	}
	return decodeWorkspace(saved, workspace.Sheets)
}

func (s *ImportService) ApplyWorkspace(ctx context.Context, importID, ownerID int64, workspace Workspace) ([]repository.ImportedProduct, Workspace, error) {
	item, err := s.repository.ImportByIDForOwner(ctx, importID, ownerID)
	if err != nil {
		return nil, Workspace{}, err
	}
	if err := validateWorkspace(workspace, item.ID); err != nil {
		return nil, Workspace{}, err
	}
	estimate, err := s.repository.EstimateByIDForOwner(ctx, item.EstimateID, ownerID)
	if err != nil {
		return nil, Workspace{}, err
	}
	count, err := s.repository.ProductCount(ctx, item.EstimateID)
	if err != nil {
		return nil, Workspace{}, err
	}
	if count > 0 && !workspace.ReplaceExisting {
		return nil, Workspace{}, ErrReplacementRequired
	}
	products, err := productsFromWorkspace(workspace, item, estimate.Currency)
	if err != nil {
		return nil, Workspace{}, err
	}
	file, err := s.files.Open(item.StoredPath)
	if err != nil {
		return nil, Workspace{}, err
	}
	defer file.Close()
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return nil, Workspace{}, &ValidationError{Field: "file", Message: "Не удалось прочитать сохранённый XLSX-файл"}
	}
	defer workbook.Close()
	assets, links, pictureWarnings := s.extractPictures(workbook, workspace.Sheet, products)
	manualAssets, manualLinks := s.manualWorkspaceAssets(ctx, importID, workspace, products, len(assets))
	assets = append(assets, manualAssets...)
	for productIndex, assetIndexes := range manualLinks {
		links[productIndex] = append(links[productIndex], assetIndexes...)
	}
	for index, warnings := range pictureWarnings {
		current := decodeWarnings(products[index].Warnings)
		current = append(current, warnings...)
		products[index].Warnings = encodeJSON(current)
		if len(current) > 0 {
			products[index].ImportStatus = "needs_correction"
		}
	}
	workspace.ImportID = importID
	workspace.ReplaceExisting = false
	savedProducts, err := s.repository.SaveWorkspaceAndReplaceProducts(ctx, workspaceModel(workspace, ownerID), item.EstimateID, item.ID, ownerID, products, assets, links)
	if err != nil {
		return nil, Workspace{}, err
	}
	savedWorkspace, _ := s.repository.WorkspaceByImportForOwner(ctx, importID, ownerID)
	resultWorkspace, _ := decodeWorkspace(savedWorkspace, workspace.Sheets)
	resultWorkspace.Images = s.workspaceImages(ctx, importID)
	savedProducts, err = s.withImageAssetIDs(ctx, savedProducts)
	if err != nil {
		return nil, Workspace{}, err
	}
	return savedProducts, resultWorkspace, nil
}

func workspaceModel(workspace Workspace, ownerID int64) repository.ImportWorkspace {
	return repository.ImportWorkspace{ID: 0, ImportID: workspace.ImportID, Sheet: workspace.Sheet, HeaderRow: workspace.HeaderRow, DataStartRow: workspace.DataStartRow, Columns: encodeJSON(workspace.Columns), Rows: encodeJSON(workspace.Rows), DeletedRows: encodeJSON(workspace.DeletedRows), DeletedColumns: encodeJSON(workspace.DeletedColumns), Batch: encodeJSON(workspace.Batch), PriceBasis: workspace.PriceBasis, PriceUnitLabel: workspace.PriceUnitLabel, Revision: workspace.Revision, UpdatedBy: ownerID, UpdatedAt: time.Now().UTC()}
}

func decodeWorkspace(model repository.ImportWorkspace, sheets []SheetInfo) (Workspace, error) {
	var workspace Workspace
	if err := json.Unmarshal(model.Columns, &workspace.Columns); err != nil {
		return Workspace{}, err
	}
	if err := json.Unmarshal(model.Rows, &workspace.Rows); err != nil {
		return Workspace{}, err
	}
	_ = json.Unmarshal(model.DeletedRows, &workspace.DeletedRows)
	_ = json.Unmarshal(model.DeletedColumns, &workspace.DeletedColumns)
	_ = json.Unmarshal(model.Batch, &workspace.Batch)
	workspace.ImportID, workspace.Sheet, workspace.HeaderRow, workspace.DataStartRow = model.ImportID, model.Sheet, model.HeaderRow, model.DataStartRow
	workspace.PriceBasis, workspace.PriceUnitLabel, workspace.UpdatedBy, workspace.Revision = model.PriceBasis, model.PriceUnitLabel, model.UpdatedBy, model.Revision
	workspace.SavedAt = model.UpdatedAt.UTC().Format(time.RFC3339)
	workspace.Sheets = sheets
	if workspace.Columns == nil {
		workspace.Columns = []WorkspaceColumn{}
	}
	if workspace.Rows == nil {
		workspace.Rows = []WorkspaceRow{}
	}
	if workspace.DeletedRows == nil {
		workspace.DeletedRows = []WorkspaceRow{}
	}
	if workspace.DeletedColumns == nil {
		workspace.DeletedColumns = []DeletedColumn{}
	}
	if workspace.Batch == (WorkspaceBatch{}) {
		workspace.Batch = WorkspaceBatch{}
	}
	workspace.Images = []WorkspaceImage{}
	workspace.Pictures = []PictureHint{}
	workspace.Warnings = []string{}
	return workspace, nil
}

func validateWorkspace(workspace Workspace, importID int64) error {
	if workspace.ImportID != 0 && workspace.ImportID != importID {
		return &ValidationError{Field: "importId", Message: "Черновик относится к другому импорту"}
	}
	if strings.TrimSpace(workspace.Sheet) == "" {
		return &ValidationError{Field: "sheet", Message: "Выберите лист"}
	}
	if workspace.HeaderRow < 1 || workspace.DataStartRow <= workspace.HeaderRow {
		return &ValidationError{Field: "rows", Message: "Начало данных должно быть после строки заголовков"}
	}
	if len(workspace.Columns) == 0 || len(workspace.Columns) > MaxColumns {
		return &ValidationError{Field: "columns", Message: "Количество колонок должно быть от 1 до 100"}
	}
	columnIDs := map[string]bool{}
	fieldOwners := map[string]string{}
	for index, column := range workspace.Columns {
		if strings.TrimSpace(column.ID) == "" || columnIDs[column.ID] {
			return &ValidationError{Field: "columns", Message: "Идентификаторы колонок должны быть уникальными"}
		}
		columnIDs[column.ID] = true
		if column.SourceIndex < 0 || column.SourceIndex > MaxColumns || !workspaceFields[column.Field] {
			return &ValidationError{Field: "mapping", Message: fmt.Sprintf("Некорректное назначение колонки %d", index+1)}
		}
		if column.Field != "unassigned" && column.Field != "unused" && column.Field != "additional" {
			if previous, exists := fieldOwners[column.Field]; exists {
				return &ValidationError{Field: "mapping", Message: fmt.Sprintf("Поле уже назначено колонкам %s и %s", previous, column.SourceName)}
			}
			fieldOwners[column.Field] = column.SourceName
		}
		if column.Field == "additional" && strings.TrimSpace(column.ExtraName) == "" {
			return &ValidationError{Field: "mapping", Message: "Для дополнительного поля укажите собственное название"}
		}
		if (column.Field == "grossWeight" || column.Field == "netWeight") && column.WeightUnit != "" && column.WeightUnit != "kg" && column.WeightUnit != "g" {
			return &ValidationError{Field: "weight", Message: "Единица веса должна быть кг или г"}
		}
		if (column.Field == "grossWeight" || column.Field == "netWeight") && column.WeightBasis != "" && column.WeightBasis != "line" && column.WeightBasis != "unit" {
			return &ValidationError{Field: "weight", Message: "Укажите, относится ли вес к строке или единице"}
		}
	}
	if fieldOwners["name"] == "" {
		return &ValidationError{Field: "mapping", Message: "Назначьте колонку «Название товара»"}
	}
	if len(workspace.Rows) > MaxRowsPerSheet || len(workspace.DeletedRows) > MaxRowsPerSheet {
		return &ValidationError{Field: "rows", Message: "В рабочей таблице больше 5 000 строк"}
	}
	rowIDs := map[string]bool{}
	for _, row := range append(append([]WorkspaceRow{}, workspace.Rows...), workspace.DeletedRows...) {
		if strings.TrimSpace(row.ID) == "" || rowIDs[row.ID] {
			return &ValidationError{Field: "rows", Message: "Идентификаторы строк должны быть уникальными"}
		}
		rowIDs[row.ID] = true
		for columnID := range row.Values {
			if !columnIDs[columnID] {
				return &ValidationError{Field: "rows", Message: "Строка содержит неизвестную колонку"}
			}
		}
	}
	if !oneOf(workspace.PriceBasis, "piece", "explicit_unit", "unknown") || (workspace.PriceBasis == "explicit_unit" && strings.TrimSpace(workspace.PriceUnitLabel) == "") {
		return &ValidationError{Field: "priceBasis", Message: "Укажите единицу цены"}
	}
	return nil
}
