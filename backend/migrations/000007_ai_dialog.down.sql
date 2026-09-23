DROP TABLE IF EXISTS product_ai_messages;
DROP TABLE IF EXISTS product_ai_dialogues;
ALTER TABLE imported_products DROP COLUMN IF EXISTS ai_data_revision;
