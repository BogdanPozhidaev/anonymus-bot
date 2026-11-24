ALTER TABLE incoming_requests
    DROP CONSTRAINT incoming_requests_status_check,
    ADD CONSTRAINT incoming_requests_status_check CHECK (status IN ('new', 'in_progress', 'processed')),
    ADD COLUMN owner_operator_id BIGINT REFERENCES operators(id) ON DELETE RESTRICT,
    ADD COLUMN session_id BIGINT UNIQUE REFERENCES sessions(id) ON DELETE RESTRICT,
    ADD CONSTRAINT incoming_requests_assignment_check CHECK (
        (status = 'in_progress' AND owner_operator_id IS NOT NULL)
        OR (status <> 'in_progress' AND owner_operator_id IS NULL AND session_id IS NULL)
    );

CREATE INDEX idx_incoming_requests_assigned_page
    ON incoming_requests (owner_operator_id, created_at DESC, id DESC)
    WHERE status = 'in_progress';
