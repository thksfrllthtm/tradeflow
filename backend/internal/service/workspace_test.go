package service

import (
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func testWorkspace() Workspace {
	return Workspace{
		ImportID: 9, Sheet: "Товары", HeaderRow: 2, DataStartRow: 3, PriceBasis: "piece",
		Columns: []WorkspaceColumn{
			{ID: "c1", SourceIndex: 1, SourceName: "Название", Field: "name"},
			{ID: "c2", SourceIndex: 2, SourceName: "Кол-во", Field: "quantity"},
			{ID: "c3", SourceIndex: 3, SourceName: "Цена", Field: "unitPrice"},
			{ID: "c4", SourceIndex: 4, SourceName: "Валюта", Field: "currency"},
			{ID: "c5", SourceIndex: 5, SourceName: "G.W.", Field: "grossWeight", WeightUnit: "kg", WeightBasis: "line"},
		},
		Rows: []WorkspaceRow{
			{ID: "r3", SourceRow: 3, Values: map[string]string{"c1": "Чайник", "c2": "2", "c3": "10,50", "c4": "USD", "c5": "1,25"}, OriginalValues: map[string]string{"c1": "Чайник", "c2": "2", "c3": "10,50", "c4": "USD", "c5": "1,25"}, Included: true},
			{ID: "r4", SourceRow: 4, Values: map[string]string{"c1": "Удалена"}, OriginalValues: map[string]string{"c1": "Удалена"}, Included: false},
		},
	}
}

func TestValidateWorkspaceRejectsDuplicateStandardField(t *testing.T) {
	workspace := testWorkspace()
	workspace.Columns[1].Field = "name"
	if err := validateWorkspace(workspace, workspace.ImportID); err == nil {
		t.Fatal("expected duplicate mapping to be rejected")
	}
}

func TestProductsFromWorkspaceKeepsOrderAndWeightMapping(t *testing.T) {
	workspace := testWorkspace()
	products, err := productsFromWorkspace(workspace, repository.EstimateImport{ID: 9, EstimateID: 4}, "KZT")
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].Name != "Чайник" || products[0].SourceRow != 3 {
		t.Fatalf("unexpected products: %#v", products)
	}
	if products[0].Quantity.String() != "2" || products[0].UnitPrice.String() != "10.5" || products[0].GrossWeight.String() != "1.25" || products[0].GrossWeightUnit != "kg" {
		t.Fatalf("unexpected normalized values: %#v", products[0])
	}
}

func TestProductsFromWorkspaceRejectsInconsistentLineTotal(t *testing.T) {
	workspace := testWorkspace()
	workspace.Columns = append(workspace.Columns, WorkspaceColumn{ID: "c6", SourceIndex: 6, SourceName: "Сумма", Field: "lineTotal"})
	workspace.Rows[0].Values["c6"] = "25"
	if _, err := productsFromWorkspace(workspace, repository.EstimateImport{ID: 9, EstimateID: 4}, "USD"); err == nil {
		t.Fatal("expected line total mismatch to be rejected")
	}
}
