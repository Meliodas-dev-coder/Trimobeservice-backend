-- google_pixel_and_misc_phone_cases.sql
-- Phone-case stock, transcribed from the handwritten sheet holding two blocks:
-- "Google Pixel (Housse)" and "Housse Divers".
--
-- Same model as seeds/redmi_phone_cases.sql: the shared 'Phone cases' category
-- (template_key 'accessory', which is what puts it in the `tech` department),
-- one product per handset, one variant/SKU per colour.
--
-- Notes on this sheet specifically:
--   * "15 K" / "2 5 K" are Ariary shorthand -> 15000.00 / 25000.00, matching the
--     "15.000 Ar" / "25.000 Ar" written in full on the Samsung sheet.
--   * Where the sheet puts a number in brackets after a colour — Pixel 6 Pro
--     "Transparent (2)", "Noir Blindé (1)" — that is the per-colour count, and
--     it is used as written.
--   * The "Divers" block spans several makers, so the brand is set per product.
--     Every row there has as many colours as its count, so nothing is ambiguous.
--
-- TWO MODELS COULD NOT BE IDENTIFIED FROM THE HANDWRITING:
--   "S0 2" and "F 52" are loaded with their sheet wording and brand_id NULL
--   (F 52 may be a Samsung Galaxy F52). Fix the name and attach a brand with:
--     UPDATE products SET name = ?, slug = ?, brand_id = (SELECT id FROM brands
--       WHERE slug = ?) WHERE slug IN ('housse-s0-2','housse-f-52');
--
-- ALSO WORTH A SECOND LOOK: the three rows priced at 25000.00 (Pixel 6 Pro,
--   Pixel 7 Pro, Itel Pop 3). The sheet gives ONE price per row, so the two
--   plain "Transparent" Pixel 6 Pro cases inherit the same 25000.00 as the
--   armoured one. Transcribed as written — correct the two SKUs if the
--   transparent ones are really 15000.00.
--
-- Loads 20 products, 31 variants, 34 pieces (13 Pixel + 21 Divers) — the sheet's
-- own totals. Re-runnable: every insert is guarded on a unique key.

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

-- --- 2. Brands --------------------------------------------------------------

INSERT INTO brands (name, slug, department, is_active)
SELECT t.name, t.slug, 'tech', TRUE
FROM (
    SELECT 'Google' AS name, 'google' AS slug
    UNION ALL SELECT 'Huawei', 'huawei'
    UNION ALL SELECT 'Oppo',   'oppo'
    UNION ALL SELECT 'Itel',   'itel'
    UNION ALL SELECT 'Xiaomi', 'xiaomi'
) AS t
WHERE NOT EXISTS (SELECT 1 FROM brands b WHERE b.slug = t.slug);

SET @category_id = (SELECT id FROM product_categories WHERE slug = 'phone-cases');

-- --- 3. Products ------------------------------------------------------------
-- LEFT JOIN on the brand so the two unidentified models still load, with a NULL
-- brand rather than a wrong one.

INSERT INTO products (category_id, brand_id, name, slug, description, is_active)
SELECT @category_id, b.id, t.name, t.slug,
       CONCAT('Housse de protection pour ', t.model, '.'), TRUE
