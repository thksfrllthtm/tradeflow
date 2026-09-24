-- Supabase открывает таблицы схемы public через свой Data API (PostgREST) ролям anon
-- и authenticated. Приложение этим API не пользуется, поэтому включаем RLS без политик:
-- через Data API строки не видны, а backend подключается владельцем таблиц и RLS
-- не затрагивается. Новым таблицам в следующих миграциях нужен свой
-- ALTER TABLE ... ENABLE ROW LEVEL SECURITY.
DO $$
DECLARE
    table_name text;
BEGIN
    FOR table_name IN SELECT tablename FROM pg_tables WHERE schemaname = current_schema() LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    END LOOP;
END $$;
