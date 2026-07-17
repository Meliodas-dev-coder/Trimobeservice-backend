-- 000027_event_line_pricing.up.sql
-- Event quotes become itemized. Previously an event request carried a single
-- `quoted_price`; the invoice could only show one lump line. The admin now
-- prices each selected service and artist, and `quoted_price` becomes the SUM of
-- those line prices — so the invoice details every cost.
--
-- These agreed amounts are distinct from the indicative `*_snapshot` columns
-- (the catalog "from" prices frozen at request time, kept for reference/compare).
-- NULL = the line has not been priced yet (also the state for legacy requests
-- quoted the old single-amount way, which the invoice still renders as one line).

ALTER TABLE event_request_services
    ADD COLUMN quoted_unit_price DECIMAL(12,2) NULL DEFAULT NULL AFTER from_price_snapshot,
    ADD CONSTRAINT chk_ers_quoted_unit_price CHECK (quoted_unit_price IS NULL OR quoted_unit_price >= 0);

ALTER TABLE event_request_artists
    ADD COLUMN quoted_fee DECIMAL(12,2) NULL DEFAULT NULL AFTER fee_snapshot,
    ADD CONSTRAINT chk_era_quoted_fee CHECK (quoted_fee IS NULL OR quoted_fee >= 0);
