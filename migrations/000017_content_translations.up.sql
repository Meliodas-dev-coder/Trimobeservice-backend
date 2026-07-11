ALTER TABLE product_categories ADD COLUMN translations JSON NULL AFTER description;
ALTER TABLE brands ADD COLUMN translations JSON NULL AFTER slug;
ALTER TABLE products ADD COLUMN translations JSON NULL AFTER description;

ALTER TABLE car_categories ADD COLUMN translations JSON NULL AFTER description;
ALTER TABLE cars ADD COLUMN translations JSON NULL AFTER description;

ALTER TABLE event_service_categories ADD COLUMN translations JSON NULL AFTER description;
ALTER TABLE event_services ADD COLUMN translations JSON NULL AFTER description;
ALTER TABLE artists ADD COLUMN translations JSON NULL AFTER bio;

ALTER TABLE practitioners ADD COLUMN translations JSON NULL AFTER bio;
ALTER TABLE healthcare_service_categories ADD COLUMN translations JSON NULL AFTER description;
ALTER TABLE healthcare_services ADD COLUMN translations JSON NULL AFTER description;
ALTER TABLE healthcare_settings ADD COLUMN translations JSON NULL AFTER emergency_note;
