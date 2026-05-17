ALTER TABLE auth_magic_links ADD COLUMN token_hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_magic_links_token_hash ON auth_magic_links(token_hash) WHERE token_hash <> '';

ALTER TABLE sessions ADD COLUMN token_hash TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash) WHERE token_hash <> '';
