-- Подбор кода начинается с поиска в базе решений (точное совпадение и похожие по фото) без
-- агента: searching — идёт поиск, ready — поиск закончен, агент запускается кнопкой сотрудника.
ALTER TABLE tnved_agent_sessions DROP CONSTRAINT tnved_agent_sessions_status_check;
ALTER TABLE tnved_agent_sessions ADD CONSTRAINT tnved_agent_sessions_status_check
    CHECK (status IN ('searching', 'ready', 'running', 'waiting', 'done', 'error', 'cancelled'));

-- start — сотрудник запустил агента после поиска в базе (считается в дневной лимит запусков).
ALTER TABLE tnved_agent_messages DROP CONSTRAINT tnved_agent_messages_kind_check;
ALTER TABLE tnved_agent_messages ADD CONSTRAINT tnved_agent_messages_kind_check
    CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'start', 'result', 'match', 'similar', 'error', 'notice'));
