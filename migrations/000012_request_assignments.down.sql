DROP INDEX idx_incoming_requests_assigned_page;

ALTER TABLE incoming_requests DROP CONSTRAINT incoming_requests_assignment_check;
UPDATE incoming_requests SET status = 'new' WHERE status = 'in_progress';
ALTER TABLE incoming_requests
    DROP COLUMN session_id,
    DROP COLUMN owner_operator_id,
    DROP CONSTRAINT incoming_requests_status_check,
    ADD CONSTRAINT incoming_requests_status_check CHECK (status IN ('new', 'processed'));
