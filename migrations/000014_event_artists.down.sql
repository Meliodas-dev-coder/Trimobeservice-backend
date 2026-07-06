-- 000014_event_artists.down.sql
-- Reverse of 000014: drop the request join first, then the roster.

DROP TABLE IF EXISTS event_request_artists;
DROP TABLE IF EXISTS artists;
