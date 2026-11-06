-- Журнал действий сотрудников для администратора: кто, что, над какой записью, с каким итогом.
-- Пишутся только изменения и вход/выход, без тела запроса. user_id без внешнего ключа и email на момент
-- действия: удаление сотрудника не стирает его историю. Повторы подряд (автосохранение) сливаются в одну
-- строку со счётчиком count, updated_at — время последнего повтора.
CREATE TABLE user_actions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT,
    email TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL,
    route TEXT NOT NULL,
    target TEXT NOT NULL DEFAULT '',
    status INT NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    count INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX user_actions_updated_idx ON user_actions (updated_at DESC);
CREATE INDEX user_actions_user_idx ON user_actions (user_id, id DESC);

-- См. 000020_row_level_security: таблица закрыта от Data API Supabase.
ALTER TABLE user_actions ENABLE ROW LEVEL SECURITY;
