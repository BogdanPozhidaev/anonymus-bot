DROP INDEX IF EXISTS idx_sessions_closed_at;
ALTER TABLE sessions DROP COLUMN closed_at;