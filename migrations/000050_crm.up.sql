-- Phase 6: CRM sales front-end. Customers and quotations remain authoritative.
CREATE TABLE crm_pipeline_stages (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL CHECK (position > 0),
    stage_type TEXT NOT NULL DEFAULT 'OPEN' CHECK (stage_type IN ('OPEN','WON','LOST')),
    probability INTEGER NOT NULL DEFAULT 0 CHECK (probability BETWEEN 0 AND 100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(company_id,name), UNIQUE(company_id,position)
);

INSERT INTO crm_pipeline_stages(company_id,name,position,stage_type,probability)
SELECT c.id,s.name,s.position,s.stage_type,s.probability FROM companies c CROSS JOIN (VALUES
 ('Qualified',1,'OPEN',20),('Discovery',2,'OPEN',40),('Proposal',3,'OPEN',65),
 ('Negotiation',4,'OPEN',85),('Won',5,'WON',100),('Lost',6,'LOST',0)
) AS s(name,position,stage_type,probability) ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION create_default_crm_stages() RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO crm_pipeline_stages(company_id,name,position,stage_type,probability) VALUES
      (NEW.id,'Qualified',1,'OPEN',20),(NEW.id,'Discovery',2,'OPEN',40),(NEW.id,'Proposal',3,'OPEN',65),
      (NEW.id,'Negotiation',4,'OPEN',85),(NEW.id,'Won',5,'WON',100),(NEW.id,'Lost',6,'LOST',0);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_company_default_crm_stages AFTER INSERT ON companies
FOR EACH ROW EXECUTE FUNCTION create_default_crm_stages();

CREATE TABLE crm_leads (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    source TEXT NOT NULL DEFAULT 'OTHER',
    name TEXT NOT NULL,
    organization TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'NEW' CHECK (status IN ('NEW','QUALIFIED','CONVERTED','DISQUALIFIED')),
    notes TEXT NOT NULL DEFAULT '',
    converted_customer_id BIGINT REFERENCES customers(id) ON DELETE RESTRICT,
    converted_contact_id BIGINT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_crm_leads_visibility ON crm_leads(company_id,owner_id,status);

CREATE TABLE crm_contacts (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    customer_id BIGINT REFERENCES customers(id) ON DELETE RESTRICT,
    lead_id BIGINT REFERENCES crm_leads(id) ON DELETE SET NULL,
    name TEXT NOT NULL, email TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX uq_crm_contacts_company_email ON crm_contacts(company_id,LOWER(email)) WHERE email <> '';
ALTER TABLE crm_leads ADD CONSTRAINT fk_crm_lead_contact FOREIGN KEY(converted_contact_id) REFERENCES crm_contacts(id) ON DELETE RESTRICT;

CREATE TABLE crm_opportunities (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    lead_id BIGINT REFERENCES crm_leads(id) ON DELETE RESTRICT,
    contact_id BIGINT REFERENCES crm_contacts(id) ON DELETE RESTRICT,
    customer_id BIGINT REFERENCES customers(id) ON DELETE RESTRICT,
    quotation_id BIGINT REFERENCES quotations(id) ON DELETE RESTRICT,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    stage_id BIGINT NOT NULL REFERENCES crm_pipeline_stages(id) ON DELETE RESTRICT,
    name TEXT NOT NULL, source TEXT NOT NULL DEFAULT 'OTHER',
    expected_value NUMERIC(18,2) NOT NULL DEFAULT 0 CHECK(expected_value >= 0),
    close_date DATE, status TEXT NOT NULL DEFAULT 'OPEN' CHECK(status IN ('OPEN','WON','LOST')),
    win_loss_reason TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(lead_id), UNIQUE(quotation_id)
);
CREATE INDEX idx_crm_opportunities_pipeline ON crm_opportunities(company_id,owner_id,status,stage_id);

CREATE TABLE crm_activities (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    lead_id BIGINT REFERENCES crm_leads(id) ON DELETE CASCADE,
    opportunity_id BIGINT REFERENCES crm_opportunities(id) ON DELETE CASCADE,
    contact_id BIGINT REFERENCES crm_contacts(id) ON DELETE SET NULL,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    activity_type TEXT NOT NULL CHECK(activity_type IN ('CALL','EMAIL','MEETING','TASK','NOTE')),
    subject TEXT NOT NULL, body TEXT NOT NULL DEFAULT '', due_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ, reminder_at TIMESTAMPTZ, reminder_sent_at TIMESTAMPTZ, escalated_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK(lead_id IS NOT NULL OR opportunity_id IS NOT NULL OR contact_id IS NOT NULL)
);
CREATE INDEX idx_crm_activities_due ON crm_activities(reminder_at,due_at) WHERE completed_at IS NULL;

CREATE TABLE crm_events (
    id BIGSERIAL PRIMARY KEY, company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL, entity_id BIGINT NOT NULL, event_type TEXT NOT NULL,
    actor_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT, details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_crm_events_timeline ON crm_events(company_id,entity_type,entity_id,created_at DESC);

INSERT INTO permissions(name,description) VALUES
 ('crm.view','View owned CRM records'),('crm.create','Create CRM leads, opportunities, and activities'),
 ('crm.edit','Update owned CRM records'),('crm.convert','Convert won opportunities to customers and quotations'),
 ('crm.team.view','View all company CRM records'),('crm.manage','Administer all company CRM records')
ON CONFLICT(name) DO UPDATE SET description=EXCLUDED.description;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE LOWER(r.name) IN ('admin','administrator') AND p.name LIKE 'crm.%' ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE LOWER(r.name) IN ('sales manager','manager') AND p.name IN ('crm.view','crm.create','crm.edit','crm.convert','crm.team.view') ON CONFLICT DO NOTHING;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE LOWER(r.name) IN ('sales rep','sales representative','sales') AND p.name IN ('crm.view','crm.create','crm.edit','crm.convert') ON CONFLICT DO NOTHING;
