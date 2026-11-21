-- +goose Up
-- +goose StatementBegin
ALTER TABLE platform.property_listings
 DROP CONSTRAINT property_listings_property_type_check,
 DROP CONSTRAINT property_price_currency,
 ADD CONSTRAINT property_listings_property_type_check CHECK(property_type IN ('apartment','house','commercial','land','new_development','other')),
 ADD CONSTRAINT property_price_currency CHECK((price_amount IS NULL AND currency_code IS NULL)
   OR (price_amount IS NOT NULL AND price_amount>=0 AND currency_code IS NOT NULL AND currency_code ~ '^[A-Z]{3}$')),
 ADD COLUMN transaction_type text CHECK(transaction_type IN ('sale','long_term_rental','short_term_rental')),
 ADD COLUMN description text CHECK(length(description)<=4000),
 ADD COLUMN country text CHECK(length(country)<=120),
 ADD COLUMN region text CHECK(length(region)<=120),
 ADD COLUMN city text CHECK(length(city)<=120),
 ADD COLUMN district text CHECK(length(district)<=120),
 ADD COLUMN address text CHECK(length(address)<=240),
 ADD COLUMN living_area_m2 numeric(10,2) CHECK(living_area_m2>0 AND living_area_m2<=area_m2),
 ADD COLUMN land_area_m2 numeric(10,2) CHECK(land_area_m2>0),
 ADD COLUMN bedrooms integer CHECK(bedrooms BETWEEN 0 AND 50),
 ADD COLUMN bathrooms integer CHECK(bathrooms BETWEEN 0 AND 50),
 ADD COLUMN floor integer CHECK(floor BETWEEN -20 AND 300),
 ADD COLUMN total_floors integer CHECK(total_floors BETWEEN 1 AND 300),
 ADD COLUMN building_type text CHECK(length(building_type)<=120),
 ADD COLUMN year_built integer CHECK(year_built BETWEEN 1000 AND 2200),
 ADD COLUMN renovation_status text CHECK(renovation_status IN ('shell','needs_renovation','renovated')),
 ADD COLUMN furnishing_status text CHECK(furnishing_status IN ('unfurnished','partial','furnished')),
 ADD COLUMN has_elevator boolean,
 ADD COLUMN has_parking boolean,
 ADD COLUMN property_status text NOT NULL DEFAULT 'active' CHECK(property_status IN ('draft','active','reserved','sold','rented','unavailable','archived')),
 ADD COLUMN published_at timestamptz,
 ADD COLUMN updated_at timestamptz,
 ADD CONSTRAINT property_floor_range CHECK(floor<=total_floors),
 ADD CONSTRAINT property_available_active CHECK(status<>'available' OR property_status='active');
CREATE INDEX property_listings_market_search ON platform.property_listings
 (organization_id,lower(city),property_type,transaction_type,currency_code,price_amount,id)
 WHERE property_status='active' AND status='available';

CREATE FUNCTION platform.property_candidates(p_org uuid,p_filters jsonb,p_status text)
RETURNS SETOF platform.property_listings LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT p.* FROM platform.property_listings p
 WHERE p.organization_id=p_org AND p.property_status=coalesce(nullif(p_status,''),'active')
 AND (nullif(p_filters->>'city','') IS NULL OR lower(p.city)=lower(p_filters->>'city'))
 AND (nullif(p_filters->>'district','') IS NULL OR lower(p.district)=lower(p_filters->>'district'))
 AND (nullif(p_filters->>'property_type','') IS NULL OR p.property_type=p_filters->>'property_type')
 AND (nullif(p_filters->>'transaction_type','') IS NULL OR p.transaction_type=p_filters->>'transaction_type')
 AND (nullif(p_filters->>'currency_code','') IS NULL OR p.currency_code=p_filters->>'currency_code')
 AND (p_filters->>'min_price_amount' IS NULL OR p.price_amount>=(p_filters->>'min_price_amount')::bigint)
 AND (p_filters->>'max_price_amount' IS NULL OR p.price_amount<=(p_filters->>'max_price_amount')::bigint)
 AND (p_filters->>'min_area_m2' IS NULL OR p.area_m2>=(p_filters->>'min_area_m2')::numeric)
 AND (p_filters->>'max_area_m2' IS NULL OR p.area_m2<=(p_filters->>'max_area_m2')::numeric)
 AND (p_filters->>'rooms' IS NULL OR p.rooms=(p_filters->>'rooms')::int)
 AND (p_filters->>'bedrooms' IS NULL OR p.bedrooms=(p_filters->>'bedrooms')::int)
 AND (nullif(p_filters->>'building_type','') IS NULL OR lower(p.building_type)=lower(p_filters->>'building_type'))
 AND (nullif(p_filters->>'renovation_status','') IS NULL OR p.renovation_status=p_filters->>'renovation_status')
 AND (p_filters->>'floor' IS NULL OR p.floor=(p_filters->>'floor')::int)
