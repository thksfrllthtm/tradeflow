ALTER TABLE imported_products
    ADD COLUMN tn_ved_code VARCHAR(10) NOT NULL DEFAULT '',
    ADD COLUMN tn_ved_code_source VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE imported_products
    ADD CONSTRAINT imported_products_tn_ved_code_format
    CHECK (tn_ved_code = '' OR tn_ved_code ~ '^[0-9]{10}$');

CREATE TABLE product_ai_analyses (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES imported_products(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    model VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL CHECK (status IN ('proposed_code','needs_clarification','failed')),
    input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    needs_recheck BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX product_ai_analyses_product_idx ON product_ai_analyses(product_id, created_at DESC);
