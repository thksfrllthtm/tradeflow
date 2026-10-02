DELETE FROM tnved_agent_messages WHERE kind = 'similar';
ALTER TABLE tnved_agent_messages DROP CONSTRAINT tnved_agent_messages_kind_check;
ALTER TABLE tnved_agent_messages ADD CONSTRAINT tnved_agent_messages_kind_check
    CHECK (kind IN ('input', 'text', 'tool', 'question', 'user', 'rerun', 'result', 'match', 'error', 'notice'));

DROP INDEX IF EXISTS code_decisions_photo_features_pending_idx;
ALTER TABLE code_decisions DROP COLUMN photo_features_at, DROP COLUMN photo_features;
