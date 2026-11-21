ALTER TABLE messages ALTER COLUMN delivered SET DEFAULT false;

CREATE TABLE message_outbox (
    message_id BIGINT PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'retrying', 'delivered', 'failed')),
    attempts INT NOT NULL DEFAULT 0,
    notice_attempts INT NOT NULL DEFAULT 0,
    notice_pending BOOLEAN NOT NULL DEFAULT false,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_token UUID
);

CREATE INDEX idx_message_outbox_ready ON message_outbox(available_at, message_id)
    WHERE status IN ('queued', 'retrying') OR notice_pending;
