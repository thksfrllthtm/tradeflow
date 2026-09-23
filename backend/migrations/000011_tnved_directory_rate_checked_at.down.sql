ALTER TABLE tnved_directory_entries
    DROP COLUMN IF EXISTS vat_checked_at,
    DROP COLUMN IF EXISTS duty_checked_at;
