-- Подбор наименования и цены керамической посуды — отдельный модуль, не связанный
-- со сметами и расчётом растаможки.
--
-- В базе компании у марки три поля: наименование, марка и цена. Наименование пишется
-- как «Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘».
-- Тип, размер и китайскую часть backend выделяет из наименования при каждом сохранении:
-- по ним ищется цена (тип + размер) и список типов для ИИ. Вручную их не вводят.
CREATE TABLE ceramic_products (
    id BIGSERIAL PRIMARY KEY,
    mark VARCHAR(128) NOT NULL CHECK (btrim(mark) <> ''),
    -- Марка для точного поиска: верхний регистр, без пробелов, полуширинные символы.
    mark_key VARCHAR(128) NOT NULL UNIQUE,
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    price NUMERIC(14,4) NOT NULL CHECK (price > 0),
    -- Выделено из name: русская часть без размера («Керамическая фруктовница для взрослых»).
    product_type VARCHAR(500) NOT NULL,
    -- product_type без регистра и лишних пробелов — ключ группировки.
    product_type_key VARCHAR(500) NOT NULL,
    size_value NUMERIC(10,2) CHECK (size_value IS NULL OR size_value > 0),
    size_unit VARCHAR(8) CHECK (size_unit IS NULL OR size_unit IN ('inch', 'cm', 'mm', 'ml', 'l')),
    -- Выделено из name: часть после «/» с китайскими иероглифами.
    chinese_name TEXT NOT NULL DEFAULT '',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((size_value IS NULL) = (size_unit IS NULL))
);

-- Подбор цены: все записи одного типа и размера.
CREATE INDEX ceramic_products_type_size_idx ON ceramic_products (product_type_key, size_unit, size_value);

-- См. 000020_row_level_security: таблица закрыта от Data API Supabase.
ALTER TABLE ceramic_products ENABLE ROW LEVEL SECURITY;
