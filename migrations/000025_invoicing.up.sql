-- 000025_invoicing.up.sql
-- Invoicing sits one layer above the four transaction domains (order, booking,
-- event, healthcare), exactly like the polymorphic `payments` ledger: an invoice
-- references a transaction via (invoiceable_type, invoiceable_id) with no FK
-- (integrity is enforced in application code, as with payments.payable_*).
--
-- An invoice is a FROZEN legal document. Seller identity, buyer identity, every
-- line and every total are snapshotted at issue time so later edits to the order
-- or to org settings never rewrite a document that was already sent.
--
-- Three kinds share one table: 'proforma' (a bill sent before payment),
-- 'final' (the definitive numbered invoice), and 'credit_note' (an "avoir", a
-- negative invoice tied to a refund). Each kind has its own gapless per-year
-- number sequence (see invoice_sequences).
--
-- Payment state is NOT stored here: amount paid / balance due are derived live
-- from the `payments` ledger, keeping money in a single source of truth.

-- Single-row seller identity + invoice defaults (pinned at id = 1, like
-- healthcare_settings). Snapshotted onto each invoice at creation.
CREATE TABLE org_settings (
    id                 BIGINT UNSIGNED NOT NULL DEFAULT 1,
    legal_name         VARCHAR(255) NOT NULL DEFAULT 'Trimobe',
    brand_name         VARCHAR(255) DEFAULT NULL,          -- display name if different from legal
    address            TEXT,                                -- full postal address block
    city               VARCHAR(128) DEFAULT NULL,
    phone              VARCHAR(64)  DEFAULT NULL,
    email              VARCHAR(255) DEFAULT NULL,
    website            VARCHAR(255) DEFAULT NULL,
    logo_url           VARCHAR(512) DEFAULT NULL,
    -- Malagasy fiscal identifiers (blank until registered).
    nif                VARCHAR(64)  DEFAULT NULL,           -- Numero d'Identification Fiscale
    stat               VARCHAR(64)  DEFAULT NULL,           -- Numero statistique
    rcs                VARCHAR(64)  DEFAULT NULL,           -- Registre du Commerce
    -- money / tax defaults
    currency           VARCHAR(8)   NOT NULL DEFAULT 'MGA',
    tax_label          VARCHAR(32)  NOT NULL DEFAULT 'TVA',
    default_tax_rate   DECIMAL(5,2) NOT NULL DEFAULT 0.00,  -- 0 until TVA-registered; then e.g. 20.00
    -- payment instructions shown on unpaid invoices
    payment_terms      VARCHAR(255) DEFAULT NULL,           -- e.g. 'Paiement a reception'
    bank_details       TEXT,                                -- bank name / account / IBAN
    mobile_money       TEXT,                                -- MVola / Orange Money numbers
    -- number prefixes per kind
    invoice_prefix     VARCHAR(16)  NOT NULL DEFAULT 'FAC',
    proforma_prefix    VARCHAR(16)  NOT NULL DEFAULT 'PRO',
    credit_note_prefix VARCHAR(16)  NOT NULL DEFAULT 'AV',
    footer_text        TEXT,                                -- thanks / legal footer
    updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT chk_org_settings_singleton CHECK (id = 1),
    CONSTRAINT chk_org_settings_tax_rate CHECK (default_tax_rate >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE invoices (
    id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    invoiceable_type  ENUM('order','booking','event','healthcare') NOT NULL,
    invoiceable_id    BIGINT UNSIGNED NOT NULL,
    source_number     VARCHAR(32) DEFAULT NULL,             -- the order/booking/request number (display)
    kind              ENUM('proforma','final','credit_note') NOT NULL DEFAULT 'proforma',
    source_invoice_id BIGINT UNSIGNED DEFAULT NULL,         -- credit_note -> the final invoice it reverses
    invoice_number    VARCHAR(32) DEFAULT NULL,             -- assigned at issue; NULL while draft
    status            ENUM('draft','issued','void','credited') NOT NULL DEFAULT 'draft',
    -- seller snapshot (frozen copy of org_settings at creation)
    seller_snapshot   JSON DEFAULT NULL,
    -- buyer snapshot (free text: works for manual/walk-in customers with no account)
    buyer_name        VARCHAR(255) DEFAULT NULL,
    buyer_phone       VARCHAR(64)  DEFAULT NULL,
    buyer_email       VARCHAR(255) DEFAULT NULL,
    buyer_address     TEXT,
    currency          VARCHAR(8)  NOT NULL DEFAULT 'MGA',
    issue_date        DATE DEFAULT NULL,                    -- set at issue
    due_date          DATE DEFAULT NULL,
    -- totals (frozen). `total` may be negative for a credit_note.
    subtotal          DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    discount          DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    tax_rate          DECIMAL(5,2)  NOT NULL DEFAULT 0.00,
    tax_amount        DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    total             DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    notes             TEXT,
    terms             TEXT,
    issued_by         BIGINT UNSIGNED DEFAULT NULL,         -- admin who issued
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_invoices_number (invoice_number),
    KEY idx_invoices_invoiceable (invoiceable_type, invoiceable_id),
    KEY idx_invoices_kind (kind),
    KEY idx_invoices_status (status),
    KEY idx_invoices_source_invoice (source_invoice_id),
    KEY idx_invoices_issued_by (issued_by),
    CONSTRAINT fk_invoices_source_invoice FOREIGN KEY (source_invoice_id)
        REFERENCES invoices (id) ON DELETE SET NULL,
    CONSTRAINT fk_invoices_issued_by FOREIGN KEY (issued_by)
        REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT chk_invoices_tax_rate CHECK (tax_rate >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Line items are snapshotted at issue so the document is self-contained. Amounts
-- may be negative on a credit_note.
CREATE TABLE invoice_lines (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    invoice_id  BIGINT UNSIGNED NOT NULL,
    description VARCHAR(512) NOT NULL,
    detail      VARCHAR(512) DEFAULT NULL,          -- secondary line (variant, dates, staff makeup…)
    quantity    DECIMAL(12,2) NOT NULL DEFAULT 1.00,
    unit_price  DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    line_total  DECIMAL(12,2) NOT NULL DEFAULT 0.00,
    sort_order  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_invoice_lines_invoice (invoice_id),
    CONSTRAINT fk_invoice_lines_invoice FOREIGN KEY (invoice_id)
        REFERENCES invoices (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Gapless per-(kind, year) counters. Incremented inside the issue transaction
-- via the LAST_INSERT_ID() sequence trick, so concurrent issues never collide
-- or leave a gap.
CREATE TABLE invoice_sequences (
    kind        VARCHAR(20) NOT NULL,
    year        INT NOT NULL,
    last_number INT NOT NULL DEFAULT 0,
    PRIMARY KEY (kind, year)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed the single settings row with Trimobe placeholders. The admin edits these
-- (legal name, address, NIF/STAT, bank details, tax rate) before issuing.
INSERT INTO org_settings (id, legal_name, brand_name, city, currency, tax_label, default_tax_rate,
    payment_terms, invoice_prefix, proforma_prefix, credit_note_prefix, footer_text)
VALUES (1, 'Trimobe', 'Trimobe', 'Antananarivo', 'MGA', 'TVA', 0.00,
    'Paiement a reception de la facture', 'FAC', 'PRO', 'AV',
    'Merci de votre confiance.');
