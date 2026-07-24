-- screen_protectors.sql
-- Screen protectors ("Cache Écran"), transcribed from the two handwritten
-- matrix sheets: the general one and the iPhone one.
--
-- THESE SHEETS HAVE A DIFFERENT SHAPE from the case/accessory ones. They are a
-- matrix: rows are handset models, columns are protector grades, and each cell
-- is a quantity. The price sits at the FOOT of each column, not on the row —
-- so the grade is what sets the price:
--
--     Simple 5.000  |  2.1D 10.000  |  Top Glass 15.000  |  Privacy 15.000
--                                              |  Privacy Ceramic 20.000
--
-- That maps onto the schema cleanly: one product per handset, one variant per
-- grade it is stocked in, each with its own price and stock. A cell left blank
-- on the sheet simply has no variant.
--
-- A screen protector has no colour, so this needs a product type the catalog did
-- not have. internal/catalog/templates.go gains a 'screen_protector' template
-- (tech department) whose single variant axis is `protection_type` — using the
-- 'accessory' template instead would have put the grade in a field labelled
-- "Color", which would be wrong on screen and wrong for anyone entering the next
-- batch by hand.
--
-- Because that axis is FILTERABLE, this file also writes product_facets — the
-- denormalised index the service rebuilds on every write. Section 6 reproduces
-- exactly what rebuildFacets() would produce, so faceting works before anyone
-- edits a product.
--
-- FOUR THINGS TO CHECK AGAINST THE PAPER:
--   1. "A50" (general sheet) has a row but NO quantity in any column, so it is
--      not loaded — there is nothing to sell. Add it if that is an omission.
--   2. "A11", "RM9" and "Max 10" are loaded with brand_id NULL: A11 could be a
--      Samsung or an Oppo, RM9 is probably a Redmi 9 but is written as an
--      abbreviation, and "Max 10" could not be placed at all.
--   3. "OPPO A53" appears on TWO rows of the general sheet (Top Glass 6 on the
--      first, Privacy 2 on the second, with Privacy Ceramic 7 and Simple 4 on
--      the "A11" line between them). Read as one Oppo A53 product carrying both
--      grades, the second row being a later addition. The cells around those
--      three rows are the least certain on either sheet.
--   4. The iPhone sheet lists the 11 Pro Max twice — once paired with the XS Max
--      (Privacy 2) and once alone (2.1D 1, Privacy 1). Kept as the sheet has
--      them: a combined "XS Max / 11 Pro Max" fit and a separate "11 Pro Max".
--
-- Loads 38 products, 62 variants, 300 pieces (252 general + 48 iPhone).
-- Re-runnable: every insert is guarded on a unique key.

START TRANSACTION;

-- --- 1. Category ------------------------------------------------------------

INSERT INTO product_categories (name, slug, template_key, description, is_active)
SELECT * FROM (
    SELECT 'Screen protectors' AS name,
           'screen-protectors' AS slug,
           'screen_protector'  AS template_key,
           'Tempered glass and privacy screen protectors, listed per handset model.' AS description,
           TRUE                AS is_active
) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM product_categories WHERE slug = 'screen-protectors');

-- --- 2. Brands (several already exist from the earlier sheets) --------------

INSERT INTO brands (name, slug, department, is_active)
SELECT t.name, t.slug, 'tech', TRUE
FROM (
    SELECT 'Tecno' AS name, 'tecno' AS slug
    UNION ALL SELECT 'Infinix',  'infinix'
    UNION ALL SELECT 'Motorola', 'motorola'
    UNION ALL SELECT 'Vivo',     'vivo'
    UNION ALL SELECT 'Honor',    'honor'
    UNION ALL SELECT 'Samsung',  'samsung'
    UNION ALL SELECT 'Huawei',   'huawei'
    UNION ALL SELECT 'Redmi',    'redmi'
    UNION ALL SELECT 'Oppo',     'oppo'
    UNION ALL SELECT 'Google',   'google'
    UNION ALL SELECT 'Apple',    'apple'
) AS t
WHERE NOT EXISTS (SELECT 1 FROM brands b WHERE b.slug = t.slug);

