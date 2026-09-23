package service

// Images of the import workspace: extracted pictures and manually uploaded row images.

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"os"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func (s *ImportService) workspaceImages(ctx context.Context, importID int64) []WorkspaceImage {
	assets, err := s.repository.AssetsForImport(ctx, importID)
	if err != nil {
		return nil
	}
	images := make([]WorkspaceImage, 0)
	for _, asset := range assets {
		rowID := asset.SourceCell
		if asset.Kind == "embedded" && asset.SourceRow > 0 {
			rowID = fmt.Sprintf("r%d", asset.SourceRow)
		}
		images = append(images, WorkspaceImage{AssetID: asset.ID, RowID: rowID, MIMEType: asset.MIMEType, FileSize: asset.FileSize, Kind: asset.Kind})
	}
	return images
}

func (s *ImportService) manualWorkspaceAssets(ctx context.Context, importID int64, workspace Workspace, products []repository.ImportedProduct, offset int) ([]repository.ImportAsset, map[int][]int) {
	assets, err := s.repository.AssetsForImport(ctx, importID)
	if err != nil {
		return nil, nil
	}
	rowToProduct := map[int]int{}
	for index, product := range products {
		rowToProduct[product.SourceRow] = index
	}
	workspaceRowToProduct := map[string]int{}
	for _, row := range workspace.Rows {
		if index, ok := rowToProduct[row.SourceRow]; ok {
			workspaceRowToProduct[row.ID] = index
		}
	}
	selected := make([]repository.ImportAsset, 0)
	links := map[int][]int{}
	for _, asset := range assets {
		if asset.Kind != "manual" {
			continue
		}
		productIndex, ok := workspaceRowToProduct[asset.SourceCell]
		if !ok {
			continue
		}
		selected = append(selected, asset)
		links[productIndex] = append(links[productIndex], offset+len(selected)-1)
	}
	return selected, links
}

func (s *ImportService) UploadWorkspaceImage(ctx context.Context, importID, ownerID int64, rowID string, replace bool, header *multipart.FileHeader) (WorkspaceImage, error) {
	item, err := s.repository.ImportByIDForOwner(ctx, importID, ownerID)
	if err != nil {
		return WorkspaceImage{}, err
	}
	workspace, err := s.GetWorkspace(ctx, importID, ownerID, "", 0, 0)
	if err != nil {
		return WorkspaceImage{}, err
	}
	var row WorkspaceRow
	for _, candidate := range workspace.Rows {
		if candidate.ID == rowID {
			row = candidate
			break
		}
	}
	if row.ID == "" {
		return WorkspaceImage{}, &ValidationError{Field: "rowId", Message: "Выберите существующую строку рабочей таблицы"}
	}
	contents, mimeType, extension, err := readSupportedImage(header)
	if err != nil {
		return WorkspaceImage{}, err
	}
	storedPath, size, err := s.files.Save(bytes.NewReader(contents), "images", extension)
	if err != nil {
		return WorkspaceImage{}, err
	}
	asset, err := s.repository.CreateAsset(ctx, repository.ImportAsset{ImportID: item.ID, SourceSheet: workspace.Sheet, SourceCell: row.ID, SourceRow: row.SourceRow, StoredPath: storedPath, MIMEType: mimeType, FileSize: size, Kind: "manual"})
	if err != nil {
		_ = s.files.Remove(storedPath)
		return WorkspaceImage{}, err
	}
	if replace {
		if err := s.repository.DeleteManualAssetsForImportRowExcept(ctx, item.ID, row.ID, asset.ID); err != nil {
			return WorkspaceImage{}, err
		}
	}
	return WorkspaceImage{AssetID: asset.ID, RowID: row.ID, MIMEType: asset.MIMEType, FileSize: asset.FileSize, Kind: asset.Kind}, nil
}

func (s *ImportService) OpenWorkspaceAsset(ctx context.Context, importID, assetID, ownerID int64) (*os.File, repository.ImportAsset, error) {
	asset, err := s.repository.AssetByIDForImportOwner(ctx, assetID, importID, ownerID)
	if err != nil {
		return nil, repository.ImportAsset{}, err
	}
	file, err := s.files.Open(asset.StoredPath)
	return file, asset, err
}

func readSupportedImage(header *multipart.FileHeader) ([]byte, string, string, error) {
	if header == nil || header.Size <= 0 || header.Size > MaxImageBytes {
		return nil, "", "", &ValidationError{Field: "image", Message: "Выберите JPG или PNG размером до 5 МБ"}
	}
	file, err := header.Open()
	if err != nil {
		return nil, "", "", err
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, MaxImageBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(contents) > MaxImageBytes {
		return nil, "", "", &ValidationError{Field: "image", Message: "Изображение больше 5 МБ"}
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(contents))
	if err != nil || config.Width < 1 || config.Height < 1 || (format != "jpeg" && format != "png") {
		return nil, "", "", &ValidationError{Field: "image", Message: "Поддерживаются корректные изображения JPG и PNG"}
	}
	if format == "png" {
		return contents, "image/png", ".png", nil
	}
	return contents, "image/jpeg", ".jpg", nil
}
