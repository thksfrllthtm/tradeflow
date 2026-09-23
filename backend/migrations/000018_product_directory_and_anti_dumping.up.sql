ALTER TABLE tnved_directory_entries
    ADD COLUMN anti_dumping_percent NUMERIC(30,12)
        CHECK (anti_dumping_percent IS NULL OR (anti_dumping_percent >= 0 AND anti_dumping_percent <= 100));

CREATE TABLE product_directory_entries (
    id BIGSERIAL PRIMARY KEY,
    category VARCHAR(64) NOT NULL DEFAULT 'Товары',
    name VARCHAR(300) NOT NULL,
    name_zh TEXT NOT NULL DEFAULT '',
    unit VARCHAR(64) NOT NULL DEFAULT 'шт.',
    synonyms_ru JSONB NOT NULL DEFAULT '[]'::jsonb,
    synonyms_zh JSONB NOT NULL DEFAULT '[]'::jsonb,
    synonyms_en JSONB NOT NULL DEFAULT '[]'::jsonb,
    tnved_directory_entry_id BIGINT REFERENCES tnved_directory_entries(id),
    verification_status VARCHAR(24) NOT NULL DEFAULT 'needs_review'
        CHECK (verification_status IN ('needs_review', 'operator_verified')),
    archived_at TIMESTAMPTZ,
    archived_by BIGINT REFERENCES users(id),
    created_by BIGINT NOT NULL REFERENCES users(id),
    updated_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX product_directory_tnved_idx ON product_directory_entries(tnved_directory_entry_id);
CREATE INDEX product_directory_category_idx ON product_directory_entries(category);
CREATE INDEX product_directory_status_idx ON product_directory_entries(verification_status);
CREATE UNIQUE INDEX product_directory_active_name_idx
    ON product_directory_entries (lower(category), lower(name))
    WHERE archived_at IS NULL;
