CREATE TABLE classifier_versions (
    id BIGSERIAL PRIMARY KEY,
    version_key VARCHAR(128) NOT NULL UNIQUE,
    dataset_kind VARCHAR(16) NOT NULL CHECK (dataset_kind IN ('real','demo')),
    source TEXT NOT NULL,
    source_date DATE,
    loaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived'))
);

CREATE TABLE classifier_codes (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES classifier_versions(id) ON DELETE CASCADE,
    code VARCHAR(32) NOT NULL,
    hierarchy_level INTEGER NOT NULL CHECK (hierarchy_level >= 0),
    parent_code VARCHAR(32) NOT NULL DEFAULT '',
    official_description TEXT NOT NULL,
    is_group BOOLEAN NOT NULL,
    valid_from DATE NOT NULL,
    valid_to DATE,
    source TEXT NOT NULL,
    UNIQUE(version_id, code),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);
CREATE INDEX classifier_codes_code_idx ON classifier_codes(code);

CREATE TABLE classifier_documents (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES classifier_versions(id) ON DELETE CASCADE,
    code_id BIGINT REFERENCES classifier_codes(id) ON DELETE CASCADE,
    parent_code VARCHAR(32) NOT NULL DEFAULT '',
    doc_type VARCHAR(32) NOT NULL CHECK (doc_type IN ('note','explanation','exception')),
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    source TEXT NOT NULL,
    valid_from DATE NOT NULL,
    valid_to DATE,
    CHECK (code_id IS NOT NULL OR parent_code <> ''),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE TABLE calculation_rules (
    id BIGSERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL REFERENCES classifier_versions(id) ON DELETE CASCADE,
    code_id BIGINT REFERENCES classifier_codes(id) ON DELETE CASCADE,
    parent_code VARCHAR(32) NOT NULL DEFAULT '',
    rule_key VARCHAR(128) NOT NULL,
    payment_type VARCHAR(64) NOT NULL,
    rate_type VARCHAR(32) NOT NULL CHECK (rate_type IN ('ad_valorem','specific','combined','fixed','unknown')),
    value NUMERIC(30,12),
    currency VARCHAR(3) NOT NULL DEFAULT '',
    unit VARCHAR(64) NOT NULL DEFAULT '',
    operation VARCHAR(32) NOT NULL DEFAULT '',
    conditions JSONB NOT NULL DEFAULT '{}'::jsonb,
    valid_from DATE NOT NULL,
    valid_to DATE,
    source TEXT NOT NULL,
    UNIQUE(version_id, rule_key),
    CHECK (code_id IS NOT NULL OR parent_code <> ''),
    CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CHECK (rate_type = 'unknown' OR value IS NOT NULL)
);
CREATE INDEX calculation_rules_lookup_idx ON calculation_rules(code_id, valid_from, valid_to);

-- Clearly isolated, non-authoritative fixture for local verification only.
INSERT INTO classifier_versions(version_key, dataset_kind, source, source_date, status)
VALUES ('demo-2026-09-08', 'demo', 'DEMO: встроенный набор для проверки интерфейса; не является официальным источником', DATE '2026-09-08', 'active');

INSERT INTO classifier_codes(version_id, code, hierarchy_level, parent_code, official_description, is_group, valid_from, source)
SELECT id, '0101', 2, '', 'DEMO-группа: живые животные', TRUE, DATE '2020-01-01', 'DEMO: неофициальный пример'
FROM classifier_versions WHERE version_key = 'demo-2026-09-08';
INSERT INTO classifier_codes(version_id, code, hierarchy_level, parent_code, official_description, is_group, valid_from, source)
SELECT id, '0101210000', 10, '0101', 'DEMO-конечный код: лошади (пример)', FALSE, DATE '2020-01-01', 'DEMO: неофициальный пример'
FROM classifier_versions WHERE version_key = 'demo-2026-09-08';
INSERT INTO classifier_codes(version_id, code, hierarchy_level, parent_code, official_description, is_group, valid_from, source)
SELECT id, '000123', 6, '', 'DEMO-конечный код с ведущими нулями', FALSE, DATE '2020-01-01', 'DEMO: неофициальный пример'
FROM classifier_versions WHERE version_key = 'demo-2026-09-08';

INSERT INTO classifier_documents(version_id, code_id, doc_type, title, content, source, valid_from)
SELECT v.id, c.id, 'note', 'Демонстрационное примечание', 'Проверочный текст. Не заменяет юридические пояснения.', 'DEMO: неофициальный пример', DATE '2020-01-01'
FROM classifier_versions v JOIN classifier_codes c ON c.version_id = v.id AND c.code = '0101210000'
WHERE v.version_key = 'demo-2026-09-08';

INSERT INTO calculation_rules(version_id, code_id, rule_key, payment_type, rate_type, value, currency, unit, operation, conditions, valid_from, source)
SELECT v.id, c.id, 'demo-duty-0101210000', 'Таможенная пошлина', 'ad_valorem', 5, '', '%', '', '{"origin":["DEMO"]}'::jsonb, DATE '2020-01-01', 'DEMO: неофициальный пример'
FROM classifier_versions v JOIN classifier_codes c ON c.version_id = v.id AND c.code = '0101210000'
WHERE v.version_key = 'demo-2026-09-08';
