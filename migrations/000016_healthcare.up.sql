-- 000016_healthcare.up.sql
-- Healthcare is Trimobe's fourth domain. It is a hybrid of the two newest ones:
-- the home-consultation side behaves like event planning (a form the team quotes
-- manually) and the package side behaves like a booking (a coverage window with
-- practitioners assigned from a roster, like drivers on cars). It follows the
-- platform's Category -> Item -> Transaction shape:
--   healthcare_service_categories -> healthcare_services -> healthcare_requests
--                                    (+ package staff)      (+ request assignments)
-- Practitioners (doctors & nurses) are a shared roster, modeled like `drivers`.
-- Payment reuses the polymorphic manual-payments ledger (payable_type 'healthcare').

-- The doctor/nurse roster. Modeled on `drivers`: a simple people table an admin
-- assigns to requests. license_number is unique but nullable (nurses may lack
-- one; MySQL permits multiple NULLs under a UNIQUE key).
CREATE TABLE practitioners (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    type           ENUM('doctor','nurse') NOT NULL,
    full_name      VARCHAR(255) NOT NULL,
    specialty      VARCHAR(128) DEFAULT NULL,   -- e.g. General medicine, Pediatrics
    phone          VARCHAR(32)  NOT NULL,
    email          VARCHAR(255) DEFAULT NULL,
    license_number VARCHAR(64)  DEFAULT NULL,
    bio            TEXT,
    photo_url      VARCHAR(512) DEFAULT NULL,
    status         ENUM('active','inactive') NOT NULL DEFAULT 'active',
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_practitioners_phone (phone),
    UNIQUE KEY uq_practitioners_license (license_number),
    KEY idx_practitioners_type (type),
    KEY idx_practitioners_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- What we offer, grouped: Home Consultation, Nursing Care, Care Packages…
CREATE TABLE healthcare_service_categories (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(128) NOT NULL,
    slug        VARCHAR(160) NOT NULL,
    description TEXT,
    icon        VARCHAR(64)  DEFAULT NULL,   -- optional PrimeIcons class, e.g. 'pi pi-home'
    image_url   VARCHAR(512) DEFAULT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_healthcare_service_categories_slug (slug),
    KEY idx_healthcare_service_categories_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- A concrete offering. Two flavors via `service_type`:
--   'consultation' — one-off home visit; `from_price` is INDICATIVE (the real
--                    number is the admin's per-request quote).
--   'package'      — a fixed-price bundle over `duration_days` (e.g. one month),
--                    whose staff makeup lives in healthcare_package_staff.
CREATE TABLE healthcare_services (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    category_id   BIGINT UNSIGNED NOT NULL,
    name          VARCHAR(255) NOT NULL,
    slug          VARCHAR(280) NOT NULL,
    description   TEXT,
    service_type  ENUM('consultation','package') NOT NULL DEFAULT 'consultation',
    from_price    DECIMAL(12,2) DEFAULT NULL,   -- indicative "from" price (consultations), MGA
    price         DECIMAL(12,2) DEFAULT NULL,   -- fixed catalog price (packages), MGA
    price_unit    VARCHAR(32)  DEFAULT NULL,     -- e.g. 'per visit', 'per month'
    duration_days INT DEFAULT NULL,              -- packages: coverage window length
    image_url     VARCHAR(512) DEFAULT NULL,
    attributes    JSON DEFAULT NULL,
    sort_order    INT NOT NULL DEFAULT 0,
    is_active     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_healthcare_services_slug (slug),
    KEY idx_healthcare_services_category (category_id),
    KEY idx_healthcare_services_type (service_type),
    KEY idx_healthcare_services_active (is_active),
    CONSTRAINT fk_healthcare_services_category FOREIGN KEY (category_id)
        REFERENCES healthcare_service_categories (id) ON DELETE RESTRICT,
    CONSTRAINT chk_healthcare_services_amounts CHECK (
        (from_price IS NULL OR from_price >= 0)
        AND (price IS NULL OR price >= 0)
        AND (duration_days IS NULL OR duration_days > 0)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The staff makeup of a package, one row per practitioner type (e.g. doctor x1,
-- nurse x2). Drives the admin's "you still need to assign N nurses" hint.
CREATE TABLE healthcare_package_staff (
    id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    service_id        BIGINT UNSIGNED NOT NULL,
    practitioner_type ENUM('doctor','nurse') NOT NULL,
    quantity          INT NOT NULL DEFAULT 1,
    created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_healthcare_package_staff (service_id, practitioner_type),
    KEY idx_healthcare_package_staff_service (service_id),
    CONSTRAINT fk_healthcare_package_staff_service FOREIGN KEY (service_id)
        REFERENCES healthcare_services (id) ON DELETE CASCADE,
    CONSTRAINT chk_healthcare_package_staff_qty CHECK (quantity >= 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The transaction: a home-care request. `request_type` mirrors the chosen
-- service's type. Consultations are quote-priced (quoted_price NULL until an
-- admin sets it); packages are fixed (quoted_price seeded from the package price
-- at creation so payment can proceed once confirmed). Name/price are snapshotted
-- so later catalog edits never reprice a live request. Admins can log phone/
-- walk-in requests (user_id NULL, customer_name set), like orders/bookings/events.
CREATE TABLE healthcare_requests (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id        BIGINT UNSIGNED DEFAULT NULL,
    customer_name  VARCHAR(255) DEFAULT NULL,        -- snapshot for phone/walk-in
    request_number VARCHAR(32) NOT NULL,             -- app-generated, e.g. HC-20260709-a1b2c3d4
    request_type   ENUM('consultation','package') NOT NULL DEFAULT 'consultation',
    service_id     BIGINT UNSIGNED DEFAULT NULL,     -- catalog reference; NULL for a free-form consultation
    service_name   VARCHAR(255) DEFAULT NULL,        -- snapshot
    category_name  VARCHAR(128) DEFAULT NULL,        -- snapshot
    price_snapshot DECIMAL(12,2) DEFAULT NULL,       -- snapshot: package price or consultation from_price
    -- patient
    patient_name   VARCHAR(255) NOT NULL,
    patient_age    INT DEFAULT NULL,
    patient_gender ENUM('male','female','other') DEFAULT NULL,
    -- scheduling (consultation uses preferred_at; package uses start_at/end_at)
    preferred_at   DATETIME DEFAULT NULL,
    start_at       DATETIME DEFAULT NULL,
    end_at         DATETIME DEFAULT NULL,
    -- location & content
    address        VARCHAR(512) NOT NULL,            -- home visit address
    symptoms       TEXT,                             -- reason / description of need
    -- lifecycle & payment (payment tracked separately, like the other domains)
    status         ENUM('requested','reviewing','quoted','confirmed','assigned','in_progress','completed','cancelled')
                   NOT NULL DEFAULT 'requested',
    payment_status ENUM('unpaid','paid','refunded') NOT NULL DEFAULT 'unpaid',
    paid_at        TIMESTAMP NULL DEFAULT NULL,
    quoted_price   DECIMAL(12,2) DEFAULT NULL,       -- admin quote / package total; drives payment
    contact_phone  VARCHAR(32) NOT NULL,
    contact_email  VARCHAR(255) DEFAULT NULL,
    note           TEXT,                             -- client's note
    admin_note     TEXT,                             -- internal notes
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_healthcare_requests_number (request_number),
    KEY idx_healthcare_requests_user (user_id),
    KEY idx_healthcare_requests_status (status),
    KEY idx_healthcare_requests_payment (payment_status),
    KEY idx_healthcare_requests_service (service_id),
    CONSTRAINT fk_healthcare_requests_user FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_healthcare_requests_service FOREIGN KEY (service_id)
        REFERENCES healthcare_services (id) ON DELETE SET NULL,
    CONSTRAINT chk_healthcare_requests_dates CHECK (
        start_at IS NULL OR end_at IS NULL OR end_at >= start_at
    ),
    CONSTRAINT chk_healthcare_requests_amounts CHECK (
        (price_snapshot IS NULL OR price_snapshot >= 0)
        AND (quoted_price IS NULL OR quoted_price >= 0)
        AND (patient_age IS NULL OR patient_age >= 0)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Which practitioners serve a request. Many rows per request cover a package's
-- "1 doctor + 2 nurses". Name/type snapshotted so deleting a practitioner keeps
-- the record readable (practitioner_id then SET NULL).
CREATE TABLE healthcare_request_assignments (
    id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    request_id        BIGINT UNSIGNED NOT NULL,
    practitioner_id   BIGINT UNSIGNED DEFAULT NULL,
    practitioner_name VARCHAR(255) NOT NULL,          -- snapshot
    practitioner_type ENUM('doctor','nurse') NOT NULL,-- snapshot
    note              VARCHAR(512) DEFAULT NULL,
    assigned_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_hra_request_practitioner (request_id, practitioner_id),
    KEY idx_hra_request (request_id),
    KEY idx_hra_practitioner (practitioner_id),
    CONSTRAINT fk_hra_request FOREIGN KEY (request_id)
        REFERENCES healthcare_requests (id) ON DELETE CASCADE,
    CONSTRAINT fk_hra_practitioner FOREIGN KEY (practitioner_id)
        REFERENCES practitioners (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Editable healthcare settings (currently the emergency contact shown atop the
-- client healthcare page). Single row, pinned at id = 1.
CREATE TABLE healthcare_settings (
    id              BIGINT UNSIGNED NOT NULL DEFAULT 1,
    emergency_phone VARCHAR(32)  DEFAULT NULL,
    emergency_hours VARCHAR(128) DEFAULT NULL,   -- e.g. '24/7'
    emergency_note  TEXT,                         -- short guidance blurb
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    CONSTRAINT chk_healthcare_settings_singleton CHECK (id = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Let the manual-payments ledger point at a healthcare request too.
ALTER TABLE payments
    MODIFY COLUMN payable_type ENUM('order','booking','event','healthcare') NOT NULL;

-- Seed the emergency contact and a starter catalog so the client page is
-- populated on a fresh install. Admins can edit/extend everything afterwards.
INSERT INTO healthcare_settings (id, emergency_phone, emergency_hours, emergency_note) VALUES
    (1, '+261 20 22 000 00', '24/7',
     'For a life-threatening emergency, call this number immediately. For non-urgent needs, request a home consultation below.');

INSERT INTO healthcare_service_categories (name, slug, description, icon, sort_order) VALUES
    ('Home Consultation', 'home-consultation', 'A doctor or nurse visits you at home for a one-off consultation.', 'pi pi-home', 1),
    ('Nursing Care',       'nursing-care',      'Home nursing: injections, wound care, monitoring, and daily assistance.', 'pi pi-heart', 2),
    ('Care Packages',      'care-packages',     'Ongoing packages combining doctors and nurses over a period.', 'pi pi-calendar', 3);

INSERT INTO healthcare_services (category_id, name, slug, description, service_type, from_price, price, price_unit, duration_days, sort_order) VALUES
    ((SELECT id FROM healthcare_service_categories WHERE slug = 'home-consultation'), 'General home consultation', 'general-home-consultation', 'A general practitioner visits your home to assess and advise.', 'consultation', 50000.00, NULL, 'per visit', NULL, 1),
    ((SELECT id FROM healthcare_service_categories WHERE slug = 'home-consultation'), 'Pediatric home consultation', 'pediatric-home-consultation', 'A pediatrician visits to see your child at home.', 'consultation', 70000.00, NULL, 'per visit', NULL, 2),
    ((SELECT id FROM healthcare_service_categories WHERE slug = 'nursing-care'), 'Home nursing visit', 'home-nursing-visit', 'A nurse comes to your home for injections, wound care, or monitoring.', 'consultation', 30000.00, NULL, 'per visit', NULL, 1),
    ((SELECT id FROM healthcare_service_categories WHERE slug = 'care-packages'), 'Monthly home care — 1 doctor, 2 nurses', 'monthly-home-care-1-doctor-2-nurses', 'One month of coverage: a doctor on call plus two nurses rotating for daily care.', 'package', NULL, 3000000.00, 'per month', 30, 1);

INSERT INTO healthcare_package_staff (service_id, practitioner_type, quantity) VALUES
    ((SELECT id FROM healthcare_services WHERE slug = 'monthly-home-care-1-doctor-2-nurses'), 'doctor', 1),
    ((SELECT id FROM healthcare_services WHERE slug = 'monthly-home-care-1-doctor-2-nurses'), 'nurse', 2);

INSERT INTO practitioners (type, full_name, specialty, phone, status) VALUES
    ('doctor', 'Dr. Hery Rakoto',  'General medicine', '+261 34 00 000 01', 'active'),
    ('doctor', 'Dr. Naina Andria', 'Pediatrics',       '+261 34 00 000 02', 'active'),
    ('nurse',  'Mamy Rasoa',       NULL,               '+261 34 00 000 03', 'active'),
    ('nurse',  'Tiana Rabe',       NULL,               '+261 34 00 000 04', 'active');
