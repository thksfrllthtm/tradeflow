CREATE TABLE gloves_estimates (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(200) NOT NULL,
    client VARCHAR(200) NOT NULL DEFAULT '',
    assessment_date DATE NOT NULL DEFAULT CURRENT_DATE,
    input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    result_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX gloves_estimates_user_updated_idx
    ON gloves_estimates(user_id, updated_at DESC);

-- Preserve the existing single per-user gloves draft as the first estimate.
INSERT INTO gloves_estimates (user_id, name, client, assessment_date, input_snapshot, result_snapshot, created_at, updated_at)
SELECT
    user_id,
    'Перчатки',
    '',
    CASE
        WHEN input_snapshot->>'assessmentDate' ~ '^\d{4}-\d{2}-\d{2}$'
            THEN (input_snapshot->>'assessmentDate')::date
        ELSE CURRENT_DATE
    END,
    input_snapshot,
    result_snapshot,
    created_at,
    updated_at
FROM gloves_calculation_drafts;
