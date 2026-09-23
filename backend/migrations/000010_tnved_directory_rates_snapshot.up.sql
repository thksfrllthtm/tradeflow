-- Extend the operator overlay without replacing the existing catalog.
ALTER TABLE tnved_directory_entries
    ADD COLUMN name_zh TEXT NOT NULL DEFAULT '',
    ADD COLUMN company_coefficient NUMERIC(30,12),
    ADD COLUMN duty_percent NUMERIC(30,12),
    ADD COLUMN duty_amount NUMERIC(30,12),
    ADD COLUMN duty_currency VARCHAR(3) NOT NULL DEFAULT '',
    ADD COLUMN duty_unit VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN vat_percent NUMERIC(30,12),
    ADD COLUMN vat_basis TEXT NOT NULL DEFAULT '';

ALTER TABLE tnved_directory_entries DROP CONSTRAINT IF EXISTS tnved_directory_entries_duty_type_check;
ALTER TABLE tnved_directory_entries DROP CONSTRAINT IF EXISTS tnved_directory_entries_vat_type_check;
-- Legacy "other" rows had no calculable value. Clear the legacy value before
-- changing the type so the old consistency check can still be satisfied.
UPDATE tnved_directory_entries SET duty_type = 'unknown', duty_value = NULL WHERE duty_type = 'other';
ALTER TABLE tnved_directory_entries
    ADD CONSTRAINT tnved_directory_entries_duty_type_check
        CHECK (duty_type IN ('unknown', 'percentage', 'specific', 'combined_minimum')),
    ADD CONSTRAINT tnved_directory_entries_vat_type_check
        CHECK (vat_type IN ('unknown', 'percentage', 'exempt')),
    ADD CONSTRAINT tnved_directory_entries_coefficient_check
        CHECK (company_coefficient IS NULL OR company_coefficient >= 0),
    ADD CONSTRAINT tnved_directory_entries_duty_percent_check
        CHECK (duty_percent IS NULL OR (duty_percent >= 0 AND duty_percent <= 100)),
    ADD CONSTRAINT tnved_directory_entries_duty_amount_check
        CHECK (duty_amount IS NULL OR duty_amount >= 0),
    ADD CONSTRAINT tnved_directory_entries_vat_percent_check
        CHECK (vat_percent IS NULL OR (vat_percent >= 0 AND vat_percent <= 100));

-- Preserve records made with the previous percentage-only form.
UPDATE tnved_directory_entries
SET duty_percent = duty_value
WHERE duty_type = 'percentage' AND duty_value IS NOT NULL AND duty_percent IS NULL;
UPDATE tnved_directory_entries
SET vat_percent = vat_value
WHERE vat_type = 'percentage' AND vat_value IS NOT NULL AND vat_percent IS NULL;
-- A legacy percentage row without a value is semantically unknown; keep the
-- distinction explicit instead of creating an unusable percentage record.
UPDATE tnved_directory_entries
SET duty_type = 'unknown'
WHERE duty_type = 'percentage' AND duty_percent IS NULL;
UPDATE tnved_directory_entries
SET vat_type = 'unknown'
WHERE vat_type = 'percentage' AND vat_percent IS NULL;

ALTER TABLE imported_products
    ADD COLUMN tnved_directory_entry_id BIGINT REFERENCES tnved_directory_entries(id) ON DELETE SET NULL,
    ADD COLUMN tnved_directory_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX imported_products_tnved_directory_idx ON imported_products(tnved_directory_entry_id);
