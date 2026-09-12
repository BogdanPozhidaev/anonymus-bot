DROP INDEX IF EXISTS idx_messages_delivered;
ALTER TABLE messages DROP COLUMN delivered;