FROM (
    -- Google Pixel block
    SELECT 'google' AS brand_slug, 'Housse Google Pixel 4 XL'  AS name, 'housse-google-pixel-4-xl'  AS slug, 'Google Pixel 4 XL'  AS model
    UNION ALL SELECT 'google', 'Housse Google Pixel 4a 4G',  'housse-google-pixel-4a-4g',  'Google Pixel 4a 4G'
    UNION ALL SELECT 'google', 'Housse Google Pixel 6a',     'housse-google-pixel-6a',     'Google Pixel 6a'
    UNION ALL SELECT 'google', 'Housse Google Pixel 6 Pro',  'housse-google-pixel-6-pro',  'Google Pixel 6 Pro'
    UNION ALL SELECT 'google', 'Housse Google Pixel 7',      'housse-google-pixel-7',      'Google Pixel 7'
    UNION ALL SELECT 'google', 'Housse Google Pixel 7 Pro',  'housse-google-pixel-7-pro',  'Google Pixel 7 Pro'
    UNION ALL SELECT 'google', 'Housse Google Pixel 8 Pro',  'housse-google-pixel-8-pro',  'Google Pixel 8 Pro'
    -- "Housse Divers" block
    UNION ALL SELECT 'huawei', 'Housse Huawei Nova 9 Pro',   'housse-huawei-nova-9-pro',   'Huawei Nova 9 Pro'
    UNION ALL SELECT 'huawei', 'Housse Huawei Nova 9',       'housse-huawei-nova-9',       'Huawei Nova 9'
    UNION ALL SELECT 'huawei', 'Housse Huawei Nova 7',       'housse-huawei-nova-7',       'Huawei Nova 7'
    UNION ALL SELECT 'huawei', 'Housse Huawei Nova 7 Pro',   'housse-huawei-nova-7-pro',   'Huawei Nova 7 Pro'
    UNION ALL SELECT 'huawei', 'Housse Huawei Nova 11',      'housse-huawei-nova-11',      'Huawei Nova 11'
    UNION ALL SELECT 'huawei', 'Housse Huawei Mate 10 Pro',  'housse-huawei-mate-10-pro',  'Huawei Mate 10 Pro'
    UNION ALL SELECT 'huawei', 'Housse Huawei Mate 20 Pro',  'housse-huawei-mate-20-pro',  'Huawei Mate 20 Pro'
    UNION ALL SELECT 'huawei', 'Housse Huawei P50 Pro',      'housse-huawei-p50-pro',      'Huawei P50 Pro'
    UNION ALL SELECT NULL,     'Housse S0 2',                'housse-s0-2',                'S0 2'
    UNION ALL SELECT NULL,     'Housse F 52',                'housse-f-52',                'F 52'
    UNION ALL SELECT 'oppo',   'Housse Oppo A53',            'housse-oppo-a53',            'Oppo A53'
    UNION ALL SELECT 'itel',   'Housse Itel Pop 3',          'housse-itel-pop-3',          'Itel Pop 3'
    UNION ALL SELECT 'xiaomi', 'Housse Xiaomi 10 Pro',       'housse-xiaomi-10-pro',       'Xiaomi 10 Pro'
) AS t
LEFT JOIN brands b ON b.slug = t.brand_slug
WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.slug = t.slug);

-- --- 4. Variants ------------------------------------------------------------
-- `color` holds the plain colour so the storefront can group on it; `label`
-- keeps the sheet's own wording (the armoured finish, the styled ones).

