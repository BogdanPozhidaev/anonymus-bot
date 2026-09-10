-- users
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT NOT NULL UNIQUE,
    username TEXT,
    first_name TEXT,
    language_code TEXT NOT NULL DEFAULT 'ru',
    active_session_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_telegram_id ON users(telegram_id);

-- operators
CREATE TABLE operators (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    login TEXT NOT NULL UNIQUE,
    email TEXT UNIQUE,
    telegram_id BIGINT UNIQUE,
    role TEXT NOT NULL CHECK (role IN ('admin', 'operator')),
    status TEXT NOT NULL DEFAULT 'invited' CHECK (status IN ('invited', 'active', 'suspended', 'disabled')),
    password_hash TEXT,
    totp_secret TEXT,
    language TEXT NOT NULL DEFAULT 'ru',
    created_by BIGINT REFERENCES operators(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE INDEX idx_operators_telegram_id ON operators(telegram_id);
CREATE INDEX idx_operators_status ON operators(status);

-- sessions
CREATE TABLE sessions (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    user_visible_name TEXT,
    client_user_id BIGINT REFERENCES users(id),
    executor_user_id BIGINT REFERENCES users(id),
    client_display_name TEXT,
    executor_display_name TEXT,
    owner_operator_id BIGINT REFERENCES operators(id),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'pending_close', 'closed')),
    language TEXT NOT NULL DEFAULT 'ru',
    close_requested_by BIGINT REFERENCES users(id),
    close_requested_at TIMESTAMPTZ,
    close_reason TEXT,
    payment_status TEXT,
    created_by BIGINT REFERENCES operators(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_owner_operator_id ON sessions(owner_operator_id);
CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_sessions_client_user_id ON sessions(client_user_id);
CREATE INDEX idx_sessions_executor_user_id ON sessions(executor_user_id);

-- Добавляем FK от users к sessions теперь, когда sessions уже существует
ALTER TABLE users
    ADD CONSTRAINT fk_users_active_session
    FOREIGN KEY (active_session_id) REFERENCES sessions(id)
    ON DELETE SET NULL;

-- messages
CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT,
    sender_user_id BIGINT REFERENCES users(id),
    sender_role TEXT NOT NULL CHECK (sender_role IN ('client', 'executor', 'operator')),
    content_type TEXT NOT NULL CHECK (content_type IN ('text', 'photo', 'voice', 'system')),
    content TEXT,
    file_id TEXT,
    internal_file_path TEXT,
    sent_by_operator BOOLEAN NOT NULL DEFAULT false,
    impersonated_role TEXT CHECK (impersonated_role IN ('client', 'executor')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_session_id ON messages(session_id);
CREATE INDEX idx_messages_created_at ON messages(created_at);

-- incoming_requests
CREATE TABLE incoming_requests (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT NOT NULL,
    username TEXT,
    first_name TEXT,
    first_message_text TEXT,
    language_code TEXT,
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'processed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_incoming_requests_status ON incoming_requests(status);
CREATE INDEX idx_incoming_requests_telegram_id ON incoming_requests(telegram_id);

-- audit_log
CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id BIGINT,
    actor_role TEXT,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id BIGINT,
    payload JSONB,
    ip_address TEXT,
    user_agent TEXT
);

CREATE INDEX idx_audit_log_actor_id ON audit_log(actor_id);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_log_target ON audit_log(target_type, target_id);