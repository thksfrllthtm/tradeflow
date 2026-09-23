-- Operator-managed overlay for the classifier. It intentionally does not
-- modify immutable classifier_versions/classifier_codes and is not an
-- official source of customs law.
CREATE TABLE tnved_directory_entries (
    id BIGSERIAL PRIMARY KEY,
    category VARCHAR(64) NOT NULL DEFAULT 'Перчатки',
    name VARCHAR(300) NOT NULL,
    code VARCHAR(10) NOT NULL,
    synonyms_ru JSONB NOT NULL DEFAULT '[]'::jsonb,
    synonyms_zh JSONB NOT NULL DEFAULT '[]'::jsonb,
    synonyms_en JSONB NOT NULL DEFAULT '[]'::jsonb,
    verification_status VARCHAR(24) NOT NULL DEFAULT 'needs_review'
        CHECK (verification_status IN ('needs_review', 'operator_verified')),
    duty_type VARCHAR(16) NOT NULL DEFAULT 'unknown'
        CHECK (duty_type IN ('unknown', 'percentage', 'other')),
    duty_value NUMERIC(30,12),
    duty_valid_from DATE,
    duty_valid_to DATE,
    duty_source TEXT NOT NULL DEFAULT '',
    vat_type VARCHAR(16) NOT NULL DEFAULT 'unknown'
        CHECK (vat_type IN ('unknown', 'percentage')),
    vat_value NUMERIC(30,12),
    vat_valid_from DATE,
    vat_valid_to DATE,
    vat_source TEXT NOT NULL DEFAULT '',
    archived_at TIMESTAMPTZ,
    archived_by BIGINT REFERENCES users(id),
    created_by BIGINT NOT NULL REFERENCES users(id),
    updated_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (code ~ '^[0-9]{10}$'),
    CHECK (duty_value IS NULL OR (duty_value >= 0 AND duty_value <= 100)),
    CHECK (vat_value IS NULL OR (vat_value >= 0 AND vat_value <= 100)),
    CHECK (duty_type = 'percentage' OR duty_value IS NULL),
    CHECK (vat_type = 'percentage' OR vat_value IS NULL),
    CHECK (duty_valid_to IS NULL OR duty_valid_from IS NULL OR duty_valid_to >= duty_valid_from),
    CHECK (vat_valid_to IS NULL OR vat_valid_from IS NULL OR vat_valid_to >= vat_valid_from)
);

CREATE INDEX tnved_directory_code_idx ON tnved_directory_entries(code);
CREATE INDEX tnved_directory_category_idx ON tnved_directory_entries(category);
CREATE INDEX tnved_directory_status_idx ON tnved_directory_entries(verification_status);
CREATE UNIQUE INDEX tnved_directory_active_name_idx
    ON tnved_directory_entries (code, lower(category), lower(name))
    WHERE archived_at IS NULL;
