-- Перечень РК: коды, по которым Казахстан по обязательствам ВТО применяет ставки ввозной
-- пошлины ниже ЕТТ ЕАЭС (Решение Совета ЕЭК от 14.10.2015 № 59). Заполняется из PDF ЕЭК
-- (разбор в браузере) или из Excel; каждая загрузка заменяет список целиком.
CREATE TABLE tnved_kz_rates (
    code VARCHAR(10) PRIMARY KEY CHECK (code ~ '^[0-9]{10}$'),
    name TEXT NOT NULL DEFAULT '',
    -- Ставка как в перечне: «8», «5, но не менее 0,03 евро за 1 кг».
    rate_text VARCHAR(300) NOT NULL CHECK (btrim(rate_text) <> ''),
    -- other — форма, которой нет в справочнике («… или …, в зависимости от того, что ниже»).
    duty_type VARCHAR(24) NOT NULL CHECK (duty_type IN ('percentage', 'specific', 'combined_minimum', 'other')),
    duty_percent NUMERIC(7,3) CHECK (duty_percent IS NULL OR duty_percent >= 0),
    duty_amount NUMERIC(14,4) CHECK (duty_amount IS NULL OR duty_amount >= 0),
    duty_currency VARCHAR(3) NOT NULL DEFAULT '',
    duty_unit VARCHAR(60) NOT NULL DEFAULT '',
    -- Период действия ставки РК по сноскам: «применяется с …» — valid_from,
    -- «ставка ЕТТ применяется с …» — valid_to (день до этой даты). Пусто — без ограничения.
    valid_from DATE,
    valid_to DATE,
    notes VARCHAR(60) NOT NULL DEFAULT '',
    notes_text TEXT NOT NULL DEFAULT '',
    page INTEGER,
    CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from <= valid_to)
);

CREATE TABLE tnved_kz_rate_imports (
    id BIGSERIAL PRIMARY KEY,
    source VARCHAR(10) NOT NULL CHECK (source IN ('pdf', 'excel')),
    file_name VARCHAR(255) NOT NULL DEFAULT '',
    row_count INTEGER NOT NULL CHECK (row_count >= 0),
    imported_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE tnved_kz_rates ENABLE ROW LEVEL SECURITY;
ALTER TABLE tnved_kz_rate_imports ENABLE ROW LEVEL SECURITY;