$$;
REVOKE ALL ON FUNCTION platform.property_candidates(uuid,jsonb,text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.property_candidates(uuid,jsonb,text) TO aba_app;

CREATE FUNCTION platform.property_availability(p_status text,p_market_status text,p_until timestamptz)
RETURNS text LANGUAGE sql STABLE SET search_path=pg_catalog AS $$
 SELECT CASE WHEN p_market_status<>'active' THEN 'unavailable'
 WHEN p_status='available' AND p_until<=statement_timestamp() THEN 'unknown' ELSE p_status END
$$;
REVOKE ALL ON FUNCTION platform.property_availability(text,text,timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.property_availability(text,text,timestamptz) TO aba_app;

CREATE FUNCTION platform.property_json(p platform.property_listings,p_full boolean)
RETURNS jsonb LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT jsonb_strip_nulls((CASE WHEN p_full THEN to_jsonb(p) ELSE to_jsonb(p)-'description' END)
 || jsonb_build_object('area_m2',p.area_m2::text,'living_area_m2',p.living_area_m2::text,
 'land_area_m2',p.land_area_m2::text,'status',platform.property_availability(p.status,p.property_status,p.verified_until)))
$$;
REVOKE ALL ON FUNCTION platform.property_json(platform.property_listings,boolean) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.property_json(platform.property_listings,boolean) TO aba_app;

CREATE FUNCTION platform.property_search_page(p_org uuid,p_filters jsonb,p_page int)
RETURNS jsonb LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 WITH sorted AS (
 SELECT platform.property_json(p,true) AS item,
 row_number() OVER (ORDER BY
 CASE WHEN p_filters->>'sort'='price_asc' THEN price_amount END ASC NULLS LAST,
 CASE WHEN p_filters->>'sort'='price_desc' THEN price_amount END DESC NULLS LAST,
 CASE WHEN p_filters->>'sort'='area_desc' THEN area_m2 END DESC NULLS LAST,
 CASE WHEN p_filters->>'sort'='newest' THEN published_at END DESC NULLS LAST,
 area_m2,id) AS position
 FROM platform.property_candidates(p_org,p_filters,p_filters->>'property_status') p
 WHERE platform.property_availability(p.status,p.property_status,p.verified_until)
 =coalesce(nullif(p_filters->>'availability',''),CASE WHEN coalesce(p_filters->>'property_status','active')='active' THEN 'available' ELSE 'unavailable' END)
 ), page AS (SELECT * FROM sorted WHERE position>(p_page-1)*20 ORDER BY position LIMIT 21)
 SELECT jsonb_build_object('items',coalesce(jsonb_agg(item ORDER BY position) FILTER(WHERE position<=p_page*20),'[]'),
 'page',p_page,'has_more',count(*)=21) FROM page
$$;
REVOKE ALL ON FUNCTION platform.property_search_page(uuid,jsonb,int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.property_search_page(uuid,jsonb,int) TO aba_app;
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate','coordinate_growth'))))
 OR (variant_id IS NULL AND conversation_id IS NULL AND parent_id IS NOT NULL AND
 ((agent_id='sales' AND action='review_sales') OR (agent_id='product_inventory' AND action='review_inventory')))
 OR (agent_id='customer' AND action IN ('workflow_preview_B','workflow_preview_C','workflow_preview_D','workflow_preview_E')
     AND conversation_id IS NOT NULL AND variant_id IS NULL AND parent_id IS NULL
     AND status IN ('completed','failed') AND correlation_id=id)
 OR (action IN ('delegate_auto_parts','delegate_construction','delegate_service','delegate_real_estate','delegate_property_viewing') AND agent_id='business_manager' AND parent_id IS NULL
     AND correlation_id=id AND conversation_id IS NOT NULL AND variant_id IS NULL AND delegation_input IS NOT NULL AND delegation_step IS NULL)
 OR (action LIKE 'delegation.%' AND parent_id IS NOT NULL AND conversation_id IS NOT NULL
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL));
CREATE OR REPLACE FUNCTION platform.agent_delegation_profile(p_org uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.is_test AND CASE p_action
 WHEN 'delegate_auto_parts' THEN o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_construction' THEN o.industry_profile='building_materials' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_service' THEN (o.industry_profile='vehicle_service' AND o.business_model='service')
   OR (o.industry_profile='mixed_nonfood' AND o.business_model='hybrid')
 WHEN 'delegate_real_estate' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_property_viewing' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 ELSE false END)
$$;
REVOKE ALL ON FUNCTION platform.agent_delegation_profile(uuid,text) FROM PUBLIC;
CREATE OR REPLACE FUNCTION platform.agent_delegation_plan(p_action text)
RETURNS TABLE(step integer, agent text, operation text) LANGUAGE sql IMMUTABLE SET search_path=pg_catalog AS $$
 SELECT v.step,v.agent,v.operation FROM (VALUES
 ('delegate_auto_parts',1,'customer','delegation.requirements'),
 ('delegate_auto_parts',2,'sales','delegation.search_plan'),
 ('delegate_auto_parts',3,'product_inventory','delegation.find_parts'),
 ('delegate_auto_parts',4,'product_inventory','delegation.verify_fitment'),
 ('delegate_auto_parts',5,'sales','delegation.proposal'),
 ('delegate_construction',1,'customer','delegation.construction_input'),
 ('delegate_construction',2,'product_inventory','delegation.construction_catalog'),
 ('delegate_construction',3,'product_inventory','delegation.construction_calculate'),
 ('delegate_construction',4,'sales','delegation.construction_offer'),
 ('delegate_service',1,'customer','delegation.service_input'),
 ('delegate_service',2,'sales','delegation.service_select'),
 ('delegate_service',3,'sales','delegation.service_availability'),
 ('delegate_service',4,'product_inventory','delegation.service_parts'),
 ('delegate_service',5,'sales','delegation.service_proposal'),
 ('delegate_real_estate',1,'customer','delegation.property_requirements'),
 ('delegate_real_estate',2,'sales','delegation.property_search'),
 ('delegate_real_estate',3,'sales','delegation.property_offer'),
 ('delegate_property_viewing',1,'customer','delegation.property_selection'),
 ('delegate_property_viewing',2,'sales','delegation.property_viewing')
 ) v(workflow,step,agent,operation)

 WHERE workflow=p_action
$$;
CREATE FUNCTION platform.agent_property_selection(p_task uuid,p_lease uuid)
RETURNS boolean LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT EXISTS(
 SELECT 1 FROM platform.agent_lease_task(p_task,p_lease) t
 CROSS JOIN LATERAL (SELECT platform.agent_delegation_context(p_task,p_lease)->'input' AS input) c
 JOIN platform.agent_tasks source ON source.organization_id=t.organization_id AND source.conversation_id=t.conversation_id
   AND source.id=(c.input->>'offer_task_id')::uuid AND source.action='delegate_real_estate' AND source.status='completed'
   AND source.deadline>statement_timestamp()
 JOIN platform.agent_executions e ON e.organization_id=source.organization_id AND e.task_id=source.id AND e.status='completed'
 WHERE t.agent_id='customer' AND t.action='delegation.property_selection'
 AND e.result->0->>'state'='proposal_ready'
 AND EXISTS(SELECT 1 FROM jsonb_array_elements(e.result->0->'properties') item WHERE item->>'id'=c.input->>'property_id'))
$$;
REVOKE ALL ON FUNCTION platform.agent_property_selection(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_property_selection(uuid,uuid) TO aba_app;

CREATE FUNCTION platform.agent_delegation_properties(p_task uuid,p_lease uuid)
RETURNS jsonb LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 WITH bound AS MATERIALIZED (
 SELECT t.organization_id,t.action,parent.delegation_input,
 platform.agent_delegation_context(p_task,p_lease)->'children'->0->'result' AS prior
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks parent ON parent.organization_id=t.organization_id AND parent.id=t.parent_id
 WHERE t.agent_id='sales' AND ((parent.action='delegate_real_estate' AND t.action IN ('delegation.property_search','delegation.property_offer'))
 OR (parent.action='delegate_property_viewing' AND t.action='delegation.property_viewing'))
 ), candidates AS (
 SELECT p FROM bound b
 CROSS JOIN LATERAL platform.property_candidates(b.organization_id,b.prior->'property_requirements','active') p
 WHERE p.status='available' AND p.verified_until>statement_timestamp()
 AND ((b.action='delegation.property_search' AND b.prior->>'state'='requirements_collected' AND p.price_amount IS NOT NULL)
 OR (b.action='delegation.property_offer' AND b.prior->>'state'='properties_found' AND p.price_amount IS NOT NULL
     AND EXISTS(SELECT 1 FROM jsonb_array_elements(b.prior->'properties') a WHERE a->>'id'=p.id::text))
 OR (b.action='delegation.property_viewing' AND b.prior->>'state'='property_selected'
     AND p.id=(b.delegation_input->>'property_id')::uuid))
 ORDER BY p.price_amount,p.area_m2,p.id LIMIT 3
 )
 SELECT CASE WHEN EXISTS(SELECT 1 FROM bound WHERE prior IS NOT NULL) THEN
 coalesce((SELECT jsonb_agg(platform.property_json(p,false) ORDER BY (p).price_amount,(p).area_m2,(p).id) FROM candidates),'[]') END
$$;
REVOKE ALL ON FUNCTION platform.agent_delegation_properties(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.agent_delegation_properties(uuid,uuid) TO aba_app;
CREATE OR REPLACE FUNCTION platform.enqueue_service_receipt() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE source platform.agent_tasks; revision bigint; task_id uuid:=gen_random_uuid();
BEGIN
 SELECT t.* INTO source
 FROM platform.orders o JOIN platform.agent_tasks t
 ON t.organization_id=o.organization_id AND t.id=o.idempotency_key AND t.conversation_id=o.conversation_id
 WHERE o.organization_id=NEW.organization_id AND o.id=NEW.order_id AND (o.status=NEW.kind OR (o.request_kind='property_viewing_request' AND o.status='pending_confirmation' AND NEW.kind='created'))
 AND ((o.request_kind IN ('service_booking','hybrid_order') AND t.action='delegate_service')
 OR (o.request_kind='property_viewing_request' AND t.action='delegate_property_viewing')) AND t.status='completed'
 AND EXISTS(SELECT 1 FROM platform.agent_executions e WHERE e.organization_id=t.organization_id
  AND e.task_id=t.id AND e.status='completed' AND e.result->0->>'state'='approval_required'
  AND e.result->0->'proposal'->'details'=o.request_details AND e.result->0->'proposal'->>'kind'=o.request_kind);
 IF NOT FOUND THEN RETURN NEW; END IF;
 SELECT version INTO STRICT revision FROM platform.orders WHERE organization_id=NEW.organization_id AND id=NEW.order_id;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,delegation_step)
 VALUES(task_id,source.organization_id,source.initiator,'business_manager',source.id,source.correlation_id,
 source.conversation_id,CASE WHEN source.action='delegate_property_viewing' THEN 'delegation.property_receipt' ELSE 'delegation.service_receipt' END,NEW.id,source.worker_hash,'queued',clock_timestamp()+interval '24 hours',5+revision)
 ON CONFLICT DO NOTHING;
 IF NOT FOUND THEN RETURN NEW; END IF;
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(task_id,source.organization_id,task_id::text,'','{}','agent',task_id);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(source.organization_id,task_id,NEW.actor_id,'business_manager','task.delegate','queued');
 RETURN NEW;
END $$;
DROP TRIGGER enqueue_service_receipt ON platform.order_events;
CREATE TRIGGER enqueue_service_receipt AFTER INSERT ON platform.order_events FOR EACH ROW
 WHEN(NEW.kind IN ('created','confirmed','rejected','cancelled','completed'))
 EXECUTE FUNCTION platform.enqueue_service_receipt();


CREATE FUNCTION platform.property_receipt_context(p_task uuid,p_lease uuid) RETURNS jsonb
LANGUAGE sql STABLE SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
 SELECT jsonb_strip_nulls(jsonb_build_object(
  'order_id',o.id,'version',o.version,'booking_status',o.status,
  'event_status',CASE WHEN ev.kind='created' THEN 'pending_confirmation' ELSE ev.kind END,
  'property_id',p.id,'property_title',p.title,
  'property_available',p.status='available' AND p.property_status='active' AND p.verified_until>statement_timestamp(),
  'starts_at',o.request_details->'selection'->'starts_at','ends_at',o.request_details->'selection'->'ends_at',
  'timezone',o.request_details->'selection'->>'timezone'))
 FROM platform.agent_lease_task(p_task,p_lease) t
 JOIN platform.agent_tasks parent ON (parent.organization_id,parent.id,parent.conversation_id)=(t.organization_id,t.parent_id,t.conversation_id)
 JOIN platform.order_events ev ON (ev.organization_id,ev.id)=(t.organization_id,t.idempotency_key)
 JOIN platform.orders o ON (o.organization_id,o.id,o.conversation_id,o.idempotency_key)=(ev.organization_id,ev.order_id,t.conversation_id,parent.id)
 JOIN platform.property_listings p ON (p.organization_id,p.id)=(o.organization_id,o.listing_id)
 JOIN platform.conversations c ON (c.organization_id,c.id,c.customer_id)=(o.organization_id,o.conversation_id,o.customer_id)
 WHERE t.agent_id='business_manager' AND t.action='delegation.property_receipt'
 AND parent.action='delegate_property_viewing' AND o.request_kind='property_viewing_request'
$$;
REVOKE ALL ON FUNCTION platform.property_receipt_context(uuid,uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.property_receipt_context(uuid,uuid) TO aba_app;

CREATE FUNCTION platform.real_estate_notification_current(p_task uuid,p_org uuid,p_conv uuid)
RETURNS boolean LANGUAGE sql VOLATILE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS (
  SELECT 1 FROM platform.agent_tasks t
  JOIN platform.agent_executions e ON (e.organization_id,e.task_id)=(t.organization_id,t.id) AND e.status='completed'
  JOIN platform.conversations c ON (c.organization_id,c.id)=(t.organization_id,t.conversation_id)
  JOIN platform.organizations z ON z.id=t.organization_id AND z.is_test
  CROSS JOIN LATERAL (SELECT e.result->0 AS body) r
  WHERE (t.id,t.organization_id,t.conversation_id)=(p_task,p_org,p_conv) AND t.agent_id='business_manager'
  AND c.status='open' AND c.control_mode='agent' AND c.expires_at>clock_timestamp()
  AND platform.booking_notification_consent(p_org,p_conv)
  AND (
   (t.action='delegate_real_estate' AND r.body->>'state' IN ('clarification_required','no_properties','proposal_ready')
    AND (r.body->>'state'<>'proposal_ready' OR NOT EXISTS(
      SELECT 1 FROM jsonb_array_elements(r.body->'properties') item
      WHERE NOT EXISTS(SELECT 1 FROM platform.property_listings p WHERE p.organization_id=p_org AND p.id=(item->>'id')::uuid
       AND p.status='available' AND p.property_status='active' AND p.verified_until>clock_timestamp()+interval '6 seconds'
       AND (p.title,p.locality,coalesce(p.city,''),coalesce(p.district,''),p.price_amount,p.currency_code,p.area_m2,p.rooms)
       =(item->>'title',item->>'locality',coalesce(item->>'city',''),coalesce(item->>'district',''),
         (item->>'price_amount')::bigint,item->>'currency_code',(item->>'area_m2')::numeric,(item->>'rooms')::int)))))
   OR (t.action='delegate_property_viewing' AND r.body->>'state'='property_unavailable')
   OR (t.action='delegation.property_receipt' AND r.body->>'state'='backend_result'
    AND EXISTS(SELECT 1 FROM platform.order_events ev
     JOIN platform.orders o ON (o.organization_id,o.id)=(ev.organization_id,ev.order_id)
     JOIN platform.property_listings p ON (p.organization_id,p.id)=(o.organization_id,o.listing_id)
     WHERE (ev.organization_id,ev.id)=(p_org,t.idempotency_key)
     AND (o.conversation_id,o.customer_id,o.idempotency_key)=(p_conv,c.customer_id,t.parent_id)
     AND o.request_kind='property_viewing_request'
     AND o.id::text=r.body->'receipt'->>'order_id' AND o.version::text=r.body->'receipt'->>'version'
     AND o.status=r.body->'receipt'->>'booking_status'
     AND ((ev.kind='created' AND o.status='pending_confirmation') OR ev.kind=o.status)
     AND (o.status NOT IN ('confirmed','pending_confirmation') OR
      ((r.body->'receipt'->>'property_available')::boolean =
       (p.status='available' AND p.property_status='active' AND p.verified_until>clock_timestamp()+interval '6 seconds')
       AND (o.status<>'confirmed' OR EXISTS(SELECT 1 FROM platform.service_bookings b
         WHERE (b.organization_id,b.request_id)=(o.organization_id,o.id) AND b.status='confirmed')))))))
  )
$$;
REVOKE ALL ON FUNCTION platform.real_estate_notification_current(uuid,uuid,uuid) FROM PUBLIC;

CREATE FUNCTION platform.enqueue_real_estate_notification() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE t platform.agent_tasks; c platform.conversations; outcome text:='channel_unavailable'; body text:=NEW.result->0->>'message';
BEGIN
 SELECT * INTO t FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.task_id
 AND agent_id='business_manager' AND action IN ('delegate_real_estate','delegate_property_viewing','delegation.property_receipt');
 IF NOT FOUND OR body IS NULL THEN RETURN NEW; END IF;
 IF t.action='delegate_property_viewing' AND NEW.result->0->>'state'<>'property_unavailable' THEN RETURN NEW; END IF;
 SELECT * INTO c FROM platform.conversations WHERE organization_id=t.organization_id AND id=t.conversation_id;
 IF c.channel='instagram' THEN
  outcome:='instagram_send_disabled';
 ELSIF NOT platform.booking_notification_consent(c.organization_id,c.id) AND c.channel<>'internal_chat' THEN
  outcome:='consent_required';
 ELSIF length(body) BETWEEN 1 AND 2000 AND platform.real_estate_notification_current(t.id,t.organization_id,t.conversation_id)
 AND ((c.channel='telegram' AND NOT EXISTS(SELECT 1 FROM platform.channel_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND 1=(SELECT count(*) FROM platform.telegram_principals p WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.telegram_principals p JOIN platform.telegram_integrations i ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
          WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND i.active AND i.mode='mock'))
   OR (c.channel='whatsapp' AND NOT EXISTS(SELECT 1 FROM platform.telegram_principals WHERE organization_id=c.organization_id AND conversation_id=c.id)
      AND EXISTS(SELECT 1 FROM platform.channel_principals p JOIN platform.channel_integrations i ON (i.organization_id,i.id)=(p.organization_id,p.integration_id)
          WHERE (p.organization_id,p.conversation_id,p.id)=(c.organization_id,c.id,c.customer_id) AND p.channel='whatsapp' AND i.channel='whatsapp'
          AND i.active AND i.mode='mock' AND i.sending_enabled AND p.window_until>clock_timestamp()))) THEN
  INSERT INTO platform.messages(id,organization_id,conversation_id,role,content,data,client_key)
   VALUES(t.id,t.organization_id,t.conversation_id,'assistant',body,jsonb_build_object('real_estate_notification',true),t.id)
   ON CONFLICT DO NOTHING;
  IF EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE id=t.id AND organization_id=t.organization_id)
   OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE id=t.id AND organization_id=t.organization_id) THEN outcome:='queued'; END IF;
 END IF;
 INSERT INTO platform.agent_audit(organization_id,task_id,execution_id,agent_id,action,result)
 VALUES(t.organization_id,t.id,NEW.id,t.agent_id,'property.delivery',outcome);
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.enqueue_real_estate_notification() FROM PUBLIC;
CREATE TRIGGER enqueue_real_estate_notification AFTER UPDATE OF status ON platform.agent_executions FOR EACH ROW
 WHEN(OLD.status='running' AND NEW.status='completed')
 EXECUTE FUNCTION platform.enqueue_real_estate_notification();

-- Viewing transitions and offline sends for that tenant use the same lock.
CREATE FUNCTION platform.real_estate_notification_transition() RETURNS trigger
LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 PERFORM pg_advisory_xact_lock(hashtextextended('real-estate-notification:'||OLD.organization_id::text,0));
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.real_estate_notification_transition() FROM PUBLIC;
CREATE TRIGGER viewing_notification_transition BEFORE UPDATE OF status,version ON platform.orders
 FOR EACH ROW WHEN(OLD.request_kind='property_viewing_request') EXECUTE FUNCTION platform.real_estate_notification_transition();

CREATE FUNCTION platform.audit_real_estate_notification_state() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
BEGIN
 INSERT INTO platform.agent_audit(organization_id,task_id,agent_id,action,result)
 SELECT organization_id,id,agent_id,'property.delivery_state',NEW.status
 FROM platform.agent_tasks WHERE organization_id=NEW.organization_id AND id=NEW.id
 AND action IN ('delegate_real_estate','delegate_property_viewing','delegation.property_receipt') AND agent_id='business_manager';
 RETURN NEW;
END $$;
REVOKE ALL ON FUNCTION platform.audit_real_estate_notification_state() FROM PUBLIC;
CREATE TRIGGER telegram_real_estate_delivery AFTER UPDATE OF status ON platform.telegram_outbox FOR EACH ROW
 WHEN(NEW.status IS DISTINCT FROM OLD.status) EXECUTE FUNCTION platform.audit_real_estate_notification_state();
CREATE TRIGGER channel_real_estate_delivery AFTER UPDATE OF status ON platform.channel_outbox FOR EACH ROW
 WHEN(NEW.status IS DISTINCT FROM OLD.status) EXECUTE FUNCTION platform.audit_real_estate_notification_state();
CREATE OR REPLACE FUNCTION platform.booking_notification_admit(p_cap uuid,p_channel text)
RETURNS TABLE(allowed boolean,lock_key bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE item record; key bigint; marker jsonb; property_marker boolean;
BEGIN
 IF p_channel='telegram' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.lease_until,q.booking_order_id,q.integration_id,q.chat_id::text AS recipient,i.mode INTO item
  FROM platform.telegram_outbox q JOIN platform.telegram_integrations i ON i.id=q.integration_id AND i.organization_id=q.organization_id
  WHERE q.capability=p_cap AND q.status='sending';
 ELSIF p_channel='whatsapp' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.lease_until,q.booking_order_id,q.integration_id,q.external_user_id AS recipient,i.mode INTO item
  FROM platform.channel_outbox q JOIN platform.channel_integrations i ON i.id=q.integration_id AND i.organization_id=q.organization_id
  WHERE q.capability=p_cap AND q.status='sending' AND i.channel='whatsapp';
 ELSE RETURN QUERY SELECT false,NULL::bigint; RETURN;
 END IF;
 IF item.id IS NULL THEN RETURN QUERY SELECT false,NULL::bigint; RETURN; END IF;
 SELECT m.data->'booking_notification',m.data ? 'real_estate_notification' INTO marker,property_marker FROM platform.messages m
 WHERE m.id=item.id AND m.organization_id=item.organization_id AND m.conversation_id=item.conversation_id;
 IF property_marker THEN
  key:=hashtextextended('real-estate-notification:'||item.organization_id::text,0);
  PERFORM pg_advisory_lock(key);
  IF item.mode='mock' AND item.lease_until>clock_timestamp()+interval '6 seconds'
   AND platform.real_estate_notification_current(item.id,item.organization_id,item.conversation_id)
   AND EXISTS(SELECT 1 FROM platform.conversations c WHERE (c.organization_id,c.id)=(item.organization_id,item.conversation_id) AND c.channel=p_channel
    AND ((p_channel='telegram' AND EXISTS(SELECT 1 FROM platform.telegram_principals p
      WHERE (p.organization_id,p.conversation_id,p.integration_id)=(c.organization_id,c.id,item.integration_id) AND p.chat_id::text=item.recipient))
     OR (p_channel='whatsapp' AND EXISTS(SELECT 1 FROM platform.channel_principals p
      WHERE (p.organization_id,p.conversation_id,p.integration_id,p.id)=(c.organization_id,c.id,item.integration_id,c.customer_id)
      AND p.external_user_id=item.recipient AND p.channel='whatsapp' AND p.window_until>clock_timestamp())))) THEN
   RETURN QUERY SELECT true,key;
  ELSE
   PERFORM pg_advisory_unlock(key);
   IF p_channel='telegram' THEN
    UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE id=item.id AND capability=p_cap;
   END IF;
   RETURN QUERY SELECT false,NULL::bigint;
  END IF;
  RETURN;
 END IF;
 IF item.booking_order_id IS NULL THEN RETURN QUERY SELECT marker IS NULL,NULL::bigint; RETURN; END IF;
 key:=hashtextextended('booking-notification:'||item.booking_order_id::text,0);
 PERFORM pg_advisory_lock(key);
 IF item.mode='mock' AND item.lease_until>clock_timestamp()+interval '6 seconds'
 AND EXISTS(SELECT 1 FROM platform.orders o JOIN platform.conversations c ON c.organization_id=o.organization_id AND c.id=o.conversation_id
  WHERE o.organization_id=item.organization_id AND o.conversation_id=item.conversation_id
  AND o.id=item.booking_order_id AND o.id::text=marker->>'order' AND o.version::text=marker->>'version' AND o.status=marker->>'status'
  AND c.customer_id=o.customer_id AND c.channel=p_channel AND c.status='open' AND c.expires_at>clock_timestamp()
  AND ((p_channel='telegram' AND EXISTS(SELECT 1 FROM platform.telegram_principals p
    WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id
    AND p.integration_id=item.integration_id AND p.chat_id::text=item.recipient))
   OR (p_channel='whatsapp' AND EXISTS(SELECT 1 FROM platform.channel_principals p
    WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND p.id=c.customer_id
    AND p.integration_id=item.integration_id AND p.external_user_id=item.recipient AND p.channel='whatsapp'
    AND p.window_until>clock_timestamp())))
  AND platform.booking_notification_consent(c.organization_id,c.id)
  AND EXISTS(SELECT 1 FROM platform.organizations x WHERE x.id=o.organization_id AND x.is_test)
  AND (o.status<>'confirmed' OR (EXISTS(SELECT 1 FROM platform.service_bookings b WHERE b.organization_id=o.organization_id AND b.request_id=o.id AND b.status='confirmed')
   AND (o.request_kind<>'hybrid_order' OR EXISTS(SELECT 1 FROM platform.hybrid_reservations h
    WHERE h.organization_id=o.organization_id AND h.order_id=o.id AND h.status='active' AND h.expires_at>clock_timestamp()+interval '6 seconds'))))) THEN
  RETURN QUERY SELECT true,key;
 ELSE
  PERFORM pg_advisory_unlock(key);
  IF p_channel='telegram' THEN
   UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE id=item.id AND capability=p_cap;
  END IF;
  RETURN QUERY SELECT false,NULL::bigint;
 END IF;
EXCEPTION WHEN query_canceled OR OTHERS THEN
 IF key IS NOT NULL THEN PERFORM pg_advisory_unlock(key); END IF;
 RAISE;
END $$;
UPDATE platform.service_metadata SET version=28 WHERE singleton;
-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.agent_tasks WHERE action IN ('delegate_real_estate','delegate_property_viewing'))
 OR EXISTS(SELECT 1 FROM platform.messages WHERE data ? 'booking_notification')
 OR EXISTS(SELECT 1 FROM platform.telegram_outbox WHERE booking_order_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM platform.channel_outbox WHERE booking_order_id IS NOT NULL)
 OR EXISTS(SELECT 1 FROM platform.property_listings p WHERE
 transaction_type IS NOT NULL OR description IS NOT NULL OR country IS NOT NULL OR region IS NOT NULL OR city IS NOT NULL
 OR district IS NOT NULL OR address IS NOT NULL OR living_area_m2 IS NOT NULL OR land_area_m2 IS NOT NULL OR bedrooms IS NOT NULL
 OR bathrooms IS NOT NULL OR floor IS NOT NULL OR total_floors IS NOT NULL OR building_type IS NOT NULL OR year_built IS NOT NULL
 OR renovation_status IS NOT NULL OR furnishing_status IS NOT NULL OR has_elevator IS NOT NULL OR has_parking IS NOT NULL
 OR property_status<>'active' OR published_at IS NOT NULL OR updated_at IS NOT NULL OR currency_code<>'RUB' OR property_type='new_development')
 THEN RAISE EXCEPTION 'rollback refused: preserve real estate data and history'; END IF;
END $$;
DROP TRIGGER channel_real_estate_delivery ON platform.channel_outbox;
DROP TRIGGER telegram_real_estate_delivery ON platform.telegram_outbox;
DROP TRIGGER viewing_notification_transition ON platform.orders;
DROP TRIGGER enqueue_real_estate_notification ON platform.agent_executions;
DROP TRIGGER enqueue_service_receipt ON platform.order_events;
CREATE TRIGGER enqueue_service_receipt AFTER INSERT ON platform.order_events FOR EACH ROW
 WHEN(NEW.kind IN ('confirmed','rejected','cancelled','completed'))
 EXECUTE FUNCTION platform.enqueue_service_receipt();

CREATE OR REPLACE FUNCTION platform.enqueue_service_receipt() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE source platform.agent_tasks; revision bigint; task_id uuid:=gen_random_uuid();
BEGIN
 SELECT t.* INTO source
 FROM platform.orders o JOIN platform.agent_tasks t
 ON t.organization_id=o.organization_id AND t.id=o.idempotency_key AND t.conversation_id=o.conversation_id
 WHERE o.organization_id=NEW.organization_id AND o.id=NEW.order_id AND o.status=NEW.kind
 AND o.request_kind IN ('service_booking','hybrid_order')
 AND t.action='delegate_service' AND t.status='completed'
 AND EXISTS(SELECT 1 FROM platform.agent_executions e WHERE e.organization_id=t.organization_id
  AND e.task_id=t.id AND e.status='completed' AND e.result->0->>'state'='approval_required'
  AND e.result->0->'proposal'->'details'=o.request_details AND e.result->0->'proposal'->>'kind'=o.request_kind);
 IF NOT FOUND THEN RETURN NEW; END IF;
 SELECT version INTO STRICT revision FROM platform.orders WHERE organization_id=NEW.organization_id AND id=NEW.order_id;
 INSERT INTO platform.agent_tasks(id,organization_id,initiator,agent_id,parent_id,correlation_id,conversation_id,action,idempotency_key,worker_hash,status,deadline,delegation_step)
 VALUES(task_id,source.organization_id,source.initiator,'business_manager',source.id,source.correlation_id,
 source.conversation_id,'delegation.service_receipt',NEW.id,source.worker_hash,'queued',clock_timestamp()+interval '24 hours',5+revision)
 ON CONFLICT DO NOTHING;
 IF NOT FOUND THEN RETURN NEW; END IF;
 INSERT INTO platform.channel_inbox(id,organization_id,external_id,external_user_id,payload,channel,agent_task_id)
 VALUES(task_id,source.organization_id,task_id::text,'','{}','agent',task_id);
 INSERT INTO platform.agent_audit(organization_id,task_id,actor_id,agent_id,action,result)
 VALUES(source.organization_id,task_id,NEW.actor_id,'business_manager','task.delegate','queued');
 RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION platform.booking_notification_admit(p_cap uuid,p_channel text)
RETURNS TABLE(allowed boolean,lock_key bigint)
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,platform,pg_temp AS $$
DECLARE item record; key bigint; marker jsonb;
BEGIN
 IF p_channel='telegram' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.lease_until,q.booking_order_id,q.integration_id,q.chat_id::text AS recipient,i.mode INTO item
  FROM platform.telegram_outbox q JOIN platform.telegram_integrations i ON i.id=q.integration_id AND i.organization_id=q.organization_id
  WHERE q.capability=p_cap AND q.status='sending';
 ELSIF p_channel='whatsapp' THEN
  SELECT q.id,q.organization_id,q.conversation_id,q.lease_until,q.booking_order_id,q.integration_id,q.external_user_id AS recipient,i.mode INTO item
  FROM platform.channel_outbox q JOIN platform.channel_integrations i ON i.id=q.integration_id AND i.organization_id=q.organization_id
  WHERE q.capability=p_cap AND q.status='sending' AND i.channel='whatsapp';
 ELSE RETURN QUERY SELECT false,NULL::bigint; RETURN;
 END IF;
 IF item.id IS NULL THEN RETURN QUERY SELECT false,NULL::bigint; RETURN; END IF;
 SELECT m.data->'booking_notification' INTO marker FROM platform.messages m
 WHERE m.id=item.id AND m.organization_id=item.organization_id AND m.conversation_id=item.conversation_id;
 IF item.booking_order_id IS NULL THEN RETURN QUERY SELECT marker IS NULL,NULL::bigint; RETURN; END IF;
 key:=hashtextextended('booking-notification:'||item.booking_order_id::text,0);
 PERFORM pg_advisory_lock(key);
 IF item.mode='mock' AND item.lease_until>clock_timestamp()+interval '6 seconds'
 AND EXISTS(SELECT 1 FROM platform.orders o JOIN platform.conversations c ON c.organization_id=o.organization_id AND c.id=o.conversation_id
  WHERE o.organization_id=item.organization_id AND o.conversation_id=item.conversation_id
  AND o.id=item.booking_order_id AND o.id::text=marker->>'order' AND o.version::text=marker->>'version' AND o.status=marker->>'status'
  AND c.customer_id=o.customer_id AND c.channel=p_channel AND c.status='open' AND c.expires_at>clock_timestamp()
  AND ((p_channel='telegram' AND EXISTS(SELECT 1 FROM platform.telegram_principals p
    WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id
    AND p.integration_id=item.integration_id AND p.chat_id::text=item.recipient))
   OR (p_channel='whatsapp' AND EXISTS(SELECT 1 FROM platform.channel_principals p
    WHERE p.organization_id=c.organization_id AND p.conversation_id=c.id AND p.id=c.customer_id
    AND p.integration_id=item.integration_id AND p.external_user_id=item.recipient AND p.channel='whatsapp'
    AND p.window_until>clock_timestamp())))
  AND platform.booking_notification_consent(c.organization_id,c.id)
  AND EXISTS(SELECT 1 FROM platform.organizations x WHERE x.id=o.organization_id AND x.is_test)
  AND (o.status<>'confirmed' OR (EXISTS(SELECT 1 FROM platform.service_bookings b WHERE b.organization_id=o.organization_id AND b.request_id=o.id AND b.status='confirmed')
   AND (o.request_kind<>'hybrid_order' OR EXISTS(SELECT 1 FROM platform.hybrid_reservations h
    WHERE h.organization_id=o.organization_id AND h.order_id=o.id AND h.status='active' AND h.expires_at>clock_timestamp()+interval '6 seconds'))))) THEN
  RETURN QUERY SELECT true,key;
 ELSE
  PERFORM pg_advisory_unlock(key);
  IF p_channel='telegram' THEN
   UPDATE platform.telegram_outbox SET status='cancelled',capability=NULL,lease_until=NULL WHERE id=item.id AND capability=p_cap;
  END IF;
  RETURN QUERY SELECT false,NULL::bigint;
 END IF;
EXCEPTION WHEN query_canceled OR OTHERS THEN
 IF key IS NOT NULL THEN PERFORM pg_advisory_unlock(key); END IF;
 RAISE;
END $$;
DROP FUNCTION platform.audit_real_estate_notification_state(),platform.real_estate_notification_transition(),
 platform.enqueue_real_estate_notification(),platform.real_estate_notification_current(uuid,uuid,uuid),platform.property_receipt_context(uuid,uuid);
DROP FUNCTION platform.agent_property_selection(uuid,uuid),platform.agent_delegation_properties(uuid,uuid),
 platform.property_search_page(uuid,jsonb,int),platform.property_json(platform.property_listings,boolean),
 platform.property_availability(text,text,timestamptz),platform.property_candidates(uuid,jsonb,text);
DROP INDEX platform.property_listings_market_search;
ALTER TABLE platform.property_listings
 DROP CONSTRAINT property_available_active, DROP CONSTRAINT property_floor_range,
 DROP CONSTRAINT property_listings_property_type_check, DROP CONSTRAINT property_price_currency,
 DROP COLUMN transaction_type, DROP COLUMN description, DROP COLUMN country, DROP COLUMN region, DROP COLUMN city,
 DROP COLUMN district, DROP COLUMN address, DROP COLUMN living_area_m2, DROP COLUMN land_area_m2,
 DROP COLUMN bedrooms, DROP COLUMN bathrooms, DROP COLUMN floor, DROP COLUMN total_floors,
 DROP COLUMN building_type, DROP COLUMN year_built, DROP COLUMN renovation_status, DROP COLUMN furnishing_status,
 DROP COLUMN has_elevator, DROP COLUMN has_parking, DROP COLUMN property_status, DROP COLUMN published_at, DROP COLUMN updated_at,
 ADD CONSTRAINT property_listings_property_type_check CHECK(property_type IN ('apartment','house','commercial','land','other')),
 ADD CONSTRAINT property_price_currency CHECK(
 (price_amount IS NULL AND currency_code IS NULL)
 OR (price_amount IS NOT NULL AND price_amount>=0 AND currency_code IS NOT NULL AND currency_code='RUB'));
ALTER TABLE platform.agent_tasks DROP CONSTRAINT agent_task_operation;
ALTER TABLE platform.agent_tasks ADD CONSTRAINT agent_task_operation CHECK (
 (agent_id IN ('customer','sales','product_inventory') AND action IN ('get_product_price','check_inventory','get_product_details') AND variant_id IS NOT NULL AND conversation_id IS NOT NULL)
 OR (variant_id IS NULL AND conversation_id IS NULL AND ((agent_id='marketing' AND action='draft') OR
 (agent_id IN ('finance','analytics') AND action='read') OR (agent_id='business_manager' AND action IN ('plan','coordinate','coordinate_growth'))))
 OR (variant_id IS NULL AND conversation_id IS NULL AND parent_id IS NOT NULL AND
 ((agent_id='sales' AND action='review_sales') OR (agent_id='product_inventory' AND action='review_inventory')))
 OR (agent_id='customer' AND action IN ('workflow_preview_B','workflow_preview_C','workflow_preview_D','workflow_preview_E')
     AND conversation_id IS NOT NULL AND variant_id IS NULL AND parent_id IS NULL
     AND status IN ('completed','failed') AND correlation_id=id)
 OR (action IN ('delegate_auto_parts','delegate_construction','delegate_service') AND agent_id='business_manager' AND parent_id IS NULL
     AND correlation_id=id AND conversation_id IS NOT NULL AND variant_id IS NULL AND delegation_input IS NOT NULL AND delegation_step IS NULL)
 OR (action LIKE 'delegation.%' AND parent_id IS NOT NULL AND conversation_id IS NOT NULL
     AND variant_id IS NULL AND delegation_step BETWEEN 1 AND 20 AND delegation_input IS NULL));
CREATE OR REPLACE FUNCTION platform.agent_delegation_profile(p_org uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.is_test AND CASE p_action
 WHEN 'delegate_auto_parts' THEN o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_construction' THEN o.industry_profile='building_materials' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_service' THEN (o.industry_profile='vehicle_service' AND o.business_model='service')
   OR (o.industry_profile='mixed_nonfood' AND o.business_model='hybrid')
 ELSE false END)
$$;
REVOKE ALL ON FUNCTION platform.agent_delegation_profile(uuid,text) FROM PUBLIC;
CREATE OR REPLACE FUNCTION platform.agent_delegation_plan(p_action text)
RETURNS TABLE(step integer, agent text, operation text) LANGUAGE sql IMMUTABLE SET search_path=pg_catalog AS $$
 SELECT v.step,v.agent,v.operation FROM (VALUES
 ('delegate_auto_parts',1,'customer','delegation.requirements'),
 ('delegate_auto_parts',2,'sales','delegation.search_plan'),
 ('delegate_auto_parts',3,'product_inventory','delegation.find_parts'),
 ('delegate_auto_parts',4,'product_inventory','delegation.verify_fitment'),
 ('delegate_auto_parts',5,'sales','delegation.proposal'),
 ('delegate_construction',1,'customer','delegation.construction_input'),
 ('delegate_construction',2,'product_inventory','delegation.construction_catalog'),
 ('delegate_construction',3,'product_inventory','delegation.construction_calculate'),
 ('delegate_construction',4,'sales','delegation.construction_offer'),
 ('delegate_service',1,'customer','delegation.service_input'),
 ('delegate_service',2,'sales','delegation.service_select'),
 ('delegate_service',3,'sales','delegation.service_availability'),
 ('delegate_service',4,'product_inventory','delegation.service_parts'),
 ('delegate_service',5,'sales','delegation.service_proposal')
 ) v(workflow,step,agent,operation)

 WHERE workflow=p_action
$$;
UPDATE platform.service_metadata SET version=27 WHERE singleton;
-- +goose StatementEnd
