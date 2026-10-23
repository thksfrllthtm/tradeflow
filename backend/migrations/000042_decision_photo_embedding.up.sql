-- Вектор описания фото записи базы решений (эмбеддинг gemini-embedding-001, 768 чисел): похожие товары
-- и поиск текстом находятся по смыслу, а не только по совпадению слов («ключ для труб» → «газовый ключ»).
-- Считается в фоне из photo_features; NULL — ещё не посчитан или эмбеддинги выключены
-- (TNVED_AI_EMBED_MODEL пуст). Индекса нет: записей сотни, перебор быстрее.
-- Локально нужен образ Postgres с pgvector (docker-compose: pgvector/pgvector:pg17); в Supabase расширение есть.
CREATE EXTENSION IF NOT EXISTS vector;
ALTER TABLE code_decisions ADD COLUMN photo_embedding vector(768);
