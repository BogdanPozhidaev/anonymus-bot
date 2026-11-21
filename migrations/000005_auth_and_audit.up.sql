ALTER TABLE operators ADD COLUMN auth_version BIGINT NOT NULL DEFAULT 1;
CREATE FUNCTION bump_operator_auth_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.role IS DISTINCT FROM OLD.role OR NEW.status IS DISTINCT FROM OLD.status THEN
        NEW.auth_version := OLD.auth_version + 1;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER operator_auth_version BEFORE UPDATE OF role, status ON operators
    FOR EACH ROW EXECUTE FUNCTION bump_operator_auth_version();

ALTER TABLE audit_log ADD COLUMN actor_type TEXT NOT NULL DEFAULT 'unknown'
    CHECK (actor_type IN ('unknown', 'operator', 'user', 'system'));
UPDATE audit_log SET actor_type = CASE
    WHEN action = 'session_stop_requested' AND actor_id IS NOT NULL THEN 'user'
    WHEN action IN ('operator_created', 'operator_updated', 'operator_sent_message_as',
                    'session_status_updated', 'session_owner_reassigned') AND actor_id IS NOT NULL THEN 'operator'
    WHEN action = 'pii_anonymized' AND actor_id IS NULL THEN 'system'
    ELSE 'unknown' END;
CREATE INDEX idx_audit_log_action_order ON audit_log(action, timestamp DESC, id DESC);
CREATE INDEX idx_audit_log_order ON audit_log(timestamp DESC, id DESC);
CREATE INDEX idx_incoming_requests_created ON incoming_requests(created_at);
