-- 000027_event_line_pricing.down.sql

ALTER TABLE event_request_artists
    DROP CHECK chk_era_quoted_fee,
    DROP COLUMN quoted_fee;

ALTER TABLE event_request_services
    DROP CHECK chk_ers_quoted_unit_price,
    DROP COLUMN quoted_unit_price;
