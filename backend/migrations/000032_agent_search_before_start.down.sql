UPDATE tnved_agent_sessions SET status = 'cancelled' WHERE status IN ('searching', 'ready');
ALTER TABLE tnved_agent_sessions DROP CONSTRAINT tnved_agent_sessions_status_check;
ALTER TABLE tnved_agent_sessions ADD CONSTRAINT tnved_agent_sessions_status_check
    CHECK (status IN ('running', 'waiting', 'done', 'error', 'cancelled'));

DELETE FROM tnved_agent_messages WHERE kind = 'start';
ALTER TABLE tnved_agent_messages DROP CONSTRAINT tnved_agent_messages_kind_check;
ALTER TABLE tnved_agent_messages ADD CONSTRAINT tnved_agent_messages_kind_check
    CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'result', 'match', 'similar', 'error', 'notice'));
