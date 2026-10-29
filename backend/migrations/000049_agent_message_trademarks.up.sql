-- Ответ «/троис» в ленте чата-справочника — сообщение trademarks.
ALTER TABLE tnved_agent_messages DROP CONSTRAINT tnved_agent_messages_kind_check;
ALTER TABLE tnved_agent_messages ADD CONSTRAINT tnved_agent_messages_kind_check
    CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'start', 'documents', 'result', 'match', 'similar', 'error', 'notice', 'trademarks'));
