-- redmi_phone_cases.sql
-- Redmi phone-case stock, transcribed from the handwritten inventory sheet
-- (Désignation "Housse — Redmi": Modèle / Couleur / Prix Unit / Nombre).
--
-- Modelling choices, so the sheet maps onto Category -> Product -> Variant:
--
--   * Category "Phone cases" with template_key = 'accessory'. The department is
--     NOT a column on the category — it is derived from the template
--     (internal/catalog/templates.go), and 'accessory' belongs to `tech`. That
--     single value is what puts this catalog under the Tech back office.
--   * Brand "Redmi" (department 'tech'). On the sheet Redmi is the phone the
--     case fits, not the case maker; the shop reads it as the brand either way,
--     and it is what makes the catalog filterable by handset family.
--   * One product per handset model, one variant (sellable SKU) per colour —
--     each with its own price and stock, which is exactly what the "Couleur /
--     Nombre" pair on the sheet describes. Colour is the only variant axis the
--     'accessory' template defines.
--   * Prices are Ariary as written ("15.000 Ar" -> 15000.00). Only the Redmi 9T
--     "blindée" (rugged) case is 25000.00.
--   * products.attributes stays NULL: the 'accessory' template's product fields
--     are connector/wattage/length, none of which apply to a case. No
--     product_facets rows are needed either — the colour axis is not marked
--     filterable, so the service would write none.
--
-- ONE LINE OF THE SHEET IS AMBIGUOUS AND IS NOT FULLY LOADED HERE:
--   Redmi Note 8 Pro lists three colours (Marron, Bleu, Noir personnalisé) but
--   a count of 4. This script loads 1 of each (3 pieces). Raise the right
--   colour's stock_quantity to 2 once you know which one it is; the ledger
--   insert at the bottom will then record the correct opening balance.
--   Sheet total: 25 pieces. Loaded here: 24.
--
-- Re-runnable: every insert is guarded by NOT EXISTS on a unique key, so
-- running this twice inserts nothing the second time.

START TRANSACTION;

-- --- 1. Category (this is what assigns the department) -----------------------

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
    SELECT 'Redmi' AS name, 'redmi' AS slug, 'tech' AS department, TRUE AS is_active
) AS tmp
WHERE NOT EXISTS (SELECT 1 FROM brands WHERE slug = 'redmi');

SET @category_id = (SELECT id FROM product_categories WHERE slug = 'phone-cases');
SET @brand_id    = (SELECT id FROM brands WHERE slug = 'redmi');

-- --- 3. Products: one per handset model -------------------------------------

INSERT INTO products (category_id, brand_id, name, slug, description, is_active)
SELECT @category_id, @brand_id, t.name, t.slug,
       CONCAT('Housse de protection pour ', t.model, '.'), TRUE
FROM (
    SELECT 'Housse Redmi Note 7 Pro'     AS name, 'housse-redmi-note-7-pro'     AS slug, 'Redmi Note 7 Pro'     AS model
    UNION ALL SELECT 'Housse Redmi Note 6 Pro',    'housse-redmi-note-6-pro',    'Redmi Note 6 Pro'
    UNION ALL SELECT 'Housse Redmi Note 8',        'housse-redmi-note-8',        'Redmi Note 8'
    UNION ALL SELECT 'Housse Redmi Note 8 Pro',    'housse-redmi-note-8-pro',    'Redmi Note 8 Pro'
    UNION ALL SELECT 'Housse Redmi 9T (blindée)',  'housse-redmi-9t-blindee',    'Redmi 9T'
    UNION ALL SELECT 'Housse Redmi Note 9 3G',     'housse-redmi-note-9-3g',     'Redmi Note 9 3G'
    UNION ALL SELECT 'Housse Redmi 10A',           'housse-redmi-10a',           'Redmi 10A'
    UNION ALL SELECT 'Housse Redmi Note 10 4G',    'housse-redmi-note-10-4g',    'Redmi Note 10 4G'
    UNION ALL SELECT 'Housse Redmi Note 10 Pro 5G','housse-redmi-note-10-pro-5g','Redmi Note 10 Pro 5G'
    UNION ALL SELECT 'Housse Redmi Note 11 Pro',   'housse-redmi-note-11-pro',   'Redmi Note 11 Pro'
    UNION ALL SELECT 'Housse Redmi Note 11 Pro 5G','housse-redmi-note-11-pro-5g','Redmi Note 11 Pro 5G'
    UNION ALL SELECT 'Housse Redmi Note 11R',      'housse-redmi-note-11r',      'Redmi Note 11R'
    UNION ALL SELECT 'Housse Redmi Note 12',       'housse-redmi-note-12',       'Redmi Note 12'
    UNION ALL SELECT 'Housse Redmi Note 12 4G',    'housse-redmi-note-12-4g',    'Redmi Note 12 4G'
    UNION ALL SELECT 'Housse Redmi Note 12 5G',    'housse-redmi-note-12-5g',    'Redmi Note 12 5G'
    UNION ALL SELECT 'Housse Redmi 13C 4G',        'housse-redmi-13c-4g',        'Redmi 13C 4G'
    UNION ALL SELECT 'Housse Redmi Note 13 Pro',   'housse-redmi-note-13-pro',   'Redmi Note 13 Pro'
    UNION ALL SELECT 'Housse Redmi K20',           'housse-redmi-k20',           'Redmi K20'
) AS t
WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.slug = t.slug);

