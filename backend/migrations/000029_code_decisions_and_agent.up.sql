-- База решений: проверенные примеры «товар из инвойса → код ТН ВЭД» с фото, тем, как товар был
-- написан в инвойсе, и обоснованием. Пополняется из подбора ИИ-агентом, вручную и импортом
-- старых инвойсов; «Проверено» ставит только администратор.
CREATE TABLE code_decisions (
    id BIGSERIAL PRIMARY KEY,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'rejected')),
    source VARCHAR(16) NOT NULL CHECK (source IN ('agent', 'manual', 'import')),
    invoice_name TEXT NOT NULL DEFAULT '',
    supplier_name TEXT NOT NULL DEFAULT '',
    final_name TEXT NOT NULL DEFAULT '',
    code VARCHAR(10) NOT NULL CHECK (code ~ '^[0-9]{10}$'),
    reason TEXT NOT NULL DEFAULT '',
    quote TEXT NOT NULL DEFAULT '',
    source_url VARCHAR(500) NOT NULL DEFAULT '',
    -- Коды, которые рассматривались и не подошли: [{"code": "...", "why": "..."}].
    rejected_codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    unit_net_kg NUMERIC(14,6),
    unit_price_usd NUMERIC(14,4),
    -- Фото (data URL, уменьшенные в браузере), миниатюра первого фото для списка и
    -- перцептивные хеши фото (16 hex-символов) для поиска того же товара по фото.
    photos JSONB NOT NULL DEFAULT '[]'::jsonb,
    thumb TEXT NOT NULL DEFAULT '',
    photo_hashes JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- Что предлагал агент (лучший код) и прав ли он оказался после проверки администратором.
    agent_code VARCHAR(10) NOT NULL DEFAULT '',
    agent_correct BOOLEAN,
    estimate_id BIGINT REFERENCES gloves_copy_estimates(id) ON DELETE SET NULL,
    item_id VARCHAR(64) NOT NULL DEFAULT '',
    session_id BIGINT,
    product_directory_id BIGINT REFERENCES product_directory_entries(id) ON DELETE SET NULL,
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    verified_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX code_decisions_status_idx ON code_decisions (status, updated_at DESC);
CREATE INDEX code_decisions_code_idx ON code_decisions (code);

-- Подбор кода ИИ-агентом для строки сметы: состояние, переписка с моделью (transcript, фото —
-- ссылками на photos) и расход токенов.
CREATE TABLE tnved_agent_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    estimate_id BIGINT REFERENCES gloves_copy_estimates(id) ON DELETE CASCADE,
    item_id VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL CHECK (status IN ('running', 'waiting', 'done', 'error', 'cancelled')),
    item JSONB NOT NULL DEFAULT '{}'::jsonb,
    photos JSONB NOT NULL DEFAULT '[]'::jsonb,
    transcript JSONB NOT NULL DEFAULT '[]'::jsonb,
    result JSONB NOT NULL DEFAULT 'null'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    steps INTEGER NOT NULL DEFAULT 0,
    runs INTEGER NOT NULL DEFAULT 0,
    prompt_tokens BIGINT NOT NULL DEFAULT 0,
    cached_tokens BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0,
    model VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tnved_agent_sessions_row_idx ON tnved_agent_sessions (estimate_id, item_id, id DESC);
CREATE INDEX tnved_agent_sessions_user_idx ON tnved_agent_sessions (user_id, created_at);

ALTER TABLE code_decisions ADD CONSTRAINT code_decisions_session_fk
    FOREIGN KEY (session_id) REFERENCES tnved_agent_sessions(id) ON DELETE SET NULL;

-- Сообщения чата подбора, как их видит сотрудник: товар, шаги агента, вопросы, итог.
CREATE TABLE tnved_agent_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES tnved_agent_sessions(id) ON DELETE CASCADE,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'result', 'match', 'error', 'notice')),
    content JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tnved_agent_messages_session_idx ON tnved_agent_messages (session_id, id);

-- Страницы кодов TWS.BY («Подробнее»), очищенные до полезного текста: скачиваются один раз.
CREATE TABLE tnved_details_pages (
    code VARCHAR(10) PRIMARY KEY CHECK (code ~ '^[0-9]{10}$'),
    url VARCHAR(500) NOT NULL,
    text TEXT NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE code_decisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE tnved_agent_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE tnved_agent_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE tnved_details_pages ENABLE ROW LEVEL SECURITY;
