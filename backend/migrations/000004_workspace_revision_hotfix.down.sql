ALTER TABLE import_workspaces
    DROP CONSTRAINT IF EXISTS import_workspaces_revision_check;

ALTER TABLE import_workspaces
    DROP COLUMN IF EXISTS revision;
