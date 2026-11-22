-- Block concurrent writes between checking legacy rows and adding the constraint.
-- The migration runner submits this file in one PostgreSQL transaction.
LOCK TABLE sessions IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM sessions WHERE client_user_id = executor_user_id) THEN
        RAISE EXCEPTION 'sessions contain participants assigned to both roles'
            USING HINT = 'Inspect the conflicting sessions and resolve their roles before retrying.';
    END IF;
END;
$$;

ALTER TABLE sessions
    ADD CONSTRAINT sessions_distinct_participants CHECK (client_user_id <> executor_user_id),
    DROP CONSTRAINT sessions_client_user_id_fkey,
    DROP CONSTRAINT sessions_executor_user_id_fkey,
    DROP CONSTRAINT sessions_owner_operator_id_fkey,
    DROP CONSTRAINT sessions_close_requested_by_fkey,
    DROP CONSTRAINT sessions_created_by_fkey;

ALTER TABLE sessions
    ADD CONSTRAINT sessions_client_user_id_fkey FOREIGN KEY (client_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    ADD CONSTRAINT sessions_executor_user_id_fkey FOREIGN KEY (executor_user_id) REFERENCES users(id) ON DELETE RESTRICT,
    ADD CONSTRAINT sessions_owner_operator_id_fkey FOREIGN KEY (owner_operator_id) REFERENCES operators(id) ON DELETE SET NULL,
    ADD CONSTRAINT sessions_close_requested_by_fkey FOREIGN KEY (close_requested_by) REFERENCES users(id) ON DELETE RESTRICT,
    ADD CONSTRAINT sessions_created_by_fkey FOREIGN KEY (created_by) REFERENCES operators(id) ON DELETE RESTRICT;

ALTER TABLE operators DROP CONSTRAINT operators_created_by_fkey;
ALTER TABLE operators ADD CONSTRAINT operators_created_by_fkey
    FOREIGN KEY (created_by) REFERENCES operators(id) ON DELETE RESTRICT;

ALTER TABLE messages DROP CONSTRAINT messages_sender_user_id_fkey;
ALTER TABLE messages ADD CONSTRAINT messages_sender_user_id_fkey
    FOREIGN KEY (sender_user_id) REFERENCES users(id) ON DELETE RESTRICT;

CREATE INDEX idx_users_active_session_id ON users(active_session_id) WHERE active_session_id IS NOT NULL;
CREATE INDEX idx_operators_created_by ON operators(created_by) WHERE created_by IS NOT NULL;
CREATE INDEX idx_sessions_created_by ON sessions(created_by) WHERE created_by IS NOT NULL;
CREATE INDEX idx_sessions_close_requested_by ON sessions(close_requested_by) WHERE close_requested_by IS NOT NULL;
CREATE INDEX idx_messages_sender_user_id ON messages(sender_user_id) WHERE sender_user_id IS NOT NULL;
