BEGIN;

DO $$
BEGIN
    IF current_user <> 'anonymus_runtime' OR
        EXISTS (SELECT 1 FROM pg_roles WHERE rolname = current_user
            AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR rolinherit)) THEN
        RAISE EXCEPTION 'unexpected runtime identity or role attributes';
    END IF;
END;
$$;

INSERT INTO users (telegram_id) VALUES (99001);
INSERT INTO operators (name, login, role) VALUES ('Permissions fixture', 'permissions-fixture', 'admin');
INSERT INTO sessions (title, client_user_id, owner_operator_id)
SELECT 'Permissions fixture', users.id, operators.id FROM users, operators
WHERE users.telegram_id = 99001 AND operators.login = 'permissions-fixture';
LOCK TABLE sessions IN SHARE MODE;
UPDATE users SET active_session_id = (SELECT id FROM sessions WHERE title = 'Permissions fixture')
WHERE telegram_id = 99001;
INSERT INTO session_invites (session_id, role, token_hash, expires_at)
SELECT id, 'executor', decode(repeat('01', 32), 'hex'), now() FROM sessions WHERE title = 'Permissions fixture';
DELETE FROM session_invites WHERE session_id = (SELECT id FROM sessions WHERE title = 'Permissions fixture');
INSERT INTO messages (session_id, sender_role, content_type)
SELECT id, 'client', 'text' FROM sessions WHERE title = 'Permissions fixture';
INSERT INTO message_outbox (message_id) SELECT messages.id FROM messages JOIN sessions
ON messages.session_id = sessions.id WHERE sessions.title = 'Permissions fixture';
DELETE FROM messages WHERE session_id = (SELECT id FROM sessions WHERE title = 'Permissions fixture');
INSERT INTO incoming_requests (telegram_id) VALUES (99002);
DELETE FROM incoming_requests WHERE telegram_id = 99002;
INSERT INTO audit_log (action) VALUES ('fixture_role_check');
UPDATE audit_log SET payload = '{"fixture":true}' WHERE action = 'fixture_role_check';

DO $$
DECLARE
    statement text;
BEGIN
    IF EXISTS (SELECT 1 FROM message_outbox) THEN
        RAISE EXCEPTION 'message outbox was not removed with its message';
    END IF;
    FOREACH statement IN ARRAY ARRAY[
        'CREATE SCHEMA fixture_denied',
        'CREATE TABLE public.fixture_denied (id int)',
        'CREATE TEMP TABLE fixture_denied (id int)',
        'ALTER TABLE public.users ADD COLUMN fixture_denied int',
        'TRUNCATE public.audit_log',
        'DELETE FROM public.users WHERE telegram_id = 99001',
        'DELETE FROM public.operators WHERE login = ''permissions-fixture''',
        'UPDATE public.audit_log SET action = ''rewritten'' WHERE action = ''fixture_role_check''',
        'DELETE FROM public.audit_log WHERE action = ''fixture_role_check''',
        'UPDATE public.schema_migrations SET dirty = true',
        'SELECT singleton FROM public.admin_bootstrap'
    ] LOOP
        BEGIN
            EXECUTE statement;
            RAISE EXCEPTION 'runtime role unexpectedly allowed: %', statement;
        EXCEPTION WHEN insufficient_privilege THEN
            NULL;
        END;
    END LOOP;
END;
$$;

ROLLBACK;
