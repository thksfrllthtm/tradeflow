-- «Документы для ввоза»: агент читает справку по товару tnved.info и акты по ссылкам из неё.
-- web_pages — кэш страниц (текст без меню и подвала, ссылки с подписями): справка и акты меняются
-- редко, а у справки tnved.info без входа маленький дневной лимит запросов.
CREATE TABLE web_pages (
    url VARCHAR(1000) PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL,
    links JSONB NOT NULL DEFAULT '[]',
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE web_pages ENABLE ROW LEVEL SECURITY;

-- documents_code — запуск агента в режиме «документы» для этого кода ('' — обычный подбор).
ALTER TABLE tnved_agent_sessions ADD COLUMN documents_code VARCHAR(10) NOT NULL DEFAULT '';

-- documents — сотрудник нажал «Документы для ввоза» (считается в дневной лимит запусков).
ALTER TABLE tnved_agent_messages DROP CONSTRAINT tnved_agent_messages_kind_check;
ALTER TABLE tnved_agent_messages ADD CONSTRAINT tnved_agent_messages_kind_check
    CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'start', 'documents', 'result', 'match', 'similar', 'error', 'notice'));
