-- 000031_position_hierarchy.down.sql

ALTER TABLE hr_positions
    DROP FOREIGN KEY fk_hr_positions_parent,
    DROP KEY idx_hr_positions_parent,
    DROP COLUMN hierarchy_rank,
    DROP COLUMN parent_position_id;
