-- 000034_hr_contract_documents.up.sql
--
-- Issued contract / agreement documents. A document is produced from an
-- hr_contract_templates body, but once issued it is a FROZEN record: the
-- rendered text is stored here, so later edits to the template can never alter
-- an already-signed document. This mirrors the invoicing rule that an issued
-- document is voided, never rewritten.
--
-- The party is deliberately flexible, matching the memo_deal decision:
--   employee_id  set  -> the counterparty is an HR employee
--   employee_id  NULL -> a free-text external party (contractor, driver, artist),
--                        the same escape hatch invoices use for walk-in buyers
-- contract_id optionally links an employment document to the structured terms in
-- hr_contracts, which stay the source of truth for salary/dates.
--
-- template_id is ON DELETE SET NULL because the frozen snapshot (code, name,
-- version) is what the document legally depends on — not the live template row.
--
-- Column named token_values, not `values`: VALUES is reserved in MySQL 8 and the
-- HR resource engine builds unquoted column lists (same lesson as `rank`).

CREATE TABLE hr_contract_documents (
    id               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    kind             ENUM('employment','memo_deal') NOT NULL,
    status           ENUM('draft','issued','signed','void') NOT NULL DEFAULT 'draft',
    reference        VARCHAR(40) NULL,

    -- frozen template identity
    template_id      BIGINT UNSIGNED NULL,
    template_code    VARCHAR(50)  NOT NULL,
    template_name    VARCHAR(150) NOT NULL,
    template_version INT NOT NULL DEFAULT 1,

    -- counterparty: an employee, or a free-text external party
    employee_id      BIGINT UNSIGNED NULL,
    contract_id      BIGINT UNSIGNED NULL,
    party_name       VARCHAR(255) NOT NULL,
    party_address    TEXT NULL,
    party_id_number  VARCHAR(80)  NULL,
    party_phone      VARCHAR(64)  NULL,
    party_email      VARCHAR(255) NULL,

    -- document facts + the frozen result
    place            VARCHAR(120) NULL,
    issue_date       DATE NULL,
    token_values     JSON NULL,
    body_rendered    MEDIUMTEXT NOT NULL,

    issued_at        TIMESTAMP NULL,
    signed_at        TIMESTAMP NULL,
    voided_at        TIMESTAMP NULL,
    void_reason      VARCHAR(255) NULL,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY uq_hr_contract_documents_reference (reference),
    KEY idx_hr_contract_documents_status (status, kind),
    KEY idx_hr_contract_documents_employee (employee_id),
    CONSTRAINT fk_hr_contract_documents_template
        FOREIGN KEY (template_id) REFERENCES hr_contract_templates(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_contract_documents_employee
        FOREIGN KEY (employee_id) REFERENCES hr_employees(id) ON DELETE SET NULL,
    CONSTRAINT fk_hr_contract_documents_contract
        FOREIGN KEY (contract_id) REFERENCES hr_contracts(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Gapless per-(kind, year) reference counter, using the same LAST_INSERT_ID
-- pattern as invoice_sequences so concurrent issues get distinct numbers.
CREATE TABLE hr_contract_sequences (
    kind        VARCHAR(20) NOT NULL,
    year        SMALLINT UNSIGNED NOT NULL,
    last_number INT UNSIGNED NOT NULL DEFAULT 0,
    PRIMARY KEY (kind, year)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
