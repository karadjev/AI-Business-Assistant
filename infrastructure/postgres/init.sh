#!/bin/sh
set -eu
: "${APP_DB_PASSWORD:?APP_DB_PASSWORD is required}"
: "${MIGRATOR_DB_PASSWORD:?MIGRATOR_DB_PASSWORD is required}"
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=ON_ERROR_STOP=1 --set=app_password="$APP_DB_PASSWORD" --set=migrator_password="$MIGRATOR_DB_PASSWORD" --set=db_name="$POSTGRES_DB" <<'SQL'
CREATE ROLE aba_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD :'migrator_password';
CREATE ROLE aba_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD :'app_password';
REVOKE ALL ON DATABASE :"db_name" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"db_name" TO aba_migrator, aba_app;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA platform AUTHORIZATION aba_migrator;
GRANT USAGE ON SCHEMA platform TO aba_app;
ALTER ROLE aba_migrator IN DATABASE :"db_name" SET search_path=platform,pg_catalog;
ALTER ROLE aba_app IN DATABASE :"db_name" SET search_path=platform,pg_catalog;
SQL
# Tests require an explicit marker, in addition to the isolated database name.
if [ "$POSTGRES_DB" = aba_test ]; then
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE platform.test_safety(singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),marker text NOT NULL);
INSERT INTO platform.test_safety(marker) VALUES ('aba-synthetic-isolated');
GRANT SELECT ON platform.test_safety TO aba_migrator,aba_app;
SQL
fi
