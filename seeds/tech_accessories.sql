-- tech_accessories.sql
-- Accessories, audio, gaming and one handset — transcribed from the two
-- handwritten sheets that follow the phone-case pages (chargers, casques,
-- manettes, cordons, bagues, souris, smart watch, Itel 5606…).
--
-- These are NOT phone cases, so they do not reuse the 'phone-cases' category.
-- A category's template_key decides its department, so the categories below pick
-- the template that actually describes the goods — all three used here
-- ('accessory', 'audio', 'phone') belong to `tech`:
--
--   Audio               -> 'audio'      casques, écouteurs, AirPods
--   Chargers & cables   -> 'accessory'  chargeurs, câbles, adaptateurs, OTG
--   Phone accessories   -> 'accessory'  cordons, bracelets, bagues, batterie, cache
--   Gaming              -> 'accessory'  manettes
--   Computing           -> 'accessory'  souris
--   Wearables           -> 'accessory'  smart watch
--   Phones              -> 'phone'      Itel 5606 (a handset, not an accessory)
--
-- `attributes` stays NULL throughout: no template field is required, and the
-- sheets carry no specs (wattage, driver size, RAM…). Inventing them would put
-- unsourced data in the catalog. No product_facets rows are needed either — the
-- colour axis is not filterable on any of these templates.
--
-- BRANDS ARE ONLY SET WHERE THE SHEET NAMES A MAKER (Xiaomi, JBL, Bose,
-- Marshall, Pioneer, Itel, Remax). "Air pod pro" at 60.000 Ar, the Lightning
-- earphones, the Jack adapter and the "Manette PS4/Xbox" are left with a NULL
-- brand rather than asserting Apple / Sony / Microsoft made them.
--
-- FOUR LINES NEEDED A JUDGEMENT CALL — all flagged again at their row:
--   1. Cordon pour téléphone: 3 colours but 5 pieces. Loaded 1 each (3); raise
--      the right two SKUs. THE ONLY PIECES NOT LOADED — 101 of the sheets' 103.
--   2. JBL Super Bass: "Gris, Grenat" but a count of 1. Read as ONE two-tone
--      unit (like "Noir et gris" on the Redmi sheet), not two units.
--   3. Casque Marshall: the sheet lists "Noir (2)", "Blanc (1)", "Noir (2)" and
--      a struck-through line = 5. Merged to Noir ×4 + Blanc ×1; colour is the
--      variant axis, so the same colour cannot be two SKUs on one product.
--   4. "Dixtup chargeur" and "Allume" are as written / expanded to
--      "Allume-cigare". Rename if either reading is wrong.
--
-- Loads 29 products, 37 variants, 101 pieces.
-- Re-runnable: every insert is guarded on a unique key.

START TRANSACTION;

-- --- 1. Categories ----------------------------------------------------------

INSERT INTO product_categories (name, slug, template_key, description, is_active)
SELECT t.name, t.slug, t.template_key, t.description, TRUE
FROM (
    SELECT 'Audio'             AS name, 'audio'             AS slug, 'audio'     AS template_key, 'Headphones, earphones, and wireless earbuds.'        AS description
    UNION ALL SELECT 'Chargers & cables', 'chargers-cables',   'accessory', 'Chargers, cables, and adapters.'
    UNION ALL SELECT 'Phone accessories', 'phone-accessories', 'accessory', 'Straps, rings, lens covers, and phone batteries.'
    UNION ALL SELECT 'Gaming',            'gaming',            'accessory', 'Controllers and gaming accessories.'
    UNION ALL SELECT 'Computing',         'computing',         'accessory', 'Mice, keyboards, and desk peripherals.'
    UNION ALL SELECT 'Wearables',         'wearables',         'accessory', 'Smart watches and wearable devices.'
    UNION ALL SELECT 'Phones',            'phones',            'phone',     'Handsets.'
) AS t
WHERE NOT EXISTS (SELECT 1 FROM product_categories c WHERE c.slug = t.slug);

