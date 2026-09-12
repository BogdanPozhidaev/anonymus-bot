ALTER TABLE messages
    ADD COLUMN delivered BOOLEAN NOT NULL DEFAULT true;

CREATE INDEX idx_messages_delivered ON messages(session_id, delivered) WHERE delivered = false;