-- 000011_product_templates.down.sql

DROP TABLE IF EXISTS product_facets;

ALTER TABLE products
    DROP COLUMN attributes;

ALTER TABLE product_categories
    DROP COLUMN template_key;
