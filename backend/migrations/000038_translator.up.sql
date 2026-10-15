-- Раздел «Переводчик»: перевод Word-документов (.docx) по сегментам через ИИ с правкой на сайте
-- и глоссарием. Сегмент — один абзац w:p документа (в таблице —
-- абзац ячейки). Языки — коды из списка в коде (zh, ru, en, kk), список расширяется без миграции.

CREATE TABLE translator_documents (
    id BIGSERIAL PRIMARY KEY,
    -- Имя загруженного файла без «.docx»: из него же имя выгрузки «<имя>_<язык>.docx».
    name VARCHAR(255) NOT NULL CHECK (btrim(name) <> ''),
    source_lang VARCHAR(8) NOT NULL,
    target_lang VARCHAR(8) NOT NULL CHECK (target_lang <> source_lang),
    -- Исходный .docx (до 20 МБ): диск Render временный, поэтому файл хранится в базе.
    original_file BYTEA NOT NULL,
    -- parsed — разобран, не переводился; translating — идёт перевод (после перезапуска сервера
    -- продолжается сам); translated — все незаблокированные сегменты переведены; failed — ошибка.
    status VARCHAR(16) NOT NULL DEFAULT 'parsed' CHECK (status IN ('parsed', 'translating', 'translated', 'failed')),
    error TEXT NOT NULL DEFAULT '',
    -- Что не разбирается на сегменты и остаётся без перевода: {"textboxes": 2, "headers": 1, …}.
    skipped JSONB NOT NULL DEFAULT '{}',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE translator_segments (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES translator_documents(id) ON DELETE CASCADE,
    -- Порядковый номер сегмента в документе: по нему сегмент находится в document.xml при выгрузке.
    idx INTEGER NOT NULL CHECK (idx >= 0),
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('paragraph', 'table_cell')),
    -- Тексты с тегами <gN>…</gN> (фрагменты со своим форматированием) и <xN/> (табуляция,
    -- перенос строки, картинка в тексте и т. п.); '' — машинного перевода или правки ещё нет.
    source_text TEXT NOT NULL,
    mt_text TEXT NOT NULL DEFAULT '',
    target_text TEXT NOT NULL DEFAULT '',
    -- Теги: {"g0": "<w:rPr>…</w:rPr>", "g1": …} — форматирование фрагмента (g0 — основное),
    -- {"x2": "tab"} — вид элемента (tab, br, image, field, formula, other).
    tags JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(16) NOT NULL DEFAULT 'machine' CHECK (status IN ('machine', 'edited', 'confirmed', 'source_changed')),
    -- Правленный или подтверждённый сегмент автоперевод не перезаписывает.
    locked BOOLEAN NOT NULL DEFAULT false,
    -- Предупреждения проверки: [{"code": "numbers", "message": "…"}].
    qa JSONB NOT NULL DEFAULT '[]',
    edited_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (document_id, idx)
);

CREATE TABLE translator_segment_history (
    id BIGSERIAL PRIMARY KEY,
    segment_id BIGINT NOT NULL REFERENCES translator_segments(id) ON DELETE CASCADE,
    target_text TEXT NOT NULL,
    changed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX translator_segment_history_segment_idx ON translator_segment_history (segment_id, changed_at DESC);

CREATE TABLE translator_glossary (
    id BIGSERIAL PRIMARY KEY,
    source_lang VARCHAR(8) NOT NULL,
    target_lang VARCHAR(8) NOT NULL,
    term VARCHAR(500) NOT NULL CHECK (btrim(term) <> ''),
    translation VARCHAR(500) NOT NULL CHECK (btrim(translation) <> ''),
    note TEXT NOT NULL DEFAULT '',
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX translator_glossary_term_idx ON translator_glossary (source_lang, target_lang, lower(term));

-- Кэш машинного перевода: hash — sha256 от исходного текста, языков и модели.
CREATE TABLE translator_mt_cache (
    hash CHAR(64) PRIMARY KEY,
    translation TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- См. 000020_row_level_security: таблицы закрыты от Data API Supabase.
ALTER TABLE translator_documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE translator_segments ENABLE ROW LEVEL SECURITY;
ALTER TABLE translator_segment_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE translator_glossary ENABLE ROW LEVEL SECURITY;
ALTER TABLE translator_mt_cache ENABLE ROW LEVEL SECURITY;
