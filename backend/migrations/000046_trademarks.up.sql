-- Таможенный реестр объектов интеллектуальной собственности КГД РК (ТРОИС): действующие записи из файла
-- КГД. Новая загрузка заменяет реестр целиком; надписи логотипов, прочитанные ИИ или введённые
-- администратором, переносятся по регистрационному номеру.
CREATE TABLE trademarks (
    -- Регистрационный номер по реестру: «00001/ТЗ-260905».
    reg_no VARCHAR(40) PRIMARY KEY CHECK (btrim(reg_no) <> ''),
    -- Наименование знака; пусто — знак только изображением (logo).
    name VARCHAR(300) NOT NULL DEFAULT '',
    -- Изображение знака: data URL JPEG до 400 px; null — картинки нет.
    logo TEXT,
    -- Надпись или бренд логотипа: прочитал ИИ (logo_name_source = 'ai') или ввёл администратор ('admin').
    logo_name VARCHAR(300) NOT NULL DEFAULT '',
    logo_name_source VARCHAR(10) NOT NULL DEFAULT '' CHECK (logo_name_source IN ('', 'ai', 'admin')),
    -- Когда ИИ смотрел логотип (даже если надпись не прочитал); null — ещё не смотрел.
    logo_read_at TIMESTAMPTZ,
    goods TEXT NOT NULL DEFAULT '',
    -- Коды ТН ВЭД из колонки товаров через пробел: «170410 170490».
    codes TEXT NOT NULL DEFAULT '',
    holder TEXT NOT NULL DEFAULT '',
    document TEXT NOT NULL DEFAULT '',
    protected_until DATE,
    representatives TEXT NOT NULL DEFAULT '',
    imported_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    imported_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX trademarks_logo_unread_idx ON trademarks (reg_no) WHERE logo IS NOT NULL AND logo_read_at IS NULL;

-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE trademarks ENABLE ROW LEVEL SECURITY;
