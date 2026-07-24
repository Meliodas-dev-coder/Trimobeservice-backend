-- samsung_phone_cases.sql
-- Phone-case stock, transcribed from the handwritten "Housse SAMSUNG" sheet.
--
-- Same model as seeds/redmi_phone_cases.sql: the shared 'Phone cases' category
-- (template_key 'accessory', which is what puts it in the `tech` department),
-- one product per handset, one variant/SKU per colour.
--
-- Every row on this sheet has as many colours as its count, so nothing here is
-- ambiguous: 15 products, 22 variants, 22 pieces.
--
-- Worth noting: the S22 Ultra appears TWICE on the sheet — grey at 15.000 Ar and
-- an armoured black-and-red at 25.000 Ar. That is one product with two variants
-- at different prices, which is exactly why price lives on the variant and not
-- on the product.
--
-- Re-runnable: every insert is guarded on a unique key.

START TRANSACTION;

-- --- 1. Shared category (no-op if seeded by another sheet) -------------------

INSERT INTO product_categories (name, slug, template_key, description, is_active)
SELECT * FROM (
    SELECT 'Phone cases'  AS name,
           'phone-cases'  AS slug,
           'accessory'    AS template_key,
           'Protective cases and covers, listed per handset model.' AS description,
           TRUE           AS is_active
) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM product_categories WHERE slug = 'phone-cases');

-- --- 2. Brand ---------------------------------------------------------------

INSERT INTO brands (name, slug, department, is_active)
SELECT * FROM (
    SELECT 'Samsung' AS name, 'samsung' AS slug, 'tech' AS department, TRUE AS is_active
) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM brands WHERE slug = 'samsung');

SET @category_id = (SELECT id FROM product_categories WHERE slug = 'phone-cases');
SET @brand_id    = (SELECT id FROM brands WHERE slug = 'samsung');

-- --- 3. Products ------------------------------------------------------------

INSERT INTO products (category_id, brand_id, name, slug, description, is_active)
SELECT @category_id, @brand_id, t.name, t.slug,
       CONCAT('Housse de protection pour ', t.model, '.'), TRUE
FROM (
    SELECT 'Housse Samsung A20'        AS name, 'housse-samsung-a20'        AS slug, 'Samsung A20'        AS model
    UNION ALL SELECT 'Housse Samsung A23',       'housse-samsung-a23',       'Samsung A23'
    UNION ALL SELECT 'Housse Samsung A50',       'housse-samsung-a50',       'Samsung A50'
    UNION ALL SELECT 'Housse Samsung A42',       'housse-samsung-a42',       'Samsung A42'
    UNION ALL SELECT 'Housse Samsung A32 5G',    'housse-samsung-a32-5g',    'Samsung A32 5G'
    UNION ALL SELECT 'Housse Samsung A05s',      'housse-samsung-a05s',      'Samsung A05s'
    UNION ALL SELECT 'Housse Samsung S10',       'housse-samsung-s10',       'Samsung S10'
    UNION ALL SELECT 'Housse Samsung S20 Plus',  'housse-samsung-s20-plus',  'Samsung S20 Plus'
    UNION ALL SELECT 'Housse Samsung S22 Ultra', 'housse-samsung-s22-ultra', 'Samsung S22 Ultra'
    UNION ALL SELECT 'Housse Samsung S23',       'housse-samsung-s23',       'Samsung S23'
    UNION ALL SELECT 'Housse Samsung S23 Plus',  'housse-samsung-s23-plus',  'Samsung S23 Plus'
    UNION ALL SELECT 'Housse Samsung S23 Ultra', 'housse-samsung-s23-ultra', 'Samsung S23 Ultra'
    UNION ALL SELECT 'Housse Samsung S24',       'housse-samsung-s24',       'Samsung S24'
    UNION ALL SELECT 'Housse Samsung S24 Plus',  'housse-samsung-s24-plus',  'Samsung S24 Plus'
    UNION ALL SELECT 'Housse Samsung S25 Ultra', 'housse-samsung-s25-ultra', 'Samsung S25 Ultra'
) AS t
WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.slug = t.slug);

-- --- 4. Variants ------------------------------------------------------------

