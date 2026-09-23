ALTER TABLE imported_products DROP CONSTRAINT IF EXISTS imported_products_source_row_check;
ALTER TABLE imported_products ADD CONSTRAINT imported_products_source_row_check CHECK (source_row >= 0);
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS quantity_unit VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS gross_weight NUMERIC(20,6);
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS gross_weight_unit VARCHAR(8) NOT NULL DEFAULT '';
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS gross_weight_basis VARCHAR(16) NOT NULL DEFAULT 'unknown';
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS net_weight NUMERIC(20,6);
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS net_weight_unit VARCHAR(8) NOT NULL DEFAULT '';
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS net_weight_basis VARCHAR(16) NOT NULL DEFAULT 'unknown';
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS material TEXT NOT NULL DEFAULT '';
ALTER TABLE imported_products ADD COLUMN IF NOT EXISTS additional_fields JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE import_workspaces (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    import_id BIGINT NOT NULL UNIQUE REFERENCES estimate_imports(id) ON DELETE CASCADE,
    sheet VARCHAR(255) NOT NULL,
    header_row INTEGER NOT NULL CHECK (header_row > 0),
    data_start_row INTEGER NOT NULL CHECK (data_start_row > header_row),
    columns JSONB NOT NULL DEFAULT '[]'::jsonb,
    rows JSONB NOT NULL DEFAULT '[]'::jsonb,
    deleted_rows JSONB NOT NULL DEFAULT '[]'::jsonb,
    deleted_columns JSONB NOT NULL DEFAULT '[]'::jsonb,
    batch JSONB NOT NULL DEFAULT '{}'::jsonb,
    price_basis VARCHAR(32) NOT NULL DEFAULT 'unknown',
    price_unit_label VARCHAR(100) NOT NULL DEFAULT '',
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX import_workspaces_import_idx ON import_workspaces(import_id);
