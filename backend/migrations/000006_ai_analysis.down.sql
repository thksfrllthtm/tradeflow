DROP TABLE IF EXISTS product_ai_analyses;
ALTER TABLE imported_products DROP CONSTRAINT IF EXISTS imported_products_tn_ved_code_format;
ALTER TABLE imported_products DROP COLUMN IF EXISTS tn_ved_code_source;
ALTER TABLE imported_products DROP COLUMN IF EXISTS tn_ved_code;
