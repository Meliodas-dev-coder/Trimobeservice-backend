-- 000013_event_planning.up.sql
-- Event planning is Trimobe's third domain. Unlike phones (fixed-price catalog)
-- and cars (time-based availability), an event is a bespoke SERVICE REQUEST that
-- the team quotes manually:
--   Category -> Item -> Transaction, i.e.
--   event_service_categories -> event_services -> event_requests
--                                                   (+ event_request_services)
-- The client browses what we offer (sound, light, catering, artists, decor…),
-- submits a request with the services they want, and an admin sets a quote.
-- Payment reuses the polymorphic manual-payments ledger (payable_type 'event').

-- What we offer, grouped: Sound, Lighting, Catering, Artists, Decoration…
CREATE TABLE event_service_categories (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name        VARCHAR(128) NOT NULL,
    slug        VARCHAR(160) NOT NULL,
    description TEXT,
    icon        VARCHAR(64)  DEFAULT NULL,   -- optional PrimeIcons class, e.g. 'pi pi-volume-up'
    image_url   VARCHAR(512) DEFAULT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_event_service_categories_slug (slug),
    KEY idx_event_service_categories_active (is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- A concrete offering within a category. `from_price` is INDICATIVE only (the
-- real number is the admin's per-request quote); `attributes` holds category-
-- specific specs (e.g. an artist's genre, a PA system's wattage).
CREATE TABLE event_services (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    category_id BIGINT UNSIGNED NOT NULL,
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(280) NOT NULL,
    description TEXT,
    from_price  DECIMAL(12,2) DEFAULT NULL,   -- indicative "from" price, MGA
    price_unit  VARCHAR(32)  DEFAULT NULL,     -- e.g. 'per event', 'per day', 'per artist'
    image_url   VARCHAR(512) DEFAULT NULL,
    attributes  JSON DEFAULT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_event_services_slug (slug),
    KEY idx_event_services_category (category_id),
    KEY idx_event_services_active (is_active),
    CONSTRAINT fk_event_services_category FOREIGN KEY (category_id)
        REFERENCES event_service_categories (id) ON DELETE RESTRICT,
    CONSTRAINT chk_event_services_price CHECK (from_price IS NULL OR from_price >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The transaction: a request to plan an event. Priced by a manual admin quote
-- (`quoted_price`), which is the source of truth for payment. `status` is the
-- planning lifecycle; payment is tracked separately (manual/offline), like
-- orders and bookings. Admins can log phone/walk-in requests (user_id NULL,
-- customer_name set).
CREATE TABLE event_requests (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    user_id        BIGINT UNSIGNED DEFAULT NULL,
    customer_name  VARCHAR(255) DEFAULT NULL,        -- snapshot for phone/walk-in requests
    request_number VARCHAR(32) NOT NULL,             -- app-generated, e.g. EVT-20260706-a1b2c3d4
    event_type     VARCHAR(64) NOT NULL,             -- wedding, corporate, birthday, concert, other…
    status         ENUM('requested','reviewing','quoted','confirmed','in_progress','completed','cancelled')
                   NOT NULL DEFAULT 'requested',
    payment_status ENUM('unpaid','paid','refunded') NOT NULL DEFAULT 'unpaid',
    paid_at        TIMESTAMP NULL DEFAULT NULL,
    event_start    DATETIME NOT NULL,
    event_end      DATETIME DEFAULT NULL,            -- multi-day events; NULL = single day
    location       VARCHAR(512) NOT NULL,
    guest_count    INT DEFAULT NULL,
    budget         DECIMAL(12,2) DEFAULT NULL,       -- client's stated budget (optional)
    quoted_price   DECIMAL(12,2) DEFAULT NULL,       -- admin's quote; drives payment total
    contact_phone  VARCHAR(32) NOT NULL,
    contact_email  VARCHAR(255) DEFAULT NULL,
    note           TEXT,                             -- client's description of the event
    admin_note     TEXT,                             -- internal planner notes
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_event_requests_number (request_number),
    KEY idx_event_requests_user (user_id),
    KEY idx_event_requests_status (status),
    KEY idx_event_requests_payment (payment_status),
    KEY idx_event_requests_start (event_start),
    CONSTRAINT fk_event_requests_user FOREIGN KEY (user_id)
        REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT chk_event_requests_dates CHECK (event_end IS NULL OR event_end >= event_start),
    CONSTRAINT chk_event_requests_amounts CHECK (
        (budget IS NULL OR budget >= 0)
        AND (quoted_price IS NULL OR quoted_price >= 0)
        AND (guest_count IS NULL OR guest_count >= 0)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The services a request selected, snapshotted (name/price frozen) so later
-- catalog edits never rewrite an existing request.
CREATE TABLE event_request_services (
    id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    request_id          BIGINT UNSIGNED NOT NULL,
    service_id          BIGINT UNSIGNED DEFAULT NULL,   -- reference; NULL if the service was later deleted
    service_name        VARCHAR(255) NOT NULL,          -- snapshot
    category_name       VARCHAR(128) DEFAULT NULL,      -- snapshot
    from_price_snapshot DECIMAL(12,2) DEFAULT NULL,     -- snapshot indicative price
    quantity            INT NOT NULL DEFAULT 1,
    note                VARCHAR(512) DEFAULT NULL,
    created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_event_request_services_request (request_id),
    KEY idx_event_request_services_service (service_id),
    CONSTRAINT fk_ers_request FOREIGN KEY (request_id)
        REFERENCES event_requests (id) ON DELETE CASCADE,
    CONSTRAINT fk_ers_service FOREIGN KEY (service_id)
        REFERENCES event_services (id) ON DELETE SET NULL,
    CONSTRAINT chk_ers_quantity CHECK (quantity >= 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Let the manual-payments ledger point at an event too.
ALTER TABLE payments
    MODIFY COLUMN payable_type ENUM('order','booking','event') NOT NULL;

-- Seed the catalog so the client-facing "what we offer" page is populated on a
-- fresh install. Admins can edit/extend it afterwards.
INSERT INTO event_service_categories (name, slug, description, icon, sort_order) VALUES
    ('Sound & PA',        'sound-pa',        'Line arrays, speakers, mixing desks, microphones, and sound engineers.', 'pi pi-volume-up', 1),
    ('Lighting',          'lighting',        'Stage, ambiance, and architectural lighting with technicians.',          'pi pi-sun',       2),
    ('Catering & Cooker', 'catering-cooker', 'Chefs, cooks, buffets, and full catering service for any headcount.',    'pi pi-shopping-bag', 3),
    ('Artists & Performers', 'artists-performers', 'Live bands, DJs, MCs, dancers, and performers who work with us.',   'pi pi-star',      4),
    ('Decoration',        'decoration',      'Floral, drapery, tables, and thematic decoration.',                       'pi pi-palette',   5),
    ('Staging & Structure', 'staging-structure', 'Stages, tents, marquees, and structures.',                           'pi pi-building',  6);

INSERT INTO event_services (category_id, name, slug, description, from_price, price_unit, sort_order) VALUES
    ((SELECT id FROM event_service_categories WHERE slug = 'sound-pa'), 'Line array PA system', 'line-array-pa-system', 'Full line array PA suitable for 300+ guests, with engineer.', 800000.00, 'per event', 1),
    ((SELECT id FROM event_service_categories WHERE slug = 'sound-pa'), 'Speech & conference sound', 'speech-conference-sound', 'Microphones and speakers for talks and conferences.', 350000.00, 'per event', 2),
    ((SELECT id FROM event_service_categories WHERE slug = 'lighting'), 'Stage lighting rig', 'stage-lighting-rig', 'Moving heads, wash, and spot lighting with a technician.', 600000.00, 'per event', 1),
    ((SELECT id FROM event_service_categories WHERE slug = 'lighting'), 'Ambiance & uplighting', 'ambiance-uplighting', 'Uplighting and mood lighting for venues.', 250000.00, 'per event', 2),
    ((SELECT id FROM event_service_categories WHERE slug = 'catering-cooker'), 'Full buffet catering', 'full-buffet-catering', 'Chef-prepared buffet, service staff included.', 25000.00, 'per guest', 1),
    ((SELECT id FROM event_service_categories WHERE slug = 'catering-cooker'), 'Cocktail & canapés', 'cocktail-canapes', 'Cocktail service with canapés and bar staff.', 18000.00, 'per guest', 2),
    ((SELECT id FROM event_service_categories WHERE slug = 'artists-performers'), 'Live band', 'live-band', 'Professional live band for your event.', 1200000.00, 'per event', 1),
    ((SELECT id FROM event_service_categories WHERE slug = 'artists-performers'), 'DJ & MC', 'dj-mc', 'DJ and master of ceremonies for the night.', 500000.00, 'per event', 2),
    ((SELECT id FROM event_service_categories WHERE slug = 'decoration'), 'Full venue decoration', 'full-venue-decoration', 'Floral, drapery, and thematic decoration of the venue.', 700000.00, 'per event', 1),
    ((SELECT id FROM event_service_categories WHERE slug = 'staging-structure'), 'Stage & marquee', 'stage-marquee', 'Stage build plus marquee/tent for guests.', 900000.00, 'per event', 1);
