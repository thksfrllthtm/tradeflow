-- «/троис» в чате-справочнике — вопрос в истории, как обычный: проверка бренда по ТРОИС идёт в фоне.
-- trademark_hits: NULL — не проверка ТРОИС; число — сколько знаков реестра найдено (0 — нет в реестре).
ALTER TABLE tnved_agent_sessions ADD COLUMN trademark_hits INTEGER CHECK (trademark_hits >= 0);
