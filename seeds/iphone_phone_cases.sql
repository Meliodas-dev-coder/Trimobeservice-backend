-- iphone_phone_cases.sql
-- Phone-case stock, transcribed from the handwritten "Housse Iphone" sheet.
--
-- Same model as seeds/redmi_phone_cases.sql: the shared 'Phone cases' category
-- (template_key 'accessory', which is what puts it in the `tech` department),
-- one product per handset, one variant/SKU per colour.
--
-- On this sheet the "colour" is often a finish rather than a colour (fleuri,
-- couleur mélangée, stylé, and the designer look-alikes). `label` keeps the
-- sheet's wording; `color` holds the nearest plain colour so the storefront can
-- still group on it, falling back to the finish name where there is no colour.
--
-- ONE LINE OF THE SHEET IS AMBIGUOUS AND IS NOT FULLY LOADED HERE:
--   iPhone 15 Pro lists three finishes (Transparent, Couleur mélangée, Stylé)
--   but a count of 4. This script loads 1 of each (3 pieces). Raise the right
--   SKU's stock_quantity to 2 once you know which one it is.
--   Sheet total: 22 pieces. Loaded here: 21.
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
    SELECT 'Apple' AS name, 'apple' AS slug, 'tech' AS department, TRUE AS is_active
) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM brands WHERE slug = 'apple');

SET @category_id = (SELECT id FROM product_categories WHERE slug = 'phone-cases');
SET @brand_id    = (SELECT id FROM brands WHERE slug = 'apple');

-- --- 3. Products ------------------------------------------------------------

INSERT INTO products (category_id, brand_id, name, slug, description, is_active)
SELECT @category_id, @brand_id, t.name, t.slug,
       CONCAT('Housse de protection pour ', t.model, '.'), TRUE
FROM (
    SELECT 'Housse iPhone XR'         AS name, 'housse-iphone-xr'         AS slug, 'iPhone XR'         AS model
    UNION ALL SELECT 'Housse iPhone XS Max',     'housse-iphone-xs-max',     'iPhone XS Max'
    UNION ALL SELECT 'Housse iPhone 11 Pro',     'housse-iphone-11-pro',     'iPhone 11 Pro'
    UNION ALL SELECT 'Housse iPhone 13 Pro',     'housse-iphone-13-pro',     'iPhone 13 Pro'
    UNION ALL SELECT 'Housse iPhone 14',         'housse-iphone-14',         'iPhone 14'
    UNION ALL SELECT 'Housse iPhone 14 Pro Max', 'housse-iphone-14-pro-max', 'iPhone 14 Pro Max'
    UNION ALL SELECT 'Housse iPhone 15',         'housse-iphone-15',         'iPhone 15'
    UNION ALL SELECT 'Housse iPhone 15 Pro',     'housse-iphone-15-pro',     'iPhone 15 Pro'
    UNION ALL SELECT 'Housse iPhone 15 Pro Max', 'housse-iphone-15-pro-max', 'iPhone 15 Pro Max'
) AS t
WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.slug = t.slug);

-- --- 4. Variants ------------------------------------------------------------

INSERT INTO product_variants (product_id, sku, label, color, attributes, price, stock_quantity, is_active)
SELECT p.id, v.sku, v.label, v.color, JSON_OBJECT('color', v.color), v.price, v.qty, TRUE
FROM (
    SELECT 'housse-iphone-xr'         AS product_slug, 'HSE-IPH-XR-NOIR'        AS sku, 'Noir' AS label, 'Noir' AS color, 15000.00 AS price, 1 AS qty
    UNION ALL SELECT 'housse-iphone-xs-max',     'HSE-IPH-XSMAX-ARGENT',     'Argenté (transparent)',   'Argenté',           15000.00, 2
    UNION ALL SELECT 'housse-iphone-11-pro',     'HSE-IPH-11PRO-TRANSNOIR',  'Transparent noir',        'Transparent noir',  15000.00, 2
    UNION ALL SELECT 'housse-iphone-13-pro',     'HSE-IPH-13PRO-TRANSFLEUR', 'Transparent (fleuri)',    'Transparent',       15000.00, 2
    UNION ALL SELECT 'housse-iphone-14',         'HSE-IPH-14-TRANSNOIR',     'Transparent noir',        'Transparent noir',  15000.00, 1
    UNION ALL SELECT 'housse-iphone-14-pro-max', 'HSE-IPH-14PROMAX-GUCCI',   'Gucci (rose et bleu)',    'Rose et bleu',      15000.00, 1
    UNION ALL SELECT 'housse-iphone-14-pro-max', 'HSE-IPH-14PROMAX-TRANSROSE','Transparent et rose',    'Transparent rose',  15000.00, 1
    UNION ALL SELECT 'housse-iphone-15',         'HSE-IPH-15-TRANSBLEU',     'Transparent bleu',        'Transparent bleu',  15000.00, 1
    -- Sheet says 4 pieces across these three finishes; see the header note.
    UNION ALL SELECT 'housse-iphone-15-pro',     'HSE-IPH-15PRO-TRANS',      'Transparent',             'Transparent',       15000.00, 1
    UNION ALL SELECT 'housse-iphone-15-pro',     'HSE-IPH-15PRO-MELANGE',    'Couleur mélangée',        'Couleur mélangée',  15000.00, 1
    UNION ALL SELECT 'housse-iphone-15-pro',     'HSE-IPH-15PRO-STYLE',      'Stylé',                   'Stylé',             15000.00, 1
    UNION ALL SELECT 'housse-iphone-15-pro-max', 'HSE-IPH-15PROMAX-CHANEL',  'Chanel',                  'Chanel',            15000.00, 7
) AS v
JOIN products p ON p.slug = v.product_slug
WHERE NOT EXISTS (SELECT 1 FROM product_variants pv WHERE pv.sku = v.sku);

-- --- 5. Open the stock ledger -----------------------------------------------

INSERT INTO stock_movements (product_variant_id, department, reason, delta, quantity_after, note)
SELECT pv.id, 'tech', 'initial', pv.stock_quantity, pv.stock_quantity,
       'Opening balance from the iPhone phone-case inventory sheet'
FROM product_variants pv
WHERE pv.sku LIKE 'HSE-IPH-%'
  AND pv.stock_quantity > 0
  AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_variant_id = pv.id);

COMMIT;

-- --- Verification ------------------------------------------------------------
-- SELECT p.name, pv.sku, pv.label, pv.price, pv.stock_quantity
-- FROM product_variants pv JOIN products p ON p.id = pv.product_id
-- WHERE pv.sku LIKE 'HSE-IPH-%' ORDER BY p.name, pv.sku;
--
-- Expected: 9 products, 12 variants, 21 pieces on hand (22 once the 15 Pro line
-- is resolved), all at 15000.00.
