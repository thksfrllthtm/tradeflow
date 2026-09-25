-- Удаление старого модуля «импорт → товары → классификатор → расчёт», ИИ-анализа
-- товаров (Anthropic/Gemini) и черновика перчаток без сметы. Сайт ими не пользуется:
-- сметы хранятся в gloves_estimates и gloves_copy_estimates, справочники —
-- в tnved_directory_entries и product_directory_entries.
DROP TABLE IF EXISTS product_ai_messages;
DROP TABLE IF EXISTS product_ai_dialogues;
DROP TABLE IF EXISTS product_ai_analyses;
DROP TABLE IF EXISTS imported_product_images;
DROP TABLE IF EXISTS import_assets;
DROP TABLE IF EXISTS import_workspaces;
DROP TABLE IF EXISTS imported_products;
DROP TABLE IF EXISTS estimate_imports;
DROP TABLE IF EXISTS customs_calculation_revisions;
DROP TABLE IF EXISTS customs_calculations;
DROP TABLE IF EXISTS estimates;
DROP TABLE IF EXISTS calculation_rules;
DROP TABLE IF EXISTS classifier_documents;
DROP TABLE IF EXISTS classifier_codes;
DROP TABLE IF EXISTS classifier_versions;
DROP TABLE IF EXISTS gloves_calculation_drafts;
