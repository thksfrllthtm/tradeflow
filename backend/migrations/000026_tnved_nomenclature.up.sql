-- Полная номенклатура ТН ВЭД ЕАЭС со ставками ввозной пошлины ЕТТ — справка, из которой
-- код попадает в рабочий справочник (tnved_directory_entries). Сейчас заполняется из
-- Excel-выгрузки TWS.BY; каждая загрузка заменяет список целиком.
CREATE TABLE tnved_nomenclature (
    code VARCHAR(10) PRIMARY KEY CHECK (code ~ '^[0-9]{10}$'),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    -- Ставка как в источнике: «12%», «50%, но не менее 1 EUR за 1 кг».
    tariff_text VARCHAR(200) NOT NULL DEFAULT '',
    -- Разбор tariff_text; combined_plus — «процент плюс сумма за единицу»,
    -- unknown — пусто или форма, которую не удалось разобрать.
    duty_type VARCHAR(24) NOT NULL CHECK (duty_type IN ('unknown', 'percentage', 'specific', 'combined_minimum', 'combined_plus')),
    duty_percent NUMERIC(7,3) CHECK (duty_percent IS NULL OR duty_percent >= 0),
    duty_amount NUMERIC(14,4) CHECK (duty_amount IS NULL OR duty_amount >= 0),
    duty_currency VARCHAR(3) NOT NULL DEFAULT '',
    duty_unit VARCHAR(40) NOT NULL DEFAULT ''
);

-- Каждая загрузка: откуда, на какую дату актуальны данные, сколько кодов и что изменилось.
CREATE TABLE tnved_nomenclature_imports (
    id BIGSERIAL PRIMARY KEY,
    source VARCHAR(100) NOT NULL,
    file_name VARCHAR(255) NOT NULL DEFAULT '',
    actual_date DATE,
    code_count INTEGER NOT NULL CHECK (code_count >= 0),
    added_count INTEGER NOT NULL DEFAULT 0,
    removed_count INTEGER NOT NULL DEFAULT 0,
    changed_count INTEGER NOT NULL DEFAULT 0,
    imported_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE tnved_nomenclature ENABLE ROW LEVEL SECURITY;
ALTER TABLE tnved_nomenclature_imports ENABLE ROW LEVEL SECURITY;
