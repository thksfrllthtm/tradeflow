ALTER TABLE users
    ADD COLUMN name VARCHAR(120) NOT NULL DEFAULT '';

-- Keep legacy rows compatible with the same trim/lower normalization used by
-- the API. The unique index prevents case variants from being registered as
-- separate accounts in future direct SQL imports as well.
UPDATE users
SET email = lower(btrim(email));

CREATE UNIQUE INDEX users_email_normalized_idx ON users (lower(email));
