ALTER TABLE product_directory_entries
    ADD COLUMN company_coefficient NUMERIC(30,12)
        CHECK (company_coefficient IS NULL OR company_coefficient >= 0);
