-- «Документы для ввоза» по коду ТН ВЭД: итог агента (сертификат соответствия, декларация соответствия,
-- СГР, условный выпуск и прочие меры) хранится отдельно от разговора — по коду, с описанием товара.
-- Один ответ — одна строка (история не теряется: у разных товаров под одним кодом меры могут различаться);
-- показывается последний по коду.
CREATE TABLE tnved_code_documents (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(10) NOT NULL,
    product TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    -- Записи AgentDocument: kind (cc, dc, sgr, conditional, other), measure, act, url, applies, basis…
    documents JSONB NOT NULL DEFAULT '[]',
    session_id BIGINT REFERENCES tnved_agent_sessions(id) ON DELETE SET NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tnved_code_documents_code_idx ON tnved_code_documents (code, created_at DESC);
-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE tnved_code_documents ENABLE ROW LEVEL SECURITY;
