-- Стоимостные индикаторы риска (СИР) КГД РК: приложения к профилям риска (файлы «24_прил CN»,
-- «25_прил CN», «26_прил CN»): сколько долларов США стоит 1 кг товара по коду ТН ВЭД для групп
-- стран происхождения. Смета по СИР считает таможенную стоимость как вес × индикатор.
-- Один профиль — одна загрузка файла; новая загрузка того же профиля заменяет его.
CREATE TABLE risk_indicator_profiles (
    id BIGSERIAL PRIMARY KEY,
    -- Номер профиля риска из шапки файла: «50100/190318/00024».
    number VARCHAR(60) NOT NULL UNIQUE CHECK (btrim(number) <> ''),
    file_name VARCHAR(255) NOT NULL DEFAULT '',
    -- Группы стран по колонкам файла: [["AD","AG",…],["CN"],["00"],…]; «00» — страна неизвестна.
    groups JSONB NOT NULL,
    code_count INTEGER NOT NULL CHECK (code_count >= 0),
    imported_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE risk_indicators (
    profile_id BIGINT NOT NULL REFERENCES risk_indicator_profiles(id) ON DELETE CASCADE,
    code VARCHAR(10) NOT NULL CHECK (code ~ '^[0-9]{10}$'),
    -- Индикатор по группам стран в порядке groups профиля, USD за 1 кг; null — «-» (индикатора нет).
    prices JSONB NOT NULL,
    PRIMARY KEY (profile_id, code)
);
CREATE INDEX risk_indicators_code_idx ON risk_indicators (code);

-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE risk_indicator_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE risk_indicators ENABLE ROW LEVEL SECURITY;
