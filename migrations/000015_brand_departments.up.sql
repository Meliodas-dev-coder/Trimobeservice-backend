-- 000015_brand_departments.up.sql
-- Split the product catalog into departments (tech vs fashion) for separate
-- management. A category's department is inherited from its product-type
-- template (defined in Go), and a product from its category; brands have no
-- template link, so they carry their own department column. Existing brands
-- default to 'tech' — the catalog started as the phone/accessory store.

ALTER TABLE brands
    ADD COLUMN department VARCHAR(32) NOT NULL DEFAULT 'tech' AFTER slug;