SET @category_id = (SELECT id FROM product_categories WHERE slug = 'screen-protectors');

-- --- 3. Products: one per handset model -------------------------------------

INSERT INTO products (category_id, brand_id, name, slug, description, is_active)
SELECT @category_id, b.id, t.name, t.slug,
       CONCAT('Protection d''écran pour ', t.model, '.'), TRUE
FROM (
    -- General sheet
    SELECT 'samsung' AS brand_slug, 'Protection écran Samsung A20' AS name, 'protection-ecran-samsung-a20' AS slug, 'Samsung A20' AS model
    UNION ALL SELECT 'samsung',  'Protection écran Samsung A40',          'protection-ecran-samsung-a40',          'Samsung A40'
    UNION ALL SELECT 'huawei',   'Protection écran Huawei P Smart 2019',  'protection-ecran-huawei-p-smart-2019',  'Huawei P Smart 2019'
    UNION ALL SELECT 'huawei',   'Protection écran Huawei P30 Lite',      'protection-ecran-huawei-p30-lite',      'Huawei P30 Lite'
    UNION ALL SELECT 'redmi',    'Protection écran Redmi Note 7',         'protection-ecran-redmi-note-7',         'Redmi Note 7'
    UNION ALL SELECT 'redmi',    'Protection écran Redmi Note 8 Pro',     'protection-ecran-redmi-note-8-pro',     'Redmi Note 8 Pro'
    UNION ALL SELECT 'tecno',    'Protection écran Tecno Spark 5 Air',    'protection-ecran-tecno-spark-5-air',    'Tecno Spark 5 Air'
    UNION ALL SELECT 'infinix',  'Protection écran Infinix Hot 10P',      'protection-ecran-infinix-hot-10p',      'Infinix Hot 10P'
    UNION ALL SELECT 'motorola', 'Protection écran Motorola G9 Plus',     'protection-ecran-motorola-g9-plus',     'Motorola G9 Plus'
    UNION ALL SELECT 'oppo',     'Protection écran Oppo A53',             'protection-ecran-oppo-a53',             'Oppo A53'
    UNION ALL SELECT NULL,       'Protection écran A11',                  'protection-ecran-a11',                  'A11'
    UNION ALL SELECT NULL,       'Protection écran RM9',                  'protection-ecran-rm9',                  'RM9'
    UNION ALL SELECT 'vivo',     'Protection écran Vivo Y85',             'protection-ecran-vivo-y85',             'Vivo Y85'
    UNION ALL SELECT 'honor',    'Protection écran Honor 8X',             'protection-ecran-honor-8x',             'Honor 8X'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 2 XL',    'protection-ecran-google-pixel-2-xl',    'Google Pixel 2 XL'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 3 XL',    'protection-ecran-google-pixel-3-xl',    'Google Pixel 3 XL'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 3',       'protection-ecran-google-pixel-3',       'Google Pixel 3'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 4a',      'protection-ecran-google-pixel-4a',      'Google Pixel 4a'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 4a 4G',   'protection-ecran-google-pixel-4a-4g',   'Google Pixel 4a 4G'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 4 XL',    'protection-ecran-google-pixel-4-xl',    'Google Pixel 4 XL'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 5a',      'protection-ecran-google-pixel-5a',      'Google Pixel 5a'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 6',       'protection-ecran-google-pixel-6',       'Google Pixel 6'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 6a',      'protection-ecran-google-pixel-6a',      'Google Pixel 6a'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 7',       'protection-ecran-google-pixel-7',       'Google Pixel 7'
    UNION ALL SELECT 'google',   'Protection écran Google Pixel 7a',      'protection-ecran-google-pixel-7a',      'Google Pixel 7a'
    UNION ALL SELECT NULL,       'Protection écran Max 10',               'protection-ecran-max-10',               'Max 10'
    -- iPhone sheet
    UNION ALL SELECT 'apple', 'Protection écran iPhone 6 / 7 / 8',            'protection-ecran-iphone-6-7-8',            'iPhone 6 / 7 / 8'
    UNION ALL SELECT 'apple', 'Protection écran iPhone X / XS',               'protection-ecran-iphone-x-xs',             'iPhone X / XS'
    UNION ALL SELECT 'apple', 'Protection écran iPhone XR / 11',              'protection-ecran-iphone-xr-11',            'iPhone XR / 11'
    UNION ALL SELECT 'apple', 'Protection écran iPhone XS Max / 11 Pro Max',  'protection-ecran-iphone-xs-max-11-pro-max','iPhone XS Max / 11 Pro Max'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 11 Pro Max',           'protection-ecran-iphone-11-pro-max',       'iPhone 11 Pro Max'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 12 / 12 Pro',          'protection-ecran-iphone-12-12-pro',        'iPhone 12 / 12 Pro'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 12 Pro Max',           'protection-ecran-iphone-12-pro-max',       'iPhone 12 Pro Max'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 13 / 13 Pro',          'protection-ecran-iphone-13-13-pro',        'iPhone 13 / 13 Pro'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 13 Pro Max',           'protection-ecran-iphone-13-pro-max',       'iPhone 13 Pro Max'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 14 Pro / 15',          'protection-ecran-iphone-14-pro-15',        'iPhone 14 Pro / 15'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 14 Pro Max',           'protection-ecran-iphone-14-pro-max',       'iPhone 14 Pro Max'
    UNION ALL SELECT 'apple', 'Protection écran iPhone 16 Pro Max',           'protection-ecran-iphone-16-pro-max',       'iPhone 16 Pro Max'
) AS t
LEFT JOIN brands b ON b.slug = t.brand_slug
WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.slug = t.slug);

