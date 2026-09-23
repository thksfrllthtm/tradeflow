DROP TABLE IF EXISTS customs_calculation_revisions;
ALTER TABLE customs_calculations
    DROP CONSTRAINT IF EXISTS customs_calculation_delivery_mode_check,
    DROP CONSTRAINT IF EXISTS customs_calculation_delivery_total_nonnegative,
    DROP COLUMN IF EXISTS calculated_at,
    DROP COLUMN IF EXISTS totals,
    DROP COLUMN IF EXISTS input_snapshot,
    DROP COLUMN IF EXISTS rounding_policy,
    DROP COLUMN IF EXISTS formula_version,
    DROP COLUMN IF EXISTS delivery_total_usd,
    DROP COLUMN IF EXISTS delivery_mode;
