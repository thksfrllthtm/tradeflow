-- Чат-справочник на главной: сессия агента без строки сметы (mode = 'reference'), заголовок —
-- первый вопрос сотрудника, для списка прошлых вопросов.
ALTER TABLE tnved_agent_sessions
    ADD COLUMN mode VARCHAR(16) NOT NULL DEFAULT 'row' CHECK (mode IN ('row', 'reference')),
    ADD COLUMN title TEXT NOT NULL DEFAULT '';
CREATE INDEX tnved_agent_sessions_reference_idx ON tnved_agent_sessions (user_id, updated_at DESC) WHERE mode = 'reference';
