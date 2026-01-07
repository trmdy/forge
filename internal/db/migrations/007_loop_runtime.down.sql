-- Rollback migration: 007_loop_runtime

DROP TRIGGER IF EXISTS update_loops_timestamp;
DROP TRIGGER IF EXISTS update_pools_timestamp;
DROP TRIGGER IF EXISTS update_profiles_timestamp;

DROP TABLE IF EXISTS loop_runs;
DROP TABLE IF EXISTS loop_queue_items;
DROP TABLE IF EXISTS loops;
DROP TABLE IF EXISTS pool_members;
DROP TABLE IF EXISTS pools;
DROP TABLE IF EXISTS profiles;
