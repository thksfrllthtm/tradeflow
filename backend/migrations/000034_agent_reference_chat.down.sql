DELETE FROM tnved_agent_sessions WHERE mode = 'reference';
DROP INDEX IF EXISTS tnved_agent_sessions_reference_idx;
ALTER TABLE tnved_agent_sessions DROP COLUMN title, DROP COLUMN mode;
