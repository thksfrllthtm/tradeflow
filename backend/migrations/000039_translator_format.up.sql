-- «Переводчик»: кроме Word (.docx) переводятся книги Excel (.xlsx). Формат определяет разбор
-- и выгрузку; у Excel нет двуязычной выгрузки.
ALTER TABLE translator_documents ADD COLUMN format VARCHAR(8) NOT NULL DEFAULT 'docx' CHECK (format IN ('docx', 'xlsx'));
