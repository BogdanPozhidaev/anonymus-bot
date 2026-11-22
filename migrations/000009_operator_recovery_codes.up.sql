CREATE TABLE operator_recovery_codes (
    operator_id bigint NOT NULL REFERENCES operators(id) ON DELETE CASCADE,
    digest bytea NOT NULL CHECK (octet_length(digest) = 32),
    PRIMARY KEY (operator_id, digest)
);
