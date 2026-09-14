ALTER TABLE sessions
    ADD COLUMN closed_at TIMESTAMPTZ;

CREATE INDEX idx_sessions_closed_at ON sessions(closed_at) WHERE closed_at IS NOT NULL;