-- --- 4. Variants: one per (model, grade) cell that carries stock -------------
-- color stays NULL — a screen protector has none. The grade lives in `label`
-- for display and in `attributes.protection_type`, which is the template axis
-- and the source of the facets written in section 6.

INSERT INTO product_variants (product_id, sku, label, color, attributes, price, stock_quantity, is_active)
SELECT p.id, v.sku, v.grade, NULL, JSON_OBJECT('protection_type', v.grade), v.price, v.qty, TRUE
FROM (
    -- General sheet: Simple 5.000
    SELECT 'protection-ecran-samsung-a20' AS product_slug, 'PRO-SAM-A20-SIMPLE' AS sku, 'Simple' AS grade, 5000.00 AS price, 1 AS qty
    UNION ALL SELECT 'protection-ecran-samsung-a40',          'PRO-SAM-A40-SIMPLE',       'Simple',           5000.00, 5
    UNION ALL SELECT 'protection-ecran-huawei-p-smart-2019',  'PRO-HUA-PSMART19-SIMPLE',  'Simple',           5000.00, 1
    UNION ALL SELECT 'protection-ecran-redmi-note-7',         'PRO-RDM-N7-SIMPLE',        'Simple',           5000.00, 1
    UNION ALL SELECT 'protection-ecran-redmi-note-8-pro',     'PRO-RDM-N8PRO-SIMPLE',     'Simple',           5000.00, 4
    UNION ALL SELECT 'protection-ecran-a11',                  'PRO-UNK-A11-SIMPLE',       'Simple',           5000.00, 4
    -- General sheet: 2.1D 10.000
    UNION ALL SELECT 'protection-ecran-samsung-a20',          'PRO-SAM-A20-21D',          '2.1D',            10000.00, 9
    UNION ALL SELECT 'protection-ecran-samsung-a40',          'PRO-SAM-A40-21D',          '2.1D',            10000.00, 9
    UNION ALL SELECT 'protection-ecran-redmi-note-7',         'PRO-RDM-N7-21D',           '2.1D',            10000.00, 4
    UNION ALL SELECT 'protection-ecran-redmi-note-8-pro',     'PRO-RDM-N8PRO-21D',        '2.1D',            10000.00, 2
    UNION ALL SELECT 'protection-ecran-tecno-spark-5-air',    'PRO-TEC-SPARK5AIR-21D',    '2.1D',            10000.00, 6
    UNION ALL SELECT 'protection-ecran-infinix-hot-10p',      'PRO-INF-HOT10P-21D',       '2.1D',            10000.00, 4
    UNION ALL SELECT 'protection-ecran-rm9',                  'PRO-UNK-RM9-21D',          '2.1D',            10000.00, 3
    UNION ALL SELECT 'protection-ecran-vivo-y85',             'PRO-VIV-Y85-21D',          '2.1D',            10000.00, 5
    -- General sheet: Top Glass 15.000
    UNION ALL SELECT 'protection-ecran-samsung-a40',          'PRO-SAM-A40-TOPGLASS',     'Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-huawei-p-smart-2019',  'PRO-HUA-PSMART19-TOPGLASS','Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-huawei-p30-lite',      'PRO-HUA-P30LITE-TOPGLASS', 'Top Glass',       15000.00, 9
    UNION ALL SELECT 'protection-ecran-redmi-note-8-pro',     'PRO-RDM-N8PRO-TOPGLASS',   'Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-infinix-hot-10p',      'PRO-INF-HOT10P-TOPGLASS',  'Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-motorola-g9-plus',     'PRO-MOT-G9PLUS-TOPGLASS',  'Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-oppo-a53',             'PRO-OPP-A53-TOPGLASS',     'Top Glass',       15000.00, 6
    UNION ALL SELECT 'protection-ecran-vivo-y85',             'PRO-VIV-Y85-TOPGLASS',     'Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-honor-8x',             'PRO-HON-8X-TOPGLASS',      'Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-google-pixel-2-xl',    'PRO-PIX-2XL-TOPGLASS',     'Top Glass',       15000.00, 3
    UNION ALL SELECT 'protection-ecran-google-pixel-3-xl',    'PRO-PIX-3XL-TOPGLASS',     'Top Glass',       15000.00, 7
    UNION ALL SELECT 'protection-ecran-google-pixel-3',       'PRO-PIX-3-TOPGLASS',       'Top Glass',       15000.00, 5
    UNION ALL SELECT 'protection-ecran-google-pixel-4a',      'PRO-PIX-4A-TOPGLASS',      'Top Glass',       15000.00, 5
    UNION ALL SELECT 'protection-ecran-google-pixel-4a-4g',   'PRO-PIX-4A4G-TOPGLASS',    'Top Glass',       15000.00, 7
    UNION ALL SELECT 'protection-ecran-google-pixel-6',       'PRO-PIX-6-TOPGLASS',       'Top Glass',       15000.00, 2
    UNION ALL SELECT 'protection-ecran-google-pixel-6a',      'PRO-PIX-6A-TOPGLASS',      'Top Glass',       15000.00, 5
    UNION ALL SELECT 'protection-ecran-google-pixel-7',       'PRO-PIX-7-TOPGLASS',       'Top Glass',       15000.00, 4
    UNION ALL SELECT 'protection-ecran-google-pixel-7a',      'PRO-PIX-7A-TOPGLASS',      'Top Glass',       15000.00, 5
    UNION ALL SELECT 'protection-ecran-google-pixel-5a',      'PRO-PIX-5A-TOPGLASS',      'Top Glass',       15000.00, 5
    UNION ALL SELECT 'protection-ecran-max-10',               'PRO-UNK-MAX10-TOPGLASS',   'Top Glass',       15000.00, 10
    -- General sheet: Privacy 15.000
    UNION ALL SELECT 'protection-ecran-huawei-p30-lite',      'PRO-HUA-P30LITE-PRIVACY',  'Privacy',         15000.00, 3
    UNION ALL SELECT 'protection-ecran-redmi-note-8-pro',     'PRO-RDM-N8PRO-PRIVACY',    'Privacy',         15000.00, 3
    UNION ALL SELECT 'protection-ecran-oppo-a53',             'PRO-OPP-A53-PRIVACY',      'Privacy',         15000.00, 2
    UNION ALL SELECT 'protection-ecran-google-pixel-4-xl',    'PRO-PIX-4XL-PRIVACY',      'Privacy',         15000.00, 5
    -- General sheet: Privacy Ceramic 20.000
    UNION ALL SELECT 'protection-ecran-samsung-a40',          'PRO-SAM-A40-PRIVCERAM',    'Privacy Ceramic', 20000.00, 17
    UNION ALL SELECT 'protection-ecran-huawei-p-smart-2019',  'PRO-HUA-PSMART19-PRIVCERAM','Privacy Ceramic',20000.00, 2
    UNION ALL SELECT 'protection-ecran-huawei-p30-lite',      'PRO-HUA-P30LITE-PRIVCERAM','Privacy Ceramic', 20000.00, 8
    UNION ALL SELECT 'protection-ecran-redmi-note-8-pro',     'PRO-RDM-N8PRO-PRIVCERAM',  'Privacy Ceramic', 20000.00, 4
    UNION ALL SELECT 'protection-ecran-a11',                  'PRO-UNK-A11-PRIVCERAM',    'Privacy Ceramic', 20000.00, 7
    -- iPhone sheet: Simple 5.000
    UNION ALL SELECT 'protection-ecran-iphone-xr-11',         'PRO-IPH-XR11-SIMPLE',      'Simple',           5000.00, 3
    UNION ALL SELECT 'protection-ecran-iphone-x-xs',          'PRO-IPH-XXS-SIMPLE',       'Simple',           5000.00, 2
    -- iPhone sheet: 2.1D 10.000
    UNION ALL SELECT 'protection-ecran-iphone-12-pro-max',    'PRO-IPH-12PM-21D',         '2.1D',            10000.00, 3
    UNION ALL SELECT 'protection-ecran-iphone-11-pro-max',    'PRO-IPH-11PM-21D',         '2.1D',            10000.00, 1
    UNION ALL SELECT 'protection-ecran-iphone-13-pro-max',    'PRO-IPH-13PM-21D',         '2.1D',            10000.00, 2
    -- iPhone sheet: Top Glass 15.000
    UNION ALL SELECT 'protection-ecran-iphone-6-7-8',         'PRO-IPH-678-TOPGLASS',     'Top Glass',       15000.00, 10
    UNION ALL SELECT 'protection-ecran-iphone-12-12-pro',     'PRO-IPH-1212PRO-TOPGLASS', 'Top Glass',       15000.00, 1
    UNION ALL SELECT 'protection-ecran-iphone-13-13-pro',     'PRO-IPH-1313PRO-TOPGLASS', 'Top Glass',       15000.00, 1
    UNION ALL SELECT 'protection-ecran-iphone-14-pro-15',     'PRO-IPH-14PRO15-TOPGLASS', 'Top Glass',       15000.00, 1
    UNION ALL SELECT 'protection-ecran-iphone-13-pro-max',    'PRO-IPH-13PM-TOPGLASS',    'Top Glass',       15000.00, 2
    UNION ALL SELECT 'protection-ecran-iphone-x-xs',          'PRO-IPH-XXS-TOPGLASS',     'Top Glass',       15000.00, 1
    UNION ALL SELECT 'protection-ecran-iphone-16-pro-max',    'PRO-IPH-16PM-TOPGLASS',    'Top Glass',       15000.00, 2
    -- iPhone sheet: Privacy 15.000
    UNION ALL SELECT 'protection-ecran-iphone-xs-max-11-pro-max','PRO-IPH-XSMAX11PM-PRIVACY','Privacy',      15000.00, 2
    UNION ALL SELECT 'protection-ecran-iphone-xr-11',         'PRO-IPH-XR11-PRIVACY',     'Privacy',         15000.00, 3
    UNION ALL SELECT 'protection-ecran-iphone-12-12-pro',     'PRO-IPH-1212PRO-PRIVACY',  'Privacy',         15000.00, 1
    UNION ALL SELECT 'protection-ecran-iphone-13-13-pro',     'PRO-IPH-1313PRO-PRIVACY',  'Privacy',         15000.00, 9
    UNION ALL SELECT 'protection-ecran-iphone-12-pro-max',    'PRO-IPH-12PM-PRIVACY',     'Privacy',         15000.00, 2
    UNION ALL SELECT 'protection-ecran-iphone-11-pro-max',    'PRO-IPH-11PM-PRIVACY',     'Privacy',         15000.00, 1
    UNION ALL SELECT 'protection-ecran-iphone-14-pro-max',    'PRO-IPH-14PM-PRIVACY',     'Privacy',         15000.00, 1
) AS v
JOIN products p ON p.slug = v.product_slug
WHERE NOT EXISTS (SELECT 1 FROM product_variants pv WHERE pv.sku = v.sku);