INSERT INTO product_variants (product_id, sku, label, color, attributes, price, stock_quantity, is_active)
SELECT p.id, v.sku, v.label, v.color, JSON_OBJECT('color', v.color), v.price, v.qty, TRUE
FROM (
    -- Google Pixel: 13 pieces
    SELECT 'housse-google-pixel-4-xl'   AS product_slug, 'HSE-PIX-4XL-NOIR'    AS sku, 'Noir' AS label, 'Noir' AS color, 15000.00 AS price, 1 AS qty
    UNION ALL SELECT 'housse-google-pixel-4a-4g',  'HSE-PIX-4A-4G-NOIR',    'Noir',            'Noir',            15000.00, 2
    UNION ALL SELECT 'housse-google-pixel-6a',     'HSE-PIX-6A-VERT',       'Vert',            'Vert',            15000.00, 1
    -- Sheet: "Transparent (2)" + "Noir Blindé (1)" at one row price of 25 K.
    UNION ALL SELECT 'housse-google-pixel-6-pro',  'HSE-PIX-6PRO-TRANS',    'Transparent',     'Transparent',     25000.00, 2
    UNION ALL SELECT 'housse-google-pixel-6-pro',  'HSE-PIX-6PRO-NOIRBL',   'Noir (blindée)',  'Noir',            25000.00, 1
    UNION ALL SELECT 'housse-google-pixel-7',      'HSE-PIX-7-GRIS',        'Gris',            'Gris',            15000.00, 1
    UNION ALL SELECT 'housse-google-pixel-7',      'HSE-PIX-7-BLEUP',       'Bleu pétrole',    'Bleu pétrole',    15000.00, 1
    UNION ALL SELECT 'housse-google-pixel-7',      'HSE-PIX-7-BLEU',        'Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-google-pixel-7-pro',  'HSE-PIX-7PRO-TRANS',    'Transparent',     'Transparent',     25000.00, 2
    UNION ALL SELECT 'housse-google-pixel-8-pro',  'HSE-PIX-8PRO-NOIR',     'Noir',            'Noir',            15000.00, 1
    -- Housse Divers: 21 pieces
    UNION ALL SELECT 'housse-huawei-nova-9-pro',   'HSE-HUA-NOVA9PRO-BLEU', 'Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-nova-9-pro',   'HSE-HUA-NOVA9PRO-ROUGE','Rouge',           'Rouge',           15000.00, 1
    UNION ALL SELECT 'housse-huawei-nova-9',       'HSE-HUA-NOVA9-NOIR',    'Noir',            'Noir',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-nova-7',       'HSE-HUA-NOVA7-BLEUP',   'Bleu pétrole',    'Bleu pétrole',    15000.00, 1
    UNION ALL SELECT 'housse-huawei-nova-7-pro',   'HSE-HUA-NOVA7PRO-ROUGE','Rouge',           'Rouge',           15000.00, 1
    UNION ALL SELECT 'housse-huawei-nova-11',      'HSE-HUA-NOVA11-ROUGE',  'Rouge',           'Rouge',           15000.00, 1
    UNION ALL SELECT 'housse-huawei-nova-11',      'HSE-HUA-NOVA11-BLEU',   'Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-nova-11',      'HSE-HUA-NOVA11-GRIS',   'Gris',            'Gris',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-mate-10-pro',  'HSE-HUA-MATE10PRO-BLEU','Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-mate-20-pro',  'HSE-HUA-MATE20PRO-NOIR','Noir',            'Noir',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-mate-20-pro',  'HSE-HUA-MATE20PRO-BLEU','Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-p50-pro',      'HSE-HUA-P50PRO-NOIR',   'Noir',            'Noir',            15000.00, 1
    UNION ALL SELECT 'housse-huawei-p50-pro',      'HSE-HUA-P50PRO-VERT',   'Vert',            'Vert',            15000.00, 1
    UNION ALL SELECT 'housse-s0-2',                'HSE-UNK-S02-VERT',      'Vert',            'Vert',            15000.00, 1
    UNION ALL SELECT 'housse-s0-2',                'HSE-UNK-S02-BLEU',      'Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-f-52',                'HSE-UNK-F52-VIOLET',    'Violet',          'Violet',          15000.00, 1
    UNION ALL SELECT 'housse-oppo-a53',            'HSE-OPP-A53-BLEU',      'Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-itel-pop-3',          'HSE-ITL-POP3-NOIR',     'Noir (stylé)',    'Noir',            25000.00, 1
    UNION ALL SELECT 'housse-itel-pop-3',          'HSE-ITL-POP3-BLEU',     'Bleu (stylé)',    'Bleu',            25000.00, 1
    UNION ALL SELECT 'housse-xiaomi-10-pro',       'HSE-XIA-10PRO-BLEU',    'Bleu',            'Bleu',            15000.00, 1
    UNION ALL SELECT 'housse-xiaomi-10-pro',       'HSE-XIA-10PRO-VERT',    'Vert',            'Vert',            15000.00, 1
) AS v
JOIN products p ON p.slug = v.product_slug
WHERE NOT EXISTS (SELECT 1 FROM product_variants pv WHERE pv.sku = v.sku);

-- --- 5. Open the stock ledger -----------------------------------------------

INSERT INTO stock_movements (product_variant_id, department, reason, delta, quantity_after, note)
SELECT pv.id, 'tech', 'initial', pv.stock_quantity, pv.stock_quantity,
       'Opening balance from the Google Pixel / Divers phone-case inventory sheet'
FROM product_variants pv
WHERE (pv.sku LIKE 'HSE-PIX-%' OR pv.sku LIKE 'HSE-HUA-%' OR pv.sku LIKE 'HSE-OPP-%'
       OR pv.sku LIKE 'HSE-ITL-%' OR pv.sku LIKE 'HSE-XIA-%' OR pv.sku LIKE 'HSE-UNK-%')
  AND pv.stock_quantity > 0
  AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_variant_id = pv.id);

COMMIT;

-- --- Verification ------------------------------------------------------------
-- SELECT b.name AS brand, p.name, pv.sku, pv.label, pv.price, pv.stock_quantity
-- FROM product_variants pv
--   JOIN products p ON p.id = pv.product_id
--   LEFT JOIN brands b ON b.id = p.brand_id
-- WHERE pv.sku REGEXP '^HSE-(PIX|HUA|OPP|ITL|XIA|UNK)-'
-- ORDER BY brand, p.name, pv.sku;
--
-- Expected: 20 products, 31 variants, 34 pieces on hand.
