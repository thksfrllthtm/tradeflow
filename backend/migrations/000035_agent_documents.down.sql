DELETE FROM tnved_agent_messages WHERE kind = 'documents';
ALTER TABLE tnved_agent_messages DROP CONSTRAINT tnved_agent_messages_kind_check;
ALTER TABLE tnved_agent_messages ADD CONSTRAINT tnved_agent_messages_kind_check
    CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'start', 'result', 'match', 'similar', 'error', 'notice'));
ALTER TABLE tnved_agent_sessions DROP COLUMN documents_code;
DROP TABLE web_pages;
