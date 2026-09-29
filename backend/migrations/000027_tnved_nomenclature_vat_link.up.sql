-- НДС Казахстана для кода (основная ставка 16%) и ссылка «Подробнее» на страницу кода
-- в источнике (у TWS.BY — https://www.tws.by/tws/tnved/code/<id>).
ALTER TABLE tnved_nomenclature
    ADD COLUMN vat_percent NUMERIC(5,2) NOT NULL DEFAULT 16 CHECK (vat_percent >= 0 AND vat_percent <= 100),
    ADD COLUMN details_url VARCHAR(500) NOT NULL DEFAULT '';
