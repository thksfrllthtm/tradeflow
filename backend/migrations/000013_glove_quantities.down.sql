ALTER TABLE imported_products
    DROP COLUMN IF EXISTS pairs,
    DROP COLUMN IF EXISTS places,
    DROP COLUMN IF EXISTS pairs_per_place;
