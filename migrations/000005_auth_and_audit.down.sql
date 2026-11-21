DROP INDEX idx_incoming_requests_created;
DROP INDEX idx_audit_log_order;
DROP INDEX idx_audit_log_action_order;
ALTER TABLE audit_log DROP COLUMN actor_type;
DROP TRIGGER operator_auth_version ON operators;
DROP FUNCTION bump_operator_auth_version();
ALTER TABLE operators DROP COLUMN auth_version;
