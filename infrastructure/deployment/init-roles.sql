-- Official PostgreSQL .sql bootstrap: no executable host script is needed.
-- psql reads private files into variables; values are never command arguments.
\set ON_ERROR_STOP on
\set app_password `cat /run/secrets/app_password`
\set migrator_password `cat /run/secrets/migrator_password`
\getenv db_name POSTGRES_DB
SELECT :'app_password' ~ '^[0-9a-f]{48}$' AND :'migrator_password' ~ '^[0-9a-f]{48}$' AND :'app_password' <> :'migrator_password' AS credentials_valid \gset
\if :credentials_valid
CREATE ROLE aba_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD :'migrator_password';
CREATE ROLE aba_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD :'app_password';
REVOKE ALL ON DATABASE :"db_name" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"db_name" TO aba_migrator, aba_app;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA platform AUTHORIZATION aba_migrator;
GRANT USAGE ON SCHEMA platform TO aba_app;
ALTER ROLE aba_migrator IN DATABASE :"db_name" SET search_path=platform,pg_catalog;
ALTER ROLE aba_app IN DATABASE :"db_name" SET search_path=platform,pg_catalog;
\else
\quit 1
\endif
