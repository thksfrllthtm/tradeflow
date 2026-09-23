DROP INDEX IF EXISTS users_email_normalized_idx;

ALTER TABLE users
    DROP COLUMN IF EXISTS name;
