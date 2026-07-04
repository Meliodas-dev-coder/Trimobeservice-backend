-- 000008_remove_car_category_image_url.up.sql
-- Car photos belong to individual cars via car_images, not to car categories.

ALTER TABLE car_categories
    DROP COLUMN image_url;
