-- Результат последнего обновления справочника (страница «Обновление данных»): то, что администратор увидел после
-- загрузки, — сводка и изменённые коды или меры. Одна строка на набор: каждая новая загрузка заменяет её.
CREATE TABLE data_update_results (
    dataset VARCHAR(20) PRIMARY KEY CHECK (dataset IN ('nomenclature', 'kz', 'antidumping', 'trademarks')),
    result JSONB NOT NULL,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- См. 000020_row_level_security: таблица закрыта от Data API Supabase.
ALTER TABLE data_update_results ENABLE ROW LEVEL SECURITY;
