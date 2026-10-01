DROP TABLE IF EXISTS tnved_details_pages;
DROP TABLE IF EXISTS tnved_agent_messages;
ALTER TABLE IF EXISTS code_decisions DROP CONSTRAINT IF EXISTS code_decisions_session_fk;
DROP TABLE IF EXISTS tnved_agent_sessions;
DROP TABLE IF EXISTS code_decisions;