INSERT INTO product_variants (product_id, sku, label, color, attributes, price, stock_quantity, is_active)
SELECT p.id, v.sku, v.label, v.color, JSON_OBJECT('color', v.color), v.price, v.qty, TRUE
FROM (
    SELECT 'housse-samsung-a20'        AS product_slug, 'HSE-SAM-A20-BLEUP'  AS sku, 'Bleu pétrole' AS label, 'Bleu pétrole' AS color, 15000.00 AS price, 1 AS qty
    UNION ALL SELECT 'housse-samsung-a23',       'HSE-SAM-A23-VERT',       'Vert',                   'Vert',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-a50',       'HSE-SAM-A50-BLEU',       'Bleu',                   'Bleu',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-a50',       'HSE-SAM-A50-NOIR',       'Noir',                   'Noir',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-a42',       'HSE-SAM-A42-NOIR',       'Noir',                   'Noir',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-a32-5g',    'HSE-SAM-A32-5G-NOIR',    'Noir',                   'Noir',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-a05s',      'HSE-SAM-A05S-GRENAT',    'Grenat stylé',           'Grenat',         25000.00, 1
    UNION ALL SELECT 'housse-samsung-s10',       'HSE-SAM-S10-BLEUP',      'Bleu pétrole',           'Bleu pétrole',   15000.00, 1
    UNION ALL SELECT 'housse-samsung-s10',       'HSE-SAM-S10-BLEU',       'Bleu',                   'Bleu',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s20-plus',  'HSE-SAM-S20P-NOIR',      'Noir',                   'Noir',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s20-plus',  'HSE-SAM-S20P-GRIS',      'Gris',                   'Gris',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s20-plus',  'HSE-SAM-S20P-BLEU',      'Bleu',                   'Bleu',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s22-ultra', 'HSE-SAM-S22U-GRIS',      'Gris',                   'Gris',           15000.00, 1
    -- Second S22 Ultra line on the sheet: armoured, hence the higher price.
    UNION ALL SELECT 'housse-samsung-s22-ultra', 'HSE-SAM-S22U-NOIRROUGE', 'Noir et rouge (blindée)','Noir et rouge',  25000.00, 1
    UNION ALL SELECT 'housse-samsung-s23',       'HSE-SAM-S23-VERT',       'Vert',                   'Vert',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s23-plus',  'HSE-SAM-S23P-NOIR',      'Noir',                   'Noir',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s23-ultra', 'HSE-SAM-S23U-VERT',      'Vert',                   'Vert',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s24',       'HSE-SAM-S24-BLEUP',      'Bleu pétrole',           'Bleu pétrole',   15000.00, 1
    UNION ALL SELECT 'housse-samsung-s24-plus',  'HSE-SAM-S24P-GRIS',      'Gris',                   'Gris',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s24-plus',  'HSE-SAM-S24P-ROUGE',     'Rouge',                  'Rouge',          15000.00, 1
    UNION ALL SELECT 'housse-samsung-s24-plus',  'HSE-SAM-S24P-BLEU',      'Bleu',                   'Bleu',           15000.00, 1
    UNION ALL SELECT 'housse-samsung-s25-ultra', 'HSE-SAM-S25U-NOIR',      'Noir (blindée)',         'Noir',           25000.00, 1
) AS v
JOIN products p ON p.slug = v.product_slug
WHERE NOT EXISTS (SELECT 1 FROM product_variants pv WHERE pv.sku = v.sku);

-- --- 5. Open the stock ledger -----------------------------------------------

INSERT INTO stock_movements (product_variant_id, department, reason, delta, quantity_after, note)
SELECT pv.id, 'tech', 'initial', pv.stock_quantity, pv.stock_quantity,
       'Opening balance from the Samsung phone-case inventory sheet'
FROM product_variants pv
WHERE pv.sku LIKE 'HSE-SAM-%'
  AND pv.stock_quantity > 0
  AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_variant_id = pv.id);

COMMIT;

-- --- Verification ------------------------------------------------------------
-- SELECT p.name, pv.sku, pv.label, pv.price, pv.stock_quantity
-- FROM product_variants pv JOIN products p ON p.id = pv.product_id
-- WHERE pv.sku LIKE 'HSE-SAM-%' ORDER BY p.name, pv.sku;
--
-- Expected: 15 products, 22 variants, 22 pieces on hand,
-- 19 at 15000.00 and 3 at 25000.00 (A05s, S22 Ultra armoured, S25 Ultra).
