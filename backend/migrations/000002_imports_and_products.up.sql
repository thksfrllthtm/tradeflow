CREATE TABLE estimate_imports (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    estimate_id BIGINT NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
    original_filename VARCHAR(255) NOT NULL,
    stored_path TEXT NOT NULL,
    sha256 CHAR(64) NOT NULL,
    file_size BIGINT NOT NULL CHECK (file_size > 0),
    status VARCHAR(32) NOT NULL DEFAULT 'uploaded' CHECK (status IN ('uploaded', 'confirmed')),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMPTZ,
    confirmed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT
);
CREATE INDEX estimate_imports_estimate_created_idx ON estimate_imports(estimate_id, created_at DESC);
CREATE INDEX estimate_imports_estimate_sha_idx ON estimate_imports(estimate_id, sha256);

CREATE TABLE imported_products (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    estimate_id BIGINT NOT NULL REFERENCES estimates(id) ON DELETE CASCADE,
    source_import_id BIGINT NOT NULL REFERENCES estimate_imports(id) ON DELETE RESTRICT,
    source_sheet VARCHAR(255) NOT NULL,
    source_row INTEGER NOT NULL CHECK (source_row > 0),
    source_values JSONB NOT NULL DEFAULT '{}'::jsonb,
    name TEXT NOT NULL DEFAULT '',
    translation TEXT NOT NULL DEFAULT '',
    sku VARCHAR(255) NOT NULL DEFAULT '',
    quantity NUMERIC(20, 6),
    unit_price NUMERIC(20, 8),
    line_total NUMERIC(20, 8),
    currency CHAR(3) NOT NULL DEFAULT '',
    weight NUMERIC(20, 6),
    characteristics TEXT NOT NULL DEFAULT '',
    photo_url TEXT NOT NULL DEFAULT '',
    price_basis VARCHAR(32) NOT NULL CHECK (price_basis IN ('piece', 'explicit_unit', 'unknown')),
    price_unit_label VARCHAR(100) NOT NULL DEFAULT '',
    weight_basis VARCHAR(32) NOT NULL CHECK (weight_basis IN ('line', 'unit', 'unknown')),
    import_status VARCHAR(32) NOT NULL DEFAULT 'imported' CHECK (import_status IN ('imported', 'needs_correction')),
    warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    excluded BOOLEAN NOT NULL DEFAULT FALSE,
    exclusion_reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX imported_products_estimate_idx ON imported_products(estimate_id, id);
CREATE INDEX imported_products_source_idx ON imported_products(source_import_id, source_sheet, source_row);

CREATE TABLE import_assets (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    import_id BIGINT NOT NULL REFERENCES estimate_imports(id) ON DELETE CASCADE,
    source_sheet VARCHAR(255) NOT NULL DEFAULT '',
    source_cell VARCHAR(32) NOT NULL DEFAULT '',
    source_row INTEGER NOT NULL DEFAULT 0,
    stored_path TEXT NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    file_size BIGINT NOT NULL CHECK (file_size > 0),
    kind VARCHAR(32) NOT NULL CHECK (kind IN ('embedded', 'manual')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX import_assets_import_idx ON import_assets(import_id);

CREATE TABLE imported_product_images (
    product_id BIGINT NOT NULL REFERENCES imported_products(id) ON DELETE CASCADE,
    asset_id BIGINT NOT NULL REFERENCES import_assets(id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, asset_id)
);
