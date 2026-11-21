-- +goose Up
CREATE TABLE platform.product_categories(id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 120),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,id));
CREATE UNIQUE INDEX categories_name ON platform.product_categories(organization_id,lower(name));
CREATE TABLE platform.products(id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),category_id uuid,name text NOT NULL CHECK(length(btrim(name)) BETWEEN 1 AND 200),description text NOT NULL DEFAULT '' CHECK(length(description)<=10000),brand text NOT NULL DEFAULT '' CHECK(length(brand)<=100),status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','active','archived')),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,id),FOREIGN KEY(organization_id,category_id) REFERENCES platform.product_categories(organization_id,id) ON DELETE RESTRICT);
CREATE INDEX products_listing ON platform.products(organization_id,status,created_at,id);
CREATE INDEX products_category ON platform.products(organization_id,category_id);
CREATE INDEX products_search ON platform.products USING gin(to_tsvector('simple',name||' '||brand));
CREATE TABLE platform.product_variants(id uuid PRIMARY KEY,organization_id uuid NOT NULL REFERENCES platform.organizations(id),product_id uuid NOT NULL,sku text NOT NULL CHECK(sku ~ '^[A-Z0-9][A-Z0-9._-]{0,63}$'),variant_name text NOT NULL CHECK(length(btrim(variant_name)) BETWEEN 1 AND 120),attributes jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(attributes)='object' AND octet_length(attributes::text)<=4096),price_amount bigint NOT NULL CHECK(price_amount BETWEEN 0 AND 9000000000000),currency_code text NOT NULL DEFAULT 'RUB' CHECK(currency_code='RUB'),version bigint NOT NULL DEFAULT 1 CHECK(version>0),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,id),UNIQUE(organization_id,sku),FOREIGN KEY(organization_id,product_id) REFERENCES platform.products(organization_id,id) ON DELETE RESTRICT);
CREATE INDEX variants_product ON platform.product_variants(organization_id,product_id);
CREATE TABLE platform.inventory(organization_id uuid NOT NULL,variant_id uuid NOT NULL,quantity_on_hand integer NOT NULL DEFAULT 0 CHECK(quantity_on_hand BETWEEN 0 AND 1000000000),quantity_reserved integer NOT NULL DEFAULT 0 CHECK(quantity_reserved>=0 AND quantity_reserved<=quantity_on_hand),version bigint NOT NULL DEFAULT 1 CHECK(version>0),updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(organization_id,variant_id),FOREIGN KEY(organization_id,variant_id) REFERENCES platform.product_variants(organization_id,id) ON DELETE RESTRICT);
CREATE TABLE platform.inventory_movements(id uuid PRIMARY KEY,organization_id uuid NOT NULL,variant_id uuid NOT NULL,actor_id uuid NOT NULL REFERENCES platform.users(id) ON DELETE RESTRICT,reason text NOT NULL CHECK(length(btrim(reason)) BETWEEN 1 AND 500),quantity_before integer NOT NULL CHECK(quantity_before>=0),quantity_after integer NOT NULL CHECK(quantity_after>=0),delta integer NOT NULL CHECK(delta=quantity_after-quantity_before),inventory_version bigint NOT NULL CHECK(inventory_version>1),created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(organization_id,variant_id,inventory_version),FOREIGN KEY(organization_id,variant_id) REFERENCES platform.product_variants(organization_id,id) ON DELETE RESTRICT);
CREATE INDEX movements_listing ON platform.inventory_movements(organization_id,created_at DESC,id);
ALTER TABLE platform.product_categories ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.product_categories FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.product_categories TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY tenant_read ON platform.product_categories FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,false));
GRANT SELECT,INSERT ON platform.product_categories TO aba_app;
CREATE POLICY tenant_insert ON platform.product_categories FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
GRANT UPDATE ON platform.product_categories TO aba_app;
CREATE POLICY tenant_update ON platform.product_categories FOR UPDATE TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
ALTER TABLE platform.products ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.products FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.products TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY tenant_read ON platform.products FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,false));
GRANT SELECT,INSERT ON platform.products TO aba_app;
CREATE POLICY tenant_insert ON platform.products FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
GRANT UPDATE ON platform.products TO aba_app;
CREATE POLICY tenant_update ON platform.products FOR UPDATE TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
ALTER TABLE platform.product_variants ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.product_variants FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.product_variants TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY tenant_read ON platform.product_variants FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,false));
GRANT SELECT,INSERT ON platform.product_variants TO aba_app;
CREATE POLICY tenant_insert ON platform.product_variants FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
GRANT UPDATE ON platform.product_variants TO aba_app;
CREATE POLICY tenant_update ON platform.product_variants FOR UPDATE TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
ALTER TABLE platform.inventory ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.inventory FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.inventory TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY tenant_read ON platform.inventory FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,false));
GRANT SELECT,INSERT ON platform.inventory TO aba_app;
CREATE POLICY tenant_insert ON platform.inventory FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
GRANT UPDATE ON platform.inventory TO aba_app;
CREATE POLICY tenant_update ON platform.inventory FOR UPDATE TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true)) WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true));
ALTER TABLE platform.inventory_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.inventory_movements FORCE ROW LEVEL SECURITY;
CREATE POLICY migration_owner ON platform.inventory_movements TO aba_migrator USING(true) WITH CHECK(true);
CREATE POLICY tenant_read ON platform.inventory_movements FOR SELECT TO aba_app USING(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,false));
GRANT SELECT,INSERT ON platform.inventory_movements TO aba_app;
CREATE POLICY tenant_insert ON platform.inventory_movements FOR INSERT TO aba_app WITH CHECK(organization_id::text=current_setting('app.organization_id',true) AND platform.has_membership(organization_id,true) AND actor_id=platform.current_user_id());
UPDATE platform.service_metadata SET version=3 WHERE singleton;
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.products) OR EXISTS(SELECT 1 FROM platform.product_categories) THEN RAISE EXCEPTION 'refusing rollback of nonempty catalog'; END IF;
END $$;
-- +goose StatementEnd
DROP TABLE platform.inventory_movements,platform.inventory,platform.product_variants,platform.products,platform.product_categories;
UPDATE platform.service_metadata SET version=2 WHERE singleton;
