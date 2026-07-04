-- 000011_product_templates.up.sql
-- Category-driven product attributes. A category picks a product "type" template
-- (phone, laptop, audio…, defined in Go); products of that category carry the
-- template's specs in a JSON `attributes` column. Filterable specs are denormalised
-- into product_facets for fast storefront faceting.

ALTER TABLE product_categories
    ADD COLUMN template_key VARCHAR(32) NOT NULL DEFAULT 'generic' AFTER slug;

ALTER TABLE products
    ADD COLUMN attributes JSON DEFAULT NULL AFTER description;

-- Denormalised, write-time index of filterable spec values (product-level specs
-- and variant axis values), rebuilt whenever a product or its variants change.
CREATE TABLE product_facets (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    product_id  BIGINT UNSIGNED NOT NULL,
    facet_key   VARCHAR(48)  NOT NULL,
    facet_value VARCHAR(128) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_product_facets_kv (facet_key, facet_value),
    KEY idx_product_facets_product (product_id),
    CONSTRAINT fk_product_facets_product FOREIGN KEY (product_id)
        REFERENCES products (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
