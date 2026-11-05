-- Доступ к вопросу чата на главной для другого сотрудника: автор (или администратор) открывает вопрос коллеге,
-- тот видит его в своей истории и только читает — уточнять, перезапускать и удалять может только автор.
CREATE TABLE tnved_agent_session_shares (
    session_id BIGINT NOT NULL REFERENCES tnved_agent_sessions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shared_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, user_id)
);
CREATE INDEX tnved_agent_session_shares_user_idx ON tnved_agent_session_shares (user_id);

-- См. 000020_row_level_security: таблица закрыта от Data API Supabase.
ALTER TABLE tnved_agent_session_shares ENABLE ROW LEVEL SECURITY;