-- --- 2. Brands --------------------------------------------------------------

INSERT INTO brands (name, slug, department, is_active)
SELECT t.name, t.slug, 'tech', TRUE
FROM (
    SELECT 'JBL' AS name, 'jbl' AS slug
    UNION ALL SELECT 'Bose',     'bose'
    UNION ALL SELECT 'Marshall', 'marshall'
    UNION ALL SELECT 'Pioneer',  'pioneer'
    UNION ALL SELECT 'Remax',    'remax'
    -- Xiaomi and Itel may already exist from google_pixel_and_misc_phone_cases.sql.
    UNION ALL SELECT 'Xiaomi',   'xiaomi'
    UNION ALL SELECT 'Itel',     'itel'
) AS t
WHERE NOT EXISTS (SELECT 1 FROM brands b WHERE b.slug = t.slug);

-- --- 3. Products ------------------------------------------------------------
-- Joined to the category by slug and LEFT JOINed to the brand, so the unbranded
-- rows load with brand_id NULL instead of a guess.

INSERT INTO products (category_id, brand_id, name, slug, description, is_active)
SELECT c.id, b.id, t.name, t.slug, t.description, TRUE
FROM (
    -- Audio
    SELECT 'audio' AS category_slug, NULL AS brand_slug, 'Casque P9' AS name, 'casque-p9' AS slug, 'Casque audio P9.' AS description
    UNION ALL SELECT 'audio', 'jbl',      'Casque JBL Pure Bass',          'casque-jbl-pure-bass',          'Casque JBL Pure Bass.'
    UNION ALL SELECT 'audio', 'jbl',      'Casque JBL Super Bass',         'casque-jbl-super-bass',         'Casque JBL Super Bass.'
    UNION ALL SELECT 'audio', 'bose',     'Casque Bose',                   'casque-bose',                   'Casque audio Bose.'
    UNION ALL SELECT 'audio', 'marshall', 'Casque Marshall',               'casque-marshall',               'Casque audio Marshall.'
    UNION ALL SELECT 'audio', 'pioneer',  'Casque professionnel Pioneer',  'casque-professionnel-pioneer',  'Casque professionnel Pioneer.'
    UNION ALL SELECT 'audio', NULL,       'AirPods Pro',                   'airpods-pro',                   'Écouteurs sans fil AirPods Pro.'
    UNION ALL SELECT 'audio', NULL,       'Écouteurs Lightning',           'ecouteurs-lightning',           'Écouteurs filaires à connecteur Lightning.'
    -- Chargers & cables
    UNION ALL SELECT 'chargers-cables', 'xiaomi', 'Chargeur rapide Xiaomi Mi 120W', 'chargeur-rapide-xiaomi-mi-120w', 'Chargeur rapide Xiaomi Mi 120W.'
    UNION ALL SELECT 'chargers-cables', NULL,     'Câble chargeur Lightning / Type-C', 'cable-chargeur-lightning-type-c', 'Câble de charge Lightning vers Type-C.'
    UNION ALL SELECT 'chargers-cables', NULL,     'Câble Type-C / Type-C',          'cable-type-c-type-c',            'Câble de charge Type-C vers Type-C.'
    UNION ALL SELECT 'chargers-cables', 'itel',   'Boîte et câble Itel (simple)',   'boite-et-cable-itel-simple',     'Chargeur secteur Itel avec câble (modèle simple).'
    UNION ALL SELECT 'chargers-cables', 'remax',  'Adaptateur OTG Remax (Android)', 'adaptateur-otg-remax-android',   'Adaptateur OTG Remax pour Android.'
    UNION ALL SELECT 'chargers-cables', NULL,     'Adaptateur Jack / Lightning',    'adaptateur-jack-lightning',      'Adaptateur audio Jack vers Lightning.'
    UNION ALL SELECT 'chargers-cables', NULL,     'Adaptateur USB 6 en 1',          'adaptateur-usb-6-en-1',          'Adaptateur USB multiport 6 en 1.'
    -- Name as written on the sheet; see the header note.
    UNION ALL SELECT 'chargers-cables', NULL,     'Dixtup chargeur',                'dixtup-chargeur',                'Chargeur (désignation reprise de la fiche d''inventaire).'
    UNION ALL SELECT 'chargers-cables', NULL,     'Allume-cigare',                  'allume-cigare',                  'Chargeur allume-cigare pour voiture.'
    UNION ALL SELECT 'chargers-cables', NULL,     'Fiche RCA-RCA 3 couleurs',       'fiche-rca-rca-3-couleurs',       'Câble RCA-RCA 3 couleurs.'
    -- Phone accessories
    UNION ALL SELECT 'phone-accessories', NULL,   'Cordon pour téléphone (long)',   'cordon-pour-telephone-long',     'Cordon tour de cou pour téléphone, modèle long.'
    UNION ALL SELECT 'phone-accessories', NULL,   'Bracelet pour téléphone (court)','bracelet-pour-telephone-court',  'Dragonne pour téléphone, modèle court.'
    UNION ALL SELECT 'phone-accessories', NULL,   'Bague pour téléphone',           'bague-pour-telephone',           'Bague support pour téléphone, ronde et stylée.'
    UNION ALL SELECT 'phone-accessories', 'itel',  'Batterie Itel BL-25',           'batterie-itel-bl-25',            'Batterie de remplacement Itel BL-25.'
    UNION ALL SELECT 'phone-accessories', NULL,   'Cache appareil photo iPhone 13 Pro', 'cache-appareil-photo-iphone-13-pro', 'Protection d''objectif pour iPhone 13 Pro.'
    -- Gaming
    UNION ALL SELECT 'gaming',    NULL, 'Manette V8',   'manette-v8',   'Manette de jeu V8.'
    UNION ALL SELECT 'gaming',    NULL, 'Manette PS4',  'manette-ps4',  'Manette de jeu pour PS4.'
    UNION ALL SELECT 'gaming',    NULL, 'Manette Xbox', 'manette-xbox', 'Manette de jeu pour Xbox.'
    -- Computing
    UNION ALL SELECT 'computing', NULL, 'Souris optique filaire', 'souris-optique-filaire', 'Souris optique filaire.'
    -- Wearables
    UNION ALL SELECT 'wearables', NULL, 'Smart watch', 'smart-watch', 'Montre connectée.'
    -- Phones
    UNION ALL SELECT 'phones', 'itel', 'Téléphone Itel 5606', 'telephone-itel-5606', 'Téléphone Itel 5606.'
) AS t
JOIN product_categories c ON c.slug = t.category_slug
LEFT JOIN brands b ON b.slug = t.brand_slug
WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.slug = t.slug);

