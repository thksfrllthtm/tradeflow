-- Расширение vector не удаляем: им могут пользоваться другие таблицы.
ALTER TABLE code_decisions DROP COLUMN IF EXISTS photo_embedding;
