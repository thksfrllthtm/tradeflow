DELETE FROM translator_documents WHERE format = 'xlsx';
ALTER TABLE translator_documents DROP COLUMN format;