-- --- 4. Variants ------------------------------------------------------------
-- Items sold without a colour choice (OTG, lens cover, USB hub) keep color NULL
-- and carry the model wording in `label`.

INSERT INTO product_variants (product_id, sku, label, color, attributes, price, stock_quantity, is_active)
SELECT p.id, v.sku, v.label, v.color, JSON_OBJECT('color', v.color), v.price, v.qty, TRUE
FROM (
    -- Audio: 18 pieces
    SELECT 'casque-p9' AS product_slug, 'AUD-P9-ROUGE' AS sku, 'Rouge' AS label, 'Rouge' AS color, 25000.00 AS price, 1 AS qty
    UNION ALL SELECT 'casque-p9',                     'AUD-P9-BLEU',                'Bleu',            'Bleu',            25000.00, 1
    UNION ALL SELECT 'casque-jbl-pure-bass',          'AUD-JBL-PUREBASS-BLEU',      'Bleu',            'Bleu',            40000.00, 1
    -- Sheet: "Gris, Grenat" with a count of 1 -> read as one two-tone unit.
    UNION ALL SELECT 'casque-jbl-super-bass',         'AUD-JBL-SUPERBASS-GRISGREN', 'Gris et grenat',  'Gris et grenat',  45000.00, 1
    UNION ALL SELECT 'casque-bose',                   'AUD-BOSE-ROUGE',             'Rouge',           'Rouge',           40000.00, 1
    UNION ALL SELECT 'casque-bose',                   'AUD-BOSE-NOIR',              'Noir',            'Noir',            40000.00, 1
    UNION ALL SELECT 'casque-bose',                   'AUD-BOSE-VIOLET',            'Violet',          'Violet',          40000.00, 1
    UNION ALL SELECT 'casque-bose',                   'AUD-BOSE-BLEU',              'Bleu',            'Bleu',            40000.00, 1
    -- Sheet lists Noir twice (2 + 2); merged, since one colour is one SKU.
    UNION ALL SELECT 'casque-marshall',               'AUD-MARSHALL-NOIR',          'Noir',            'Noir',            45000.00, 4
    UNION ALL SELECT 'casque-marshall',               'AUD-MARSHALL-BLANC',         'Blanc',           'Blanc',           45000.00, 1
    UNION ALL SELECT 'casque-professionnel-pioneer',  'AUD-PIONEER-GRISNOIR',       'Gris et noir',    'Gris et noir',   130000.00, 1
    UNION ALL SELECT 'airpods-pro',                   'AUD-AIRPODSPRO-BLANC',       'Blanc',           'Blanc',           60000.00, 1
    UNION ALL SELECT 'airpods-pro',                   'AUD-AIRPODSPRO-NOIR',        'Noir',            'Noir',            60000.00, 1
    UNION ALL SELECT 'ecouteurs-lightning',           'AUD-ECOUT-LIGHT-BLANC',      'Blanc',           'Blanc',           15000.00, 2
    -- Chargers & cables: 27 pieces
    UNION ALL SELECT 'chargeur-rapide-xiaomi-mi-120w','CHG-XIA-MI120-BLANC',        'Blanc',           'Blanc',           25000.00, 4
    UNION ALL SELECT 'cable-chargeur-lightning-type-c','CHG-CABLE-LIGHT-TYPEC-BLANC','Blanc',          'Blanc',           15000.00, 3
    UNION ALL SELECT 'cable-type-c-type-c',           'CHG-CABLE-TYPEC-TYPEC-BLANC','Blanc',           'Blanc',           10000.00, 1
    UNION ALL SELECT 'boite-et-cable-itel-simple',    'CHG-ITEL-BOITE-CABLE-NOIR',  'Noir',            'Noir',             5000.00, 3
    UNION ALL SELECT 'adaptateur-otg-remax-android',  'CHG-REMAX-OTG',              'Modèle unique',   NULL,               6000.00, 5
    UNION ALL SELECT 'adaptateur-jack-lightning',     'CHG-ADAPT-JACK-LIGHT-BLANC', 'Blanc',           'Blanc',           20000.00, 2
    UNION ALL SELECT 'adaptateur-usb-6-en-1',         'CHG-ADAPT-USB-6EN1',         'Modèle unique',   NULL,              10000.00, 1
    UNION ALL SELECT 'dixtup-chargeur',               'CHG-DIXTUP-BLANCNOIR',       'Blanc et noir',   'Blanc et noir',    5000.00, 4
    UNION ALL SELECT 'allume-cigare',                 'CHG-ALLUME-CIGARE-NOIR',     'Noir',            'Noir',            20000.00, 2
    UNION ALL SELECT 'fiche-rca-rca-3-couleurs',      'CHG-FICHE-RCA-ROUGE',        'Rouge',           'Rouge',            3000.00, 2
    -- Phone accessories: 45 pieces (43 loaded, see the cordon note)
    UNION ALL SELECT 'cordon-pour-telephone-long',    'ACC-CORDON-VIOLETORANGE',    'Violet et orange','Violet et orange',10000.00, 1
    UNION ALL SELECT 'cordon-pour-telephone-long',    'ACC-CORDON-NOIRGRIS',        'Noir et gris',    'Noir et gris',    10000.00, 1
    -- Sheet says 5 pieces across these three colours; see the header note.
    UNION ALL SELECT 'cordon-pour-telephone-long',    'ACC-CORDON-BLEU',            'Bleu',            'Bleu',            10000.00, 1
    UNION ALL SELECT 'bracelet-pour-telephone-court', 'ACC-BRACELET-AUCHOIX',       'Au choix',        NULL,               6000.00, 17
    UNION ALL SELECT 'bague-pour-telephone',          'ACC-BAGUE-AUCHOIX',          'Au choix',        NULL,               6000.00, 21
    UNION ALL SELECT 'batterie-itel-bl-25',           'ACC-ITEL-BL25-ROUGE',        'Rouge',           'Rouge',           20000.00, 2
    UNION ALL SELECT 'cache-appareil-photo-iphone-13-pro','ACC-CACHE-IPH13PRO',     'Modèle unique',   NULL,              30000.00, 2
    -- Gaming: 6 pieces
    UNION ALL SELECT 'manette-v8',                    'GAM-MANETTE-V8-NOIR',        'Noir',            'Noir',            50000.00, 1
    UNION ALL SELECT 'manette-ps4',                   'GAM-MANETTE-PS4-NOIR',       'Noir',            'Noir',            70000.00, 2
    UNION ALL SELECT 'manette-xbox',                  'GAM-MANETTE-XBOX-NOIR',      'Noir',            'Noir',            50000.00, 3
    -- Computing / Wearables / Phones: 5 pieces
    UNION ALL SELECT 'souris-optique-filaire',        'CMP-SOURIS-OPTIQUE-NOIR',    'Noir',            'Noir',            10000.00, 3
    UNION ALL SELECT 'smart-watch',                   'WCH-SMARTWATCH-NOIR',        'Noir',            'Noir',           120000.00, 1
    UNION ALL SELECT 'telephone-itel-5606',           'PHN-ITEL-5606-BLEU',         'Bleu',            'Bleu',            70000.00, 1
) AS v
JOIN products p ON p.slug = v.product_slug
WHERE NOT EXISTS (SELECT 1 FROM product_variants pv WHERE pv.sku = v.sku);

