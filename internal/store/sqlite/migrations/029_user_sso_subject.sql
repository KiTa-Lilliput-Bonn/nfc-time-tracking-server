-- +goose Up
-- SSO (OIDC): feste Kennung ("sub") des Kontos beim Identity Provider. Wird beim ersten
-- SSO-Login über den Benutzernamen verknüpft; NULL = nicht verknüpft.
ALTER TABLE users ADD COLUMN sso_subject TEXT;
CREATE UNIQUE INDEX idx_users_sso_subject ON users(sso_subject) WHERE sso_subject IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_sso_subject;
ALTER TABLE users DROP COLUMN sso_subject;
