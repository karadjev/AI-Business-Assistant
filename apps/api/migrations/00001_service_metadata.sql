-- +goose Up
CREATE TABLE platform.service_metadata (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 version integer NOT NULL CHECK (version > 0),
 created_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO platform.service_metadata (singleton, version) VALUES (true, 1);
GRANT SELECT ON platform.service_metadata TO aba_app;

-- +goose Down
DROP TABLE platform.service_metadata;
