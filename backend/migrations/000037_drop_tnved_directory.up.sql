-- Справочник ТН ВЭД удалён с сайта: ставки и описание кода берутся из номенклатуры ТН ВЭД ЕАЭС,
-- а карточка товара хранит свой код сама (раньше — ссылкой на карточку справочника ТН ВЭД).
ALTER TABLE product_directory_entries ADD COLUMN code VARCHAR(10) NOT NULL DEFAULT '';
UPDATE product_directory_entries p SET code = t.code
FROM tnved_directory_entries t
WHERE t.id = p.tnved_directory_entry_id;
ALTER TABLE product_directory_entries
    ADD CONSTRAINT product_directory_code_check CHECK (code = '' OR code ~ '^[0-9]{10}$');
CREATE INDEX product_directory_code_idx ON product_directory_entries(code);

DROP INDEX IF EXISTS product_directory_tnved_idx;
ALTER TABLE product_directory_entries DROP COLUMN tnved_directory_entry_id;
DROP TABLE tnved_directory_entries;
