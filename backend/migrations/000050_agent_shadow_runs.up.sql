-- Теневой подбор кода ТН ВЭД: тот же товар параллельно с основной моделью подбирает вторая модель (Claude),
-- сотрудник её не видит. Одна строка на сессию, только первый ход подбора по строке сметы. Расход тени —
-- отдельно от сессии и лимитов сотрудника; модели сравнивает администратор в журнале агента.
-- messages — лента событий тени, как сообщения чата сотрудника (tool, text, question, result, notice, error).
-- status skipped — тень не запускалась (бюджет или дневной лимит), причина в error.
CREATE TABLE tnved_agent_shadow_runs (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL UNIQUE REFERENCES tnved_agent_sessions(id) ON DELETE CASCADE,
    model VARCHAR(100) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL CHECK (status IN ('running', 'done', 'error', 'skipped')),
    transcript JSONB NOT NULL DEFAULT '[]'::jsonb,
    messages JSONB NOT NULL DEFAULT '[]'::jsonb,
    result JSONB NOT NULL DEFAULT 'null'::jsonb,
    error TEXT NOT NULL DEFAULT '',
    steps INTEGER NOT NULL DEFAULT 0,
    prompt_tokens BIGINT NOT NULL DEFAULT 0,
    cached_tokens BIGINT NOT NULL DEFAULT 0,
    completion_tokens BIGINT NOT NULL DEFAULT 0,
    cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0,
    vision_cost_usd NUMERIC(12,6) NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);
CREATE INDEX tnved_agent_shadow_runs_started_idx ON tnved_agent_shadow_runs (started_at DESC);

-- См. 000020_row_level_security: таблица закрыта от Data API Supabase.
ALTER TABLE tnved_agent_shadow_runs ENABLE ROW LEVEL SECURITY;

-- Журнал агента: подборы за период и запись базы решений по подбору.
CREATE INDEX tnved_agent_sessions_created_idx ON tnved_agent_sessions (created_at DESC);
CREATE INDEX code_decisions_session_idx ON code_decisions (session_id);
