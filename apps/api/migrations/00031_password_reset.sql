-- +goose Up
CREATE TABLE platform.password_reset_tokens(
 id uuid PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES platform.users(id) ON DELETE CASCADE,
 token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 expires_at timestamptz NOT NULL,
 consumed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX password_reset_user ON platform.password_reset_tokens(user_id,expires_at);
GRANT SELECT,INSERT,UPDATE ON platform.password_reset_tokens TO aba_app;
-- Keep session-table privileges narrow. The definer function requires a valid
-- reset token and updates token, password and sessions atomically.
-- +goose StatementBegin
CREATE FUNCTION platform.complete_password_reset(p_token_hash bytea,p_password_hash text) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE uid uuid;
BEGIN
 UPDATE platform.password_reset_tokens SET consumed_at=now()
  WHERE token_hash=p_token_hash AND consumed_at IS NULL AND expires_at>now()
  RETURNING user_id INTO uid;
 IF uid IS NULL THEN RETURN false; END IF;
 UPDATE platform.users SET password_hash=p_password_hash WHERE id=uid;
 UPDATE platform.sessions SET revoked_at=now() WHERE user_id=uid AND revoked_at IS NULL;
 RETURN true;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION platform.complete_password_reset(bytea,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.complete_password_reset(bytea,text) TO aba_app;
UPDATE platform.service_metadata SET version=31 WHERE singleton;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.password_reset_tokens WHERE consumed_at IS NULL AND expires_at>now()) THEN
  RAISE EXCEPTION 'rollback refused: active password reset tokens exist';
 END IF;
END $$;
-- +goose StatementEnd
UPDATE platform.service_metadata SET version=30 WHERE singleton;
REVOKE ALL ON FUNCTION platform.complete_password_reset(bytea,text) FROM PUBLIC,aba_app;
DROP FUNCTION platform.complete_password_reset(bytea,text);
DROP TABLE platform.password_reset_tokens;
