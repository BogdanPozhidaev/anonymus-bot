ALTER TABLE message_outbox
    ADD COLUMN recipient_user_id BIGINT,
    ADD COLUMN operator_id BIGINT,
    ADD COLUMN operator_auth_version BIGINT,
    ADD COLUMN request_key UUID,
    ADD CONSTRAINT message_outbox_operator_target_check CHECK (
        (recipient_user_id IS NULL AND operator_id IS NULL AND operator_auth_version IS NULL AND request_key IS NULL)
        OR (recipient_user_id IS NOT NULL AND operator_id IS NOT NULL AND operator_auth_version IS NOT NULL)
    );

CREATE UNIQUE INDEX idx_message_outbox_operator_request
    ON message_outbox (operator_id, request_key) WHERE request_key IS NOT NULL;