-- --- 5. Open the stock ledger -----------------------------------------------

INSERT INTO stock_movements (product_variant_id, department, reason, delta, quantity_after, note)
SELECT pv.id, 'tech', 'initial', pv.stock_quantity, pv.stock_quantity,
       'Opening balance from the screen-protector inventory sheets'
FROM product_variants pv
WHERE pv.sku LIKE 'PRO-%'
  AND pv.stock_quantity > 0
  AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_variant_id = pv.id);

-- --- 6. Facets --------------------------------------------------------------
-- `protection_type` is a filterable axis, so the storefront's facet index needs
-- rows for it. This reproduces rebuildFacets(): the distinct axis values of a
-- product's ACTIVE variants. (The template defines no product-level fields, so
-- there is nothing else to contribute.) Any later edit through the admin API
-- rewrites these rows from the same source.

INSERT INTO product_facets (product_id, facet_key, facet_value)
SELECT DISTINCT pv.product_id, 'protection_type', pv.label
FROM product_variants pv
WHERE pv.sku LIKE 'PRO-%'
  AND pv.is_active = TRUE
  AND NOT EXISTS (
      SELECT 1 FROM product_facets f
      WHERE f.product_id = pv.product_id
        AND f.facet_key = 'protection_type'
        AND f.facet_value = pv.label
  );

COMMIT;

-- --- Verification ------------------------------------------------------------
-- SELECT b.name AS brand, p.name, pv.sku, pv.label AS grade, pv.price, pv.stock_quantity
-- FROM product_variants pv
--   JOIN products p ON p.id = pv.product_id
--   LEFT JOIN brands b ON b.id = p.brand_id
-- WHERE pv.sku LIKE 'PRO-%' ORDER BY p.name, pv.price;
--
-- Per-grade totals, which should match the columns of the two sheets:
-- SELECT pv.label AS grade, pv.price, COUNT(*) AS skus, SUM(pv.stock_quantity) AS pieces
-- FROM product_variants pv WHERE pv.sku LIKE 'PRO-%' GROUP BY pv.label, pv.price;
--   Simple           5000.00   8 SKUs   21 pieces
--   2.1D            10000.00  11 SKUs   48 pieces
--   Top Glass       15000.00  27 SKUs  161 pieces
--   Privacy         15000.00  11 SKUs   32 pieces
--   Privacy Ceramic 20000.00   5 SKUs   38 pieces
--                             62 SKUs  300 pieces
