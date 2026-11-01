package dbadminapp

import (
	"testing"

	"github.com/Fleetdock/fleetdock/backend/internal/platform/engine"
)

func TestBuildDiagram(t *testing.T) {
	cols := &engine.ColumnCatalog{Tables: []engine.CatalogTable{
		{Name: "orders", Kind: "table", Columns: []engine.CatalogColumn{{Name: "id", Type: "int", PrimaryKey: true}}},
		{Name: "open_orders", Kind: "view"},
		{Schema: "billing", Name: "invoices", Kind: "table"},
	}}
	tables := []engine.TableInfo{
		{Schema: "shop", Name: "orders", RowCount: 42},          // MySQL: Schema is the database
		{Schema: "billing", Name: "invoices", RowCount: 7},      // PostgreSQL
		{Schema: "public", Name: "invoices", RowCount: 1000000}, // same name, other schema
	}
	d := buildDiagram(cols, nil, tables)
	if len(d.Tables) != 3 || d.Relations == nil || d.Truncated {
		t.Fatalf("diagram = %+v", d)
	}
	if d.Tables[0].RowEstimate == nil || *d.Tables[0].RowEstimate != 42 {
		t.Errorf("orders estimate = %v", d.Tables[0].RowEstimate)
	}
	if d.Tables[1].RowEstimate != nil || d.Tables[1].Columns == nil {
		t.Errorf("view = %+v", d.Tables[1])
	}
	if d.Tables[2].RowEstimate == nil || *d.Tables[2].RowEstimate != 7 {
		t.Errorf("invoices estimate = %v", d.Tables[2].RowEstimate)
	}
}
