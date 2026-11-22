CREATE TABLE operator_telegram_links (
    operator_id bigint PRIMARY KEY REFERENCES operators(id) ON DELETE CASCADE,
    auth_version bigint NOT NULL,
    code_digest bytea NOT NULL UNIQUE CHECK (octet_length(code_digest) = 32),
    expires_at timestamptz NOT NULL,
    candidate_telegram_id bigint CHECK (candidate_telegram_id > 0),
    confirmation_digest bytea CHECK (octet_length(confirmation_digest) = 32),
    CHECK ((candidate_telegram_id IS NULL) = (confirmation_digest IS NULL))
);
