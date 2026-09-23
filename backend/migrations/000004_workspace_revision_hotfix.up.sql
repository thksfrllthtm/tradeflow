-- Migration 000003 was already applied before the workspace revision field
-- was introduced in the model. Add it separately so existing installations
-- are upgraded without losing saved workspace data.
ALTER TABLE import_workspaces
    ADD COLUMN IF NOT EXISTS revision INTEGER NOT NULL DEFAULT 1;

ALTER TABLE import_workspaces
    DROP CONSTRAINT IF EXISTS import_workspaces_revision_check;

ALTER TABLE import_workspaces
    ADD CONSTRAINT import_workspaces_revision_check CHECK (revision > 0);