-- --- 4. Variants: one sellable SKU per colour -------------------------------
-- `label` keeps the sheet's own wording (the bow, the custom finish); `color`
-- stays the plain colour so the storefront can group on it. The colour is also
-- written to `attributes`, which is where the template axis lives and where the
-- admin UI reads a variant's title from.

INSERT INTO product_variants (product_id, sku, label, color, attributes, price, stock_quantity, is_active)
SELECT p.id, v.sku, v.label, v.color, JSON_OBJECT('color', v.color), v.price, v.qty, TRUE
FROM (
    SELECT 'housse-redmi-note-7-pro'      AS product_slug, 'HSE-N7PRO-NOIR'       AS sku, 'Noir (nœud orange)' AS label, 'Noir'         AS color, 15000.00 AS price, 1 AS qty
    UNION ALL SELECT 'housse-redmi-note-6-pro',     'HSE-N6PRO-ROSE',      'Rose',                'Rose',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-8',         'HSE-N8-NOIR',         'Noir',                'Noir',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-8',         'HSE-N8-BLEUP',        'Bleu pétrole',        'Bleu pétrole', 15000.00, 1
    -- Sheet says 4 pieces across these three colours; see the header note.
    UNION ALL SELECT 'housse-redmi-note-8-pro',     'HSE-N8PRO-MARRON',    'Marron',              'Marron',       15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-8-pro',     'HSE-N8PRO-BLEU',      'Bleu',                'Bleu',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-8-pro',     'HSE-N8PRO-NOIR',      'Noir (personnalisé)', 'Noir',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-9t-blindee',     'HSE-9T-NOIRGRIS',     'Noir et gris',        'Noir et gris', 25000.00, 1
    UNION ALL SELECT 'housse-redmi-note-9-3g',      'HSE-N9-3G-NOIR',      'Noir',                'Noir',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-10a',            'HSE-10A-NOIR',        'Noir',                'Noir',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-10a',            'HSE-10A-VERT',        'Vert',                'Vert',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-10-4g',     'HSE-N10-4G-NOIR',     'Noir',                'Noir',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-10-4g',     'HSE-N10-4G-VERT',     'Vert',                'Vert',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-10-pro-5g', 'HSE-N10PRO-5G-MARRON','Marron',              'Marron',       15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-11-pro',    'HSE-N11PRO-BLEU',     'Bleu',                'Bleu',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-11-pro-5g', 'HSE-N11PRO-5G-MARRON','Marron stylé',        'Marron',       15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-11r',       'HSE-N11R-NOIR',       'Noir',                'Noir',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-12',        'HSE-N12-GRENAT',      'Grenat',              'Grenat',       15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-12-4g',     'HSE-N12-4G-BLEU',     'Bleu',                'Bleu',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-12-5g',     'HSE-N12-5G-VIOLET',   'Violet',              'Violet',       15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-12-5g',     'HSE-N12-5G-BLEU',     'Bleu (avec nœud)',    'Bleu',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-13c-4g',         'HSE-13C-4G-VERT',     'Vert',                'Vert',         15000.00, 1
    UNION ALL SELECT 'housse-redmi-note-13-pro',    'HSE-N13PRO-BLEUP',    'Bleu pétrole',        'Bleu pétrole', 15000.00, 1
    UNION ALL SELECT 'housse-redmi-k20',            'HSE-K20-VERT',        'Vert',                'Vert',         15000.00, 1
) AS v
JOIN products p ON p.slug = v.product_slug
WHERE NOT EXISTS (SELECT 1 FROM product_variants pv WHERE pv.sku = v.sku);

-- --- 5. Open the stock ledger -----------------------------------------------
-- stock_quantity above is what is on hand; stock_movements is what explains it.
-- Mirrors the opening-balance insert in migration 000035 so the on-hand level
-- reconciles against the sum of movements from the very first read.

INSERT INTO stock_movements (product_variant_id, department, reason, delta, quantity_after, note)
SELECT pv.id, 'tech', 'initial', pv.stock_quantity, pv.stock_quantity,
       'Opening balance from the Redmi phone-case inventory sheet'
FROM product_variants pv
WHERE pv.sku LIKE 'HSE-%'
  AND pv.stock_quantity > 0
  AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_variant_id = pv.id);

COMMIT;

-- --- Verification ------------------------------------------------------------
-- SELECT p.name, pv.sku, pv.label, pv.price, pv.stock_quantity
-- FROM product_variants pv JOIN products p ON p.id = pv.product_id
-- WHERE p.category_id = (SELECT id FROM product_categories WHERE slug = 'phone-cases')
-- ORDER BY p.name, pv.sku;
--
-- Expected: 18 products, 24 variants, 24 pieces on hand (25 once the Note 8 Pro
-- line is resolved), 23 at 15000.00 and 1 at 25000.00.
