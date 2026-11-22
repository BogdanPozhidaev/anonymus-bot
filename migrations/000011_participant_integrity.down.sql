DROP INDEX idx_users_active_session_id;
DROP INDEX idx_operators_created_by;
DROP INDEX idx_sessions_created_by;
DROP INDEX idx_sessions_close_requested_by;
DROP INDEX idx_messages_sender_user_id;

ALTER TABLE sessions
    DROP CONSTRAINT sessions_distinct_participants,
    DROP CONSTRAINT sessions_client_user_id_fkey,
    DROP CONSTRAINT sessions_executor_user_id_fkey,
    DROP CONSTRAINT sessions_owner_operator_id_fkey,
    DROP CONSTRAINT sessions_close_requested_by_fkey,
    DROP CONSTRAINT sessions_created_by_fkey;

ALTER TABLE sessions
    ADD CONSTRAINT sessions_client_user_id_fkey FOREIGN KEY (client_user_id) REFERENCES users(id),
    ADD CONSTRAINT sessions_executor_user_id_fkey FOREIGN KEY (executor_user_id) REFERENCES users(id),
    ADD CONSTRAINT sessions_owner_operator_id_fkey FOREIGN KEY (owner_operator_id) REFERENCES operators(id),
    ADD CONSTRAINT sessions_close_requested_by_fkey FOREIGN KEY (close_requested_by) REFERENCES users(id),
    ADD CONSTRAINT sessions_created_by_fkey FOREIGN KEY (created_by) REFERENCES operators(id);

ALTER TABLE operators DROP CONSTRAINT operators_created_by_fkey;
ALTER TABLE operators ADD CONSTRAINT operators_created_by_fkey FOREIGN KEY (created_by) REFERENCES operators(id);

ALTER TABLE messages DROP CONSTRAINT messages_sender_user_id_fkey;
ALTER TABLE messages ADD CONSTRAINT messages_sender_user_id_fkey FOREIGN KEY (sender_user_id) REFERENCES users(id);
