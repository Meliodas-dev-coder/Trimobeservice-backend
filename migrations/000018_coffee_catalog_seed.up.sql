-- 000018_coffee_catalog_seed.up.sql
-- Coffee reuses the phone catalog machinery: product -> variants (SKUs) -> cart
-- -> checkout -> order. A product's department is derived from its category's
-- template, so coffee products need one coffee-department category to attach to.
-- Admins manage only coffee *products*, so we seed the category and the
-- "Kafe Misiona" brand here (idempotently) rather than exposing screens for them.
--
-- The `SELECT ... FROM (SELECT ...) tmp WHERE NOT EXISTS (...)` form is the
-- portable MySQL idiom for an insert-if-absent (a WHERE needs a FROM, so the
-- literals are wrapped in a derived table). Running the migration twice is a
-- no-op.

INSERT INTO product_categories (name, slug, template_key, is_active)
SELECT * FROM (SELECT 'Coffee' AS name, 'coffee' AS slug, 'coffee' AS template_key, TRUE AS is_active) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM product_categories WHERE slug = 'coffee');

INSERT INTO brands (name, slug, department, is_active)
SELECT * FROM (SELECT 'Kafe Misiona' AS name, 'kafe-misiona' AS slug, 'coffee' AS department, TRUE AS is_active) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM brands WHERE slug = 'kafe-misiona');
