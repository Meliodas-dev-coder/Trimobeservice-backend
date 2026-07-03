-- 000004_mobility_cars.up.sql
-- Car hire (with driver): Category -> Car, plus a driver roster.
-- Category carries a default daily rate; each car's own daily_rate is the
-- source of truth at booking time (prefilled from the category default).

CREATE TABLE car_categories (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name               VARCHAR(128) NOT NULL,   -- Luxury, Bus, Cargo, SUV, Sedan...
    slug               VARCHAR(160) NOT NULL,
    description        TEXT,
    default_daily_rate DECIMAL(12,2) NOT NULL DEFAULT 0,   -- baseline; prefills new cars
    image_url          VARCHAR(512) DEFAULT NULL,
    sort_order         INT          NOT NULL DEFAULT 0,
    is_active          BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_car_categories_slug (slug),
    CONSTRAINT chk_car_category_rate CHECK (default_daily_rate >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE cars (
    id                 BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    category_id        BIGINT UNSIGNED NOT NULL,
    name               VARCHAR(255) NOT NULL,   -- display title, e.g. "Mercedes S-Class 2023"
    slug               VARCHAR(280) NOT NULL,
    make               VARCHAR(128) DEFAULT NULL,
    model              VARCHAR(128) DEFAULT NULL,
    year               SMALLINT UNSIGNED DEFAULT NULL,
    registration_plate VARCHAR(32)  DEFAULT NULL,
    color              VARCHAR(64)  DEFAULT NULL,
    seats              SMALLINT UNSIGNED DEFAULT NULL,
    transmission       ENUM('manual','automatic') DEFAULT NULL,
    fuel_type          VARCHAR(32)  DEFAULT NULL,
    daily_rate         DECIMAL(12,2) NOT NULL,  -- real rate; source of truth at booking
    attributes         JSON DEFAULT NULL,       -- category-specific specs (payload_kg, luggage_m3, tail_lift...)
    description        TEXT,
    status             ENUM('available','maintenance','inactive') NOT NULL DEFAULT 'available',
    created_at         TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_cars_slug (slug),
    UNIQUE KEY uq_cars_plate (registration_plate),   -- MySQL allows multiple NULLs here
    KEY idx_cars_category (category_id),
    KEY idx_cars_status (status),
    CONSTRAINT fk_cars_category FOREIGN KEY (category_id)
        REFERENCES car_categories (id) ON DELETE RESTRICT,
    CONSTRAINT chk_cars_rate CHECK (daily_rate >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE car_images (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    car_id     BIGINT UNSIGNED NOT NULL,
    url        VARCHAR(512) NOT NULL,
    alt_text   VARCHAR(255) DEFAULT NULL,
    is_primary BOOLEAN      NOT NULL DEFAULT FALSE,
    sort_order INT          NOT NULL DEFAULT 0,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_car_images_car (car_id),
    CONSTRAINT fk_car_images_car FOREIGN KEY (car_id)
        REFERENCES cars (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE drivers (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    full_name      VARCHAR(255) NOT NULL,
    phone          VARCHAR(32)  NOT NULL,
    license_number VARCHAR(64)  NOT NULL,
    status         ENUM('available','assigned','inactive') NOT NULL DEFAULT 'available',
    notes          TEXT,
    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_drivers_phone (phone),
    UNIQUE KEY uq_drivers_license (license_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
