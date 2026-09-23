CREATE TABLE gloves_copy_estimates (
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

CREATE INDEX gloves_copy_estimates_user_updated_idx
    ON gloves_copy_estimates(user_id, updated_at DESC);
