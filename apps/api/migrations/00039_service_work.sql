-- +goose Up
CREATE TABLE platform.service_staff_resources (
 organization_id uuid NOT NULL,
 resource_id uuid NOT NULL,
 user_id uuid NOT NULL REFERENCES platform.users(id),
 bound_by uuid NOT NULL REFERENCES platform.users(id),
 bound_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(organization_id,resource_id),
 UNIQUE(organization_id,user_id),
 UNIQUE(organization_id,resource_id,user_id),
 FOREIGN KEY(organization_id,resource_id) REFERENCES platform.service_resources(organization_id,id)
);
CREATE TABLE platform.service_assignments (
 organization_id uuid NOT NULL,
 order_id uuid NOT NULL,
 booking_id uuid NOT NULL,
 resource_id uuid NOT NULL,
 user_id uuid NOT NULL,
 assigned_by uuid NOT NULL REFERENCES platform.users(id),
 assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(organization_id,order_id),
 FOREIGN KEY(organization_id,order_id) REFERENCES platform.orders(organization_id,id),
 FOREIGN KEY(organization_id,booking_id,resource_id) REFERENCES platform.service_booking_allocations(organization_id,booking_id,resource_id),
 FOREIGN KEY(organization_id,resource_id,user_id) REFERENCES platform.service_staff_resources(organization_id,resource_id,user_id)
);
CREATE INDEX service_assignments_schedule ON platform.service_assignments(organization_id,user_id);
CREATE TABLE platform.service_receipts (
 organization_id uuid NOT NULL,
 order_id uuid NOT NULL,
 received_minor bigint NOT NULL CHECK(received_minor BETWEEN 1 AND 1000000000000),
 cost_minor bigint NOT NULL CHECK(cost_minor BETWEEN 0 AND 1000000000000),
 recorded_by uuid NOT NULL REFERENCES platform.users(id),
 recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(organization_id,order_id),
 FOREIGN KEY(organization_id,order_id) REFERENCES platform.service_assignments(organization_id,order_id)
);

-- +goose StatementBegin
DO $$
DECLARE name text;
BEGIN
 FOREACH name IN ARRAY ARRAY['service_staff_resources','service_assignments','service_receipts'] LOOP
  EXECUTE format('ALTER TABLE platform.%I ENABLE ROW LEVEL SECURITY',name);
  EXECUTE format('ALTER TABLE platform.%I FORCE ROW LEVEL SECURITY',name);
  EXECUTE format('CREATE POLICY migration_owner ON platform.%I TO aba_migrator USING(true) WITH CHECK(true)',name);
  EXECUTE format('CREATE POLICY owner_insert ON platform.%I FOR INSERT TO aba_app WITH CHECK (organization_id::text=current_setting(''app.organization_id'',true) AND platform.is_test_org(organization_id) AND platform.has_membership(organization_id,true))',name);
 END LOOP;
END $$;
-- +goose StatementEnd
CREATE POLICY owner_read ON platform.service_staff_resources FOR SELECT TO aba_app USING (organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.has_membership(organization_id,true));
CREATE POLICY operator_read ON platform.service_assignments FOR SELECT TO aba_app USING (organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.can_operate_orders(organization_id));
CREATE POLICY owner_read ON platform.service_receipts FOR SELECT TO aba_app USING (organization_id::text=current_setting('app.organization_id',true) AND platform.is_test_org(organization_id) AND platform.has_membership(organization_id,true));
GRANT SELECT,INSERT ON platform.service_staff_resources,platform.service_assignments,platform.service_receipts TO aba_app;
-- +goose StatementBegin
CREATE FUNCTION platform.service_work_totals(p_org uuid) RETURNS jsonb
 LANGUAGE sql STABLE SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_build_object(
  'completed_count',(SELECT count(*) FROM platform.orders WHERE organization_id=p_org AND request_kind='service_booking' AND status='completed'),
  'paid_count',count(*),'received_minor',sum(received_minor),'cost_minor',sum(cost_minor),'difference_minor',sum(received_minor-cost_minor))
 FROM platform.service_receipts WHERE organization_id=p_org
$$;

CREATE FUNCTION platform.service_agent_totals(p_task uuid,p_lease uuid) RETURNS jsonb
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT platform.service_work_totals(t.organization_id)
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.organizations o ON o.id=t.organization_id
 WHERE t.agent_id IN ('finance','analytics') AND t.action='read' AND o.is_test
  AND o.business_model IN ('service','hybrid')
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION platform.service_work_totals(uuid),platform.service_agent_totals(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.service_work_totals(uuid),platform.service_agent_totals(uuid,uuid) TO aba_app;
UPDATE platform.service_metadata SET version=39 WHERE singleton;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM platform.service_assignments) OR EXISTS (SELECT 1 FROM platform.service_staff_resources)
 OR EXISTS (SELECT 1 FROM platform.service_receipts) THEN
  RAISE EXCEPTION 'rollback refused: preserve service assignments and confirmed payments';
 END IF;
END $$;
-- +goose StatementEnd
DROP FUNCTION IF EXISTS platform.service_agent_totals(uuid,uuid),platform.service_work_totals(uuid);
DROP TABLE platform.service_receipts,platform.service_assignments,platform.service_staff_resources;
UPDATE platform.service_metadata SET version=38 WHERE singleton;
