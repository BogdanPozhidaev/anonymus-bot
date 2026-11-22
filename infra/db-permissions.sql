SET LOCAL search_path = pg_catalog;
SET LOCAL password_encryption = 'scram-sha-256';
SELECT pg_advisory_xact_lock(hashtextextended('anonymus:runtime-role', 0));

DO $$
DECLARE
    runtime_oid oid;
BEGIN
    IF current_user = 'anonymus_runtime' THEN
        RAISE EXCEPTION 'the database owner must use a different name from anonymus_runtime';
    END IF;
    SELECT oid INTO runtime_oid FROM pg_roles WHERE rolname = 'anonymus_runtime';
    IF runtime_oid IS NULL THEN
        CREATE ROLE anonymus_runtime;
    ELSIF EXISTS (SELECT 1 FROM pg_auth_members WHERE member = runtime_oid)
        OR EXISTS (SELECT 1 FROM pg_shdepend WHERE refclassid = 'pg_authid'::regclass
                   AND refobjid = runtime_oid AND deptype = 'o') THEN
        RAISE EXCEPTION 'anonymus_runtime must have no role memberships or owned objects';
    END IF;
END;
$$;

ALTER ROLE anonymus_runtime WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
    NOINHERIT NOREPLICATION NOBYPASSRLS;
\password anonymus_runtime

SELECT format('REVOKE ALL ON DATABASE %I FROM anonymus_runtime', current_database()) \gexec
SELECT format('REVOKE CREATE, TEMPORARY ON DATABASE %I FROM PUBLIC', current_database()) \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO anonymus_runtime', current_database()) \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM anonymus_runtime;
GRANT USAGE ON SCHEMA public TO anonymus_runtime;

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM anonymus_runtime;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM anonymus_runtime;

GRANT SELECT, INSERT, UPDATE ON public.users, public.operators, public.sessions,
    public.messages, public.message_outbox, public.incoming_requests, public.session_invites,
    public.operator_telegram_links TO anonymus_runtime;
GRANT SELECT, INSERT ON public.audit_log, public.operator_recovery_codes TO anonymus_runtime;
GRANT UPDATE (payload) ON public.audit_log TO anonymus_runtime;
GRANT DELETE ON public.messages, public.incoming_requests, public.session_invites,
    public.operator_telegram_links, public.operator_recovery_codes TO anonymus_runtime;
GRANT USAGE ON SEQUENCE public.users_id_seq, public.operators_id_seq, public.sessions_id_seq,
    public.messages_id_seq, public.incoming_requests_id_seq, public.audit_log_id_seq TO anonymus_runtime;
