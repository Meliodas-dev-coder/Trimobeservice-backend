-- 000025_invoicing.down.sql
-- Reverse of 000025: drop invoicing tables (children first).

DROP TABLE IF EXISTS invoice_lines;
DROP TABLE IF EXISTS invoice_sequences;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS org_settings;
