ALTER TABLE customs_calculations
    ADD COLUMN delivery_mode VARCHAR(16) NOT NULL DEFAULT '',
    ADD COLUMN delivery_total_usd NUMERIC(30,12),
    ADD COLUMN formula_version VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN rounding_policy VARCHAR(64) NOT NULL DEFAULT '',
    ADD COLUMN input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN totals JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN calculated_at TIMESTAMPTZ;

ALTER TABLE customs_calculations
    ADD CONSTRAINT customs_calculation_delivery_mode_check
        CHECK (delivery_mode IN ('', 'wagons', 'total')),
    ADD CONSTRAINT customs_calculation_delivery_total_nonnegative
        CHECK (delivery_total_usd IS NULL OR delivery_total_usd >= 0);

CREATE TABLE customs_calculation_revisions (
    id BIGSERIAL PRIMARY KEY,
    calculation_id BIGINT NOT NULL REFERENCES customs_calculations(id) ON DELETE CASCADE,
    revision_no BIGINT NOT NULL,
    formula_version VARCHAR(64) NOT NULL,
    rounding_policy VARCHAR(64) NOT NULL,
    input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT customs_calculation_revision_unique UNIQUE (calculation_id, revision_no)
);

CREATE INDEX customs_calculation_revisions_calculation_idx
    ON customs_calculation_revisions(calculation_id, revision_no DESC);
