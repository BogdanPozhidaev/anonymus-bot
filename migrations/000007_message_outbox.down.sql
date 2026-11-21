DROP TABLE message_outbox;
ALTER TABLE messages ALTER COLUMN delivered SET DEFAULT true;
