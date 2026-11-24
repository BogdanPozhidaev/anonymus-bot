DROP INDEX idx_message_outbox_operator_request;
ALTER TABLE message_outbox
    DROP CONSTRAINT message_outbox_operator_target_check,
    DROP COLUMN request_key,
    DROP COLUMN operator_auth_version,
    DROP COLUMN operator_id,
    DROP COLUMN recipient_user_id;
