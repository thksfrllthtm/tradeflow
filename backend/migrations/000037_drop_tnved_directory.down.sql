-- Карточки справочника ТН ВЭД не восстанавливаются: код, который с ними работал, удалён.
-- Данные можно вернуть только из резервной копии базы или из выгрузки Excel справочника.
ALTER TABLE product_directory_entries DROP CONSTRAINT IF EXISTS product_directory_code_check;
DROP INDEX IF EXISTS product_directory_code_idx;
ALTER TABLE product_directory_entries DROP COLUMN IF EXISTS code;
