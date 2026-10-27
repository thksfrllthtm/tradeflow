-- Заголовки прошлых вопросов справочника — краткое наименование товара из ответа агента
-- (новые вопросы получают его сами при ответе).
UPDATE tnved_agent_sessions
SET title = left(regexp_replace(btrim(result->>'name'), '\s+', ' ', 'g'), 80)
WHERE mode = 'reference' AND btrim(coalesce(result->>'name', '')) <> '';
