DROP INDEX IF EXISTS imported_products_tnved_directory_idx;
ALTER TABLE imported_products
    DROP COLUMN IF EXISTS tnved_directory_snapshot,
    DROP COLUMN IF EXISTS tnved_directory_entry_id;
ALTER TABLE tnved_directory_entries
    DROP CONSTRAINT IF EXISTS tnved_directory_entries_vat_percent_check,
    DROP CONSTRAINT IF EXISTS tnved_directory_entries_duty_amount_check,
    DROP CONSTRAINT IF EXISTS tnved_directory_entries_duty_percent_check,
    DROP CONSTRAINT IF EXISTS tnved_directory_entries_coefficient_check,
    DROP CONSTRAINT IF EXISTS tnved_directory_entries_vat_type_check,
    DROP CONSTRAINT IF EXISTS tnved_directory_entries_duty_type_check;
UPDATE tnved_directory_entries SET duty_type = 'unknown' WHERE duty_type NOT IN ('unknown', 'percentage', 'other');
UPDATE tnved_directory_entries SET vat_type = 'unknown' WHERE vat_type NOT IN ('unknown', 'percentage');
ALTER TABLE tnved_directory_entries
    ADD CONSTRAINT tnved_directory_entries_duty_type_check CHECK (duty_type IN ('unknown', 'percentage', 'other')),
    ADD CONSTRAINT tnved_directory_entries_vat_type_check CHECK (vat_type IN ('unknown', 'percentage'));
ALTER TABLE tnved_directory_entries
    DROP COLUMN IF EXISTS vat_basis,
    DROP COLUMN IF EXISTS vat_percent,
    DROP COLUMN IF EXISTS duty_unit,
    DROP COLUMN IF EXISTS duty_currency,
    DROP COLUMN IF EXISTS duty_amount,
    DROP COLUMN IF EXISTS duty_percent,
    DROP COLUMN IF EXISTS company_coefficient,
    DROP COLUMN IF EXISTS name_zh;
