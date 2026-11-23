-- +goose Up
-- +goose StatementBegin
-- Auto service may require a catalog item as part of an appointment. This is
-- an additive industry/model pairing; existing profile combinations remain.
ALTER TABLE platform.organizations DROP CONSTRAINT industry_model_pair;
ALTER TABLE platform.organizations ADD CONSTRAINT industry_model_pair CHECK (
  (business_model='trade' AND industry_profile IN ('electronics','auto_parts','building_materials','furniture','apparel'))
  OR (business_model='service' AND industry_profile IN ('vehicle_service','personal_services'))
  OR (business_model='agency' AND industry_profile='real_estate')
  OR (business_model='hybrid' AND industry_profile IN ('mixed_nonfood','vehicle_service'))
);
CREATE OR REPLACE FUNCTION platform.agent_delegation_profile(p_org uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.is_test AND CASE p_action
 WHEN 'delegate_auto_parts' THEN o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_construction' THEN o.industry_profile='building_materials' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_service' THEN (o.industry_profile='vehicle_service' AND o.business_model IN ('service','hybrid'))
   OR (o.industry_profile='mixed_nonfood' AND o.business_model='hybrid')
 WHEN 'delegate_real_estate' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_property_viewing' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_conversation' THEN true
 ELSE false END)
$$;
UPDATE platform.service_metadata SET version=30 WHERE singleton;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM platform.organizations WHERE business_model='hybrid' AND industry_profile='vehicle_service') THEN
  RAISE EXCEPTION 'rollback refused: hybrid vehicle_service profiles exist';
 END IF;
END $$;
ALTER TABLE platform.organizations DROP CONSTRAINT industry_model_pair;
ALTER TABLE platform.organizations ADD CONSTRAINT industry_model_pair CHECK (
  (business_model='trade' AND industry_profile IN ('electronics','auto_parts','building_materials','furniture','apparel'))
  OR (business_model='service' AND industry_profile IN ('vehicle_service','personal_services'))
  OR (business_model='agency' AND industry_profile='real_estate')
  OR (business_model='hybrid' AND industry_profile='mixed_nonfood')
);
CREATE OR REPLACE FUNCTION platform.agent_delegation_profile(p_org uuid,p_action text) RETURNS boolean
LANGUAGE sql STABLE SET search_path=pg_catalog,platform AS $$
 SELECT EXISTS(SELECT 1 FROM platform.organizations o WHERE o.id=p_org AND o.is_test AND CASE p_action
 WHEN 'delegate_auto_parts' THEN o.industry_profile='auto_parts' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_construction' THEN o.industry_profile='building_materials' AND o.business_model IN ('trade','hybrid')
 WHEN 'delegate_service' THEN (o.industry_profile='vehicle_service' AND o.business_model='service')
   OR (o.industry_profile='mixed_nonfood' AND o.business_model='hybrid')
 WHEN 'delegate_real_estate' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_property_viewing' THEN o.industry_profile='real_estate' AND o.business_model='agency'
 WHEN 'delegate_conversation' THEN true
 ELSE false END)
$$;
UPDATE platform.service_metadata SET version=29 WHERE singleton;
-- +goose StatementEnd
