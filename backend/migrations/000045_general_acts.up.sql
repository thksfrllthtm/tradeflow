-- Общая база «Документов для ввоза»: перечни продукции, при ввозе которой нужен сертификат или
-- декларация о соответствии (СС/ДС). Основной список задан в программе; сюда добавляются перечни из
-- каждой открытой справки tnved.info, так что база растёт сама. Тексты актов — в web_pages.
CREATE TABLE general_acts (
    url VARCHAR(1000) PRIMARY KEY,
    title TEXT NOT NULL,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE general_acts ENABLE ROW LEVEL SECURITY;
