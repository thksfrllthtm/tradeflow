-- agent_started_at — когда агент впервые взялся за подбор (после описания фото и поиска в базе): время до
-- итога в журнале считается от него, одинаково с тенью, которая стартует в тот же момент.
ALTER TABLE tnved_agent_sessions ADD COLUMN agent_started_at TIMESTAMPTZ;