-- --- 5. Open the stock ledger -----------------------------------------------

INSERT INTO stock_movements (product_variant_id, department, reason, delta, quantity_after, note)
SELECT pv.id, 'tech', 'initial', pv.stock_quantity, pv.stock_quantity,
       'Opening balance from the accessories inventory sheets'
FROM product_variants pv
WHERE pv.sku REGEXP '^(AUD|CHG|ACC|GAM|CMP|WCH|PHN)-'
  AND pv.stock_quantity > 0
  AND NOT EXISTS (SELECT 1 FROM stock_movements sm WHERE sm.product_variant_id = pv.id);

COMMIT;

-- --- Verification ------------------------------------------------------------
-- SELECT c.name AS category, b.name AS brand, p.name, pv.sku, pv.label,
--        pv.price, pv.stock_quantity
-- FROM product_variants pv
--   JOIN products p ON p.id = pv.product_id
--   JOIN product_categories c ON c.id = p.category_id
--   LEFT JOIN brands b ON b.id = p.brand_id
-- WHERE pv.sku REGEXP '^(AUD|CHG|ACC|GAM|CMP|WCH|PHN)-'
-- ORDER BY category, p.name, pv.sku;
--
-- Expected: 29 products, 37 variants, 101 pieces on hand (103 once the cordon
-- line is resolved).
