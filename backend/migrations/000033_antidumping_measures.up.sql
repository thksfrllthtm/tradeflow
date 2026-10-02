-- Действующие антидемпинговые (и другие защитные) меры ЕАЭС из реестра ЕЭК
-- (remedies.eaeunion.org). codes — коды ТН ВЭД меры цифрами (10 знаков — точный код, короче —
-- товарная позиция), countries — страны-экспортёры (ISO), rates — ставка «для прочих
-- производителей» по стране, % от таможенной стоимости. suspended — мера временно не применяется.
CREATE TABLE tnved_antidumping_measures (
    id BIGSERIAL PRIMARY KEY,
    number VARCHAR(20) NOT NULL UNIQUE,
    measure_type VARCHAR(8) NOT NULL DEFAULT 'AD',
    short_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    codes JSONB NOT NULL DEFAULT '[]'::jsonb,
    countries JSONB NOT NULL DEFAULT '[]'::jsonb,
    rates JSONB NOT NULL DEFAULT '{}'::jsonb,
    rate_text TEXT NOT NULL DEFAULT '',
    min_percent NUMERIC(7,3),
    max_percent NUMERIC(7,3),
    start_date DATE,
    end_date DATE,
    suspended BOOLEAN NOT NULL DEFAULT false,
    needs_check TEXT NOT NULL DEFAULT '',
    foundation TEXT NOT NULL DEFAULT '',
    document_url TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Загрузки реестра: из файла программы (seed) или с сайта ЕЭК по кнопке администратора.
CREATE TABLE tnved_antidumping_imports (
    id BIGSERIAL PRIMARY KEY,
    source VARCHAR(16) NOT NULL CHECK (source IN ('seed', 'eec')),
    measure_count INTEGER NOT NULL,
    imported_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
