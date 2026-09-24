DO $$
DECLARE
    table_name text;
BEGIN
    FOR table_name IN SELECT tablename FROM pg_tables WHERE schemaname = current_schema() LOOP
        EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', table_name);
    END LOOP;
END $$;
