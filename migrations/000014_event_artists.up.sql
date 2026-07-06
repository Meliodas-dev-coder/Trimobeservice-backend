-- 000014_event_artists.up.sql
-- Christian/gospel artists Trimobe works with, as a browsable public roster that
-- clients can hand-pick when they send an event request. An artist is a rich,
-- public profile (photo, bio, genres, sample links) — closer to the drivers
-- roster than to a generic event service. Availability is coordinated off-system
-- for now (no date-overlap enforcement).
--
-- Multi-value fields (genres, formats, languages, occasions, links) are stored as
-- comma/newline-separated text so the config-driven admin form round-trips them
-- cleanly; the client splits them into tags for display and filtering.

CREATE TABLE artists (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    stage_name   VARCHAR(255) NOT NULL,          -- stage / ministry name
    slug         VARCHAR(280) NOT NULL,
    tagline      VARCHAR(255) DEFAULT NULL,
    bio          TEXT,
    home_base    VARCHAR(160) DEFAULT NULL,      -- city / region
    photo_url    VARCHAR(512) DEFAULT NULL,
    group_size   VARCHAR(64)  DEFAULT NULL,      -- e.g. 'Solo', 'Band of 6', 'Choir 20+'
    genres       VARCHAR(512) DEFAULT NULL,      -- comma-separated: gospel, worship, choir…
    formats      VARCHAR(512) DEFAULT NULL,      -- comma-separated: worship leader, soloist, band…
    languages    VARCHAR(255) DEFAULT NULL,      -- comma-separated: Malagasy, French, English
    occasions    VARCHAR(512) DEFAULT NULL,      -- comma-separated: Sunday service, crusade, wedding…
    sample_links TEXT,                           -- newline/comma-separated URLs (YouTube/Spotify/Facebook)
    social_links TEXT,                           -- newline/comma-separated URLs
    from_fee     DECIMAL(12,2) DEFAULT NULL,     -- indicative "from" fee, MGA
    is_featured  BOOLEAN NOT NULL DEFAULT FALSE,
    sort_order   INT NOT NULL DEFAULT 0,
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_artists_slug (slug),
    KEY idx_artists_active (is_active),
    KEY idx_artists_featured (is_featured),
    CONSTRAINT chk_artists_fee CHECK (from_fee IS NULL OR from_fee >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- The artists a request named, snapshotted (name/fee frozen) so later roster
-- edits never rewrite an existing request.
CREATE TABLE event_request_artists (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    request_id    BIGINT UNSIGNED NOT NULL,
    artist_id     BIGINT UNSIGNED DEFAULT NULL,   -- reference; NULL if the artist was later deleted
    artist_name   VARCHAR(255) NOT NULL,          -- snapshot
    fee_snapshot  DECIMAL(12,2) DEFAULT NULL,     -- snapshot indicative fee
    note          VARCHAR(512) DEFAULT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_era_request (request_id),
    KEY idx_era_artist (artist_id),
    CONSTRAINT fk_era_request FOREIGN KEY (request_id)
        REFERENCES event_requests (id) ON DELETE CASCADE,
    CONSTRAINT fk_era_artist FOREIGN KEY (artist_id)
        REFERENCES artists (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed a few gospel artists so the public roster is populated on a fresh install.
INSERT INTO artists (stage_name, slug, tagline, bio, home_base, group_size, genres, formats, languages, occasions, from_fee, is_featured, sort_order) VALUES
    ('Voninavo Praise', 'voninavo-praise',
     'Worship leader for services and crusades',
     'Voninavo Praise leads congregations into worship with a heart for revival, drawing on Malagasy gospel and contemporary praise.',
     'Antananarivo', 'Solo + backing',
     'praise & worship, contemporary Christian', 'worship leader, soloist',
     'Malagasy, French', 'Sunday service, crusade/convention, concert',
     900000.00, TRUE, 1),
    ('Chorale Fiderana', 'chorale-fiderana',
     'Gospel choir for weddings and conventions',
     'A 24-voice gospel choir known for rich harmonies in Malagasy and French, equally at home at weddings, funerals, and large conventions.',
     'Antananarivo', 'Choir 24',
     'gospel choir, hymns/choral', 'choir',
     'Malagasy, French', 'wedding, funeral/memorial, crusade/convention',
     1500000.00, TRUE, 2),
    ('Rija & The Testimony Band', 'rija-testimony-band',
     'Afro-gospel band for concerts and youth events',
     'High-energy afro-gospel band blending live instruments and testimony-driven songwriting for concerts and youth gatherings.',
     'Antananarivo', 'Band of 6',
     'afro-gospel, contemporary Christian', 'band',
     'Malagasy, French, English', 'concert, youth event, crusade/convention',
     1200000.00, FALSE, 3),
    ('Hanitra Soloist', 'hanitra-soloist',
     'Gospel soloist for weddings and services',
     'A gospel soloist with a warm, intimate style, often booked for wedding ceremonies, baptisms, and Sunday services.',
     'Toamasina', 'Solo',
     'gospel, hymns/choral', 'soloist',
     'Malagasy', 'wedding, baptism, Sunday service, funeral/memorial',
     400000.00, FALSE, 4);
