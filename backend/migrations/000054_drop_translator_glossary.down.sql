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
ALTER TABLE translator_glossary ENABLE ROW LEVEL SECURITY;
