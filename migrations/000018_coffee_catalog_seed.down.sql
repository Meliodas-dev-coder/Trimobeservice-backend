-- 000018_coffee_catalog_seed.down.sql
-- Remove the seeded coffee brand + category. Fails (by FK RESTRICT) if coffee
-- products still reference the category — delete those products first.

DELETE FROM brands WHERE slug = 'kafe-misiona' AND department = 'coffee';
DELETE FROM product_categories WHERE slug = 'coffee' AND template_key = 'coffee';
