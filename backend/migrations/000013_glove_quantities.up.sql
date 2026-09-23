ALTER TABLE imported_products
    ADD COLUMN pairs NUMERIC(30,12),
    ADD COLUMN places NUMERIC(30,12),
    ADD COLUMN pairs_per_place NUMERIC(30,12);
