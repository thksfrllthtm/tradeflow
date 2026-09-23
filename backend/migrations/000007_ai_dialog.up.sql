ALTER TABLE imported_products
    ADD COLUMN ai_data_revision INTEGER NOT NULL DEFAULT 1;

CREATE TABLE product_ai_dialogues (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL UNIQUE REFERENCES imported_products(id) ON DELETE CASCADE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE product_ai_messages (
    id BIGSERIAL PRIMARY KEY,
    dialogue_id BIGINT NOT NULL REFERENCES product_ai_dialogues(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    author VARCHAR(16) NOT NULL CHECK (author IN ('user', 'assistant')),
    content TEXT NOT NULL,
    attachment_asset_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    response JSONB NOT NULL DEFAULT '{}'::jsonb,
    model VARCHAR(128) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'completed',
    client_request_id VARCHAR(128),
    analysis_id BIGINT REFERENCES product_ai_analyses(id) ON DELETE SET NULL,
    product_revision INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT product_ai_messages_author_user_check CHECK (author = 'assistant' OR user_id IS NOT NULL)
);

CREATE UNIQUE INDEX product_ai_messages_request_idx
    ON product_ai_messages(dialogue_id, client_request_id)
    WHERE client_request_id IS NOT NULL;
CREATE INDEX product_ai_messages_dialogue_idx
    ON product_ai_messages(dialogue_id, created_at, id);
