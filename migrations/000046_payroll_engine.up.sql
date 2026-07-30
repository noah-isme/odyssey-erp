CREATE TABLE payroll_rule_versions (
    id BIGSERIAL PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    rule_type TEXT NOT NULL CHECK (rule_type IN ('TAX','BPJS','COMPANY')),
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    source_name TEXT NOT NULL,
    source_url TEXT NOT NULL,
    source_reference TEXT NOT NULL,
    reviewed_by BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
    reviewed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE payroll_ptkp_statuses (
    id BIGSERIAL PRIMARY KEY,
    rule_version_id BIGINT NOT NULL REFERENCES payroll_rule_versions(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    ter_category TEXT NOT NULL CHECK (ter_category IN ('A','B','C')),
    annual_amount NUMERIC(18,2) NOT NULL CHECK (annual_amount >= 0),
    UNIQUE (rule_version_id, code)
);

CREATE TABLE payroll_ter_brackets (
    id BIGSERIAL PRIMARY KEY,
    rule_version_id BIGINT NOT NULL REFERENCES payroll_rule_versions(id) ON DELETE RESTRICT,
    category TEXT NOT NULL CHECK (category IN ('A','B','C')),
    lower_bound NUMERIC(18,2) NOT NULL CHECK (lower_bound >= 0),
    upper_bound NUMERIC(18,2) NULL,
    rate_bps INTEGER NOT NULL CHECK (rate_bps BETWEEN 0 AND 10000),
    UNIQUE (rule_version_id, category, lower_bound),
    CHECK (upper_bound IS NULL OR upper_bound > lower_bound)
);

CREATE TABLE payroll_bpjs_rates (
    id BIGSERIAL PRIMARY KEY,
    rule_version_id BIGINT NOT NULL REFERENCES payroll_rule_versions(id) ON DELETE RESTRICT,
    program TEXT NOT NULL CHECK (program IN ('HEALTH','JHT','JP','JKK','JKM')),
    risk_class TEXT NOT NULL DEFAULT '',
    employee_rate_bps INTEGER NOT NULL DEFAULT 0 CHECK (employee_rate_bps BETWEEN 0 AND 10000),
    employer_rate_bps INTEGER NOT NULL DEFAULT 0 CHECK (employer_rate_bps BETWEEN 0 AND 10000),
    employer_taxable BOOLEAN NOT NULL DEFAULT FALSE,
    wage_floor NUMERIC(18,2) NULL,
    wage_cap NUMERIC(18,2) NULL,
    UNIQUE (rule_version_id, program, risk_class),
    CHECK (wage_floor IS NULL OR wage_floor >= 0),
    CHECK (wage_cap IS NULL OR wage_cap >= 0)
);

CREATE TABLE payroll_company_policies (
    id BIGSERIAL PRIMARY KEY,
    rule_version_id BIGINT NOT NULL REFERENCES payroll_rule_versions(id) ON DELETE RESTRICT,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    overtime_divisor INTEGER NOT NULL DEFAULT 173 CHECK (overtime_divisor > 0),
    first_hour_multiplier_bps INTEGER NOT NULL DEFAULT 15000 CHECK (first_hour_multiplier_bps > 0),
    subsequent_hour_multiplier_bps INTEGER NOT NULL DEFAULT 20000 CHECK (subsequent_hour_multiplier_bps > 0),
    currency TEXT NOT NULL DEFAULT 'IDR',
    rounding_unit INTEGER NOT NULL DEFAULT 1 CHECK (rounding_unit > 0),
    jkk_risk_class TEXT NOT NULL DEFAULT 'LOW',
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    UNIQUE (company_id, effective_from),
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE payroll_periods (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    pay_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','CLOSED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code),
    CHECK (ends_on >= starts_on)
);

CREATE TABLE payroll_compensation_assignments (
    id BIGSERIAL PRIMARY KEY,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    base_salary NUMERIC(18,2) NOT NULL CHECK (base_salary >= 0),
    ptkp_code TEXT NOT NULL,
    bpjs_health BOOLEAN NOT NULL DEFAULT TRUE,
    bpjs_employment BOOLEAN NOT NULL DEFAULT TRUE,
    bank_code TEXT NOT NULL DEFAULT '',
    bank_account_number TEXT NOT NULL DEFAULT '',
    bank_account_name TEXT NOT NULL DEFAULT '',
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (effective_to IS NULL OR effective_to >= effective_from),
    UNIQUE (employee_id, effective_from)
);

CREATE TABLE payroll_components (
    id BIGSERIAL PRIMARY KEY,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('EARNING','DEDUCTION','EMPLOYER_CONTRIBUTION')),
    taxable BOOLEAN NOT NULL DEFAULT TRUE,
    bpjs_base BOOLEAN NOT NULL DEFAULT FALSE,
    recurring BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (company_id, code)
);

CREATE TABLE payroll_recurring_components (
    id BIGSERIAL PRIMARY KEY,
    assignment_id BIGINT NOT NULL REFERENCES payroll_compensation_assignments(id) ON DELETE CASCADE,
    component_id BIGINT NOT NULL REFERENCES payroll_components(id) ON DELETE RESTRICT,
    amount NUMERIC(18,2) NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    CHECK (effective_to IS NULL OR effective_to >= effective_from)
);

CREATE TABLE payroll_adjustments (
    id BIGSERIAL PRIMARY KEY,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    period_id BIGINT NOT NULL REFERENCES payroll_periods(id) ON DELETE CASCADE,
    component_id BIGINT NOT NULL REFERENCES payroll_components(id) ON DELETE RESTRICT,
    amount NUMERIC(18,2) NOT NULL CHECK (amount <> 0),
    reason TEXT NOT NULL,
    reversal_of_id BIGINT NULL REFERENCES payroll_adjustments(id) ON DELETE RESTRICT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payroll_overtime (
    id BIGSERIAL PRIMARY KEY,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    period_id BIGINT NOT NULL REFERENCES payroll_periods(id) ON DELETE CASCADE,
    worked_on DATE NOT NULL,
    minutes INTEGER NOT NULL CHECK (minutes > 0),
    approved_by BIGINT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payroll_thr (
    id BIGSERIAL PRIMARY KEY,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE CASCADE,
    period_id BIGINT NOT NULL REFERENCES payroll_periods(id) ON DELETE CASCADE,
    amount NUMERIC(18,2) NOT NULL CHECK (amount >= 0),
    service_months INTEGER NOT NULL CHECK (service_months >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (employee_id, period_id)
);

CREATE TABLE payroll_runs (
    id BIGSERIAL PRIMARY KEY,
    run_uuid UUID NOT NULL UNIQUE,
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    period_id BIGINT NOT NULL REFERENCES payroll_periods(id) ON DELETE RESTRICT,
    run_type TEXT NOT NULL DEFAULT 'REGULAR' CHECK (run_type IN ('REGULAR','ADJUSTMENT','REVERSAL')),
    reversal_of_id BIGINT NULL REFERENCES payroll_runs(id) ON DELETE RESTRICT,
    tax_rule_version_id BIGINT NOT NULL REFERENCES payroll_rule_versions(id) ON DELETE RESTRICT,
    bpjs_rule_version_id BIGINT NOT NULL REFERENCES payroll_rule_versions(id) ON DELETE RESTRICT,
    company_policy_id BIGINT NOT NULL REFERENCES payroll_company_policies(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','APPROVAL','POSTED','REVERSED')),
    approval_request_id BIGINT NULL REFERENCES approval_requests(id) ON DELETE SET NULL,
    journal_entry_id BIGINT NULL REFERENCES journal_entries(id) ON DELETE RESTRICT,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    submitted_at TIMESTAMPTZ NULL,
    posted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX payroll_one_posted_regular_run ON payroll_runs(company_id, period_id)
    WHERE status = 'POSTED' AND run_type = 'REGULAR';

CREATE TABLE payroll_run_lines (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES payroll_runs(id) ON DELETE RESTRICT,
    employee_id BIGINT NOT NULL REFERENCES hr_employees(id) ON DELETE RESTRICT,
    department_id BIGINT NULL REFERENCES hr_departments(id) ON DELETE SET NULL,
    cost_center_id BIGINT NULL REFERENCES cost_centers(id) ON DELETE SET NULL,
    ptkp_code TEXT NOT NULL,
    ter_category TEXT NOT NULL CHECK (ter_category IN ('A','B','C')),
    base_salary NUMERIC(18,2) NOT NULL,
    allowances NUMERIC(18,2) NOT NULL DEFAULT 0,
    overtime NUMERIC(18,2) NOT NULL DEFAULT 0,
    thr NUMERIC(18,2) NOT NULL DEFAULT 0,
    gross NUMERIC(18,2) NOT NULL,
    employee_bpjs NUMERIC(18,2) NOT NULL DEFAULT 0,
    employer_bpjs NUMERIC(18,2) NOT NULL DEFAULT 0,
    pph21 NUMERIC(18,2) NOT NULL DEFAULT 0,
    other_deductions NUMERIC(18,2) NOT NULL DEFAULT 0,
    net_pay NUMERIC(18,2) NOT NULL,
    breakdown JSONB NOT NULL DEFAULT '[]'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, employee_id)
);

CREATE TABLE payroll_run_deductions (
    id BIGSERIAL PRIMARY KEY,
    run_line_id BIGINT NOT NULL REFERENCES payroll_run_lines(id) ON DELETE RESTRICT,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    amount NUMERIC(18,2) NOT NULL CHECK (amount >= 0),
    employee_paid BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE TABLE payroll_payslips (
    id BIGSERIAL PRIMARY KEY,
    run_line_id BIGINT NOT NULL UNIQUE REFERENCES payroll_run_lines(id) ON DELETE RESTRICT,
    document_key TEXT NOT NULL UNIQUE,
    checksum TEXT NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ NULL
);

CREATE TABLE payroll_payment_batches (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL UNIQUE REFERENCES payroll_runs(id) ON DELETE RESTRICT,
    format TEXT NOT NULL DEFAULT 'CSV',
    status TEXT NOT NULL DEFAULT 'READY' CHECK (status IN ('READY','EXPORTED','PAID','CANCELLED')),
    instruction_count INTEGER NOT NULL,
    total_amount NUMERIC(18,2) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payroll_account_mappings (
    company_id BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    mapping_type TEXT NOT NULL CHECK (mapping_type IN ('SALARY_EXPENSE','EMPLOYER_BPJS_EXPENSE','PAYROLL_PAYABLE','PPH21_PAYABLE','BPJS_PAYABLE')),
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    PRIMARY KEY (company_id, mapping_type)
);

CREATE OR REPLACE FUNCTION prevent_posted_payroll_run_mutation() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status = 'POSTED' AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'posted payroll runs are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER payroll_runs_posted_immutable BEFORE UPDATE ON payroll_runs
FOR EACH ROW EXECUTE FUNCTION prevent_posted_payroll_run_mutation();

CREATE OR REPLACE FUNCTION prevent_posted_payroll_line_mutation() RETURNS TRIGGER AS $$
DECLARE target_run_id BIGINT;
BEGIN
    IF TG_OP = 'DELETE' THEN target_run_id := OLD.run_id; ELSE target_run_id := NEW.run_id; END IF;
    IF EXISTS (SELECT 1 FROM payroll_runs WHERE id=target_run_id AND status='POSTED') THEN
        RAISE EXCEPTION 'posted payroll lines are immutable';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER payroll_lines_posted_immutable BEFORE INSERT OR UPDATE OR DELETE ON payroll_run_lines
FOR EACH ROW EXECUTE FUNCTION prevent_posted_payroll_line_mutation();

INSERT INTO payroll_rule_versions(code,rule_type,effective_from,source_name,source_url,source_reference,reviewed_at)
VALUES
 ('DJP-PP58-2023','TAX','2024-01-01','JDIH Kementerian Keuangan','https://jdih.kemenkeu.go.id/download/e47c3fc4-a912-4bf1-bcad-335fee3f71f8/2023pp058.pdf','PP 58 Tahun 2023; PMK 168 Tahun 2023',NOW()),
 ('BPJS-2026-03','BPJS','2026-03-01','BPJS Ketenagakerjaan / BPJS Kesehatan','https://www.bpjsketenagakerjaan.go.id/penerima-upah.html','PP 44-46/2015; Perpres 64/2020; BPJS letter B/1226/022026',NOW());

INSERT INTO payroll_ptkp_statuses(rule_version_id,code,ter_category,annual_amount)
SELECT id, v.code, v.category, v.amount FROM payroll_rule_versions,
(VALUES ('TK/0','A',54000000),('TK/1','A',58500000),('K/0','A',58500000),
        ('TK/2','B',63000000),('K/1','B',63000000),('TK/3','B',67500000),('K/2','B',67500000),
        ('K/3','C',72000000)) v(code,category,amount)
WHERE payroll_rule_versions.code='DJP-PP58-2023';

INSERT INTO payroll_ter_brackets(rule_version_id,category,lower_bound,upper_bound,rate_bps)
SELECT rv.id, x.category, x.lo, x.hi, x.rate FROM payroll_rule_versions rv CROSS JOIN (VALUES
('A',0,5400000,0),('A',5400000,5650000,25),('A',5650000,5950000,50),('A',5950000,6300000,75),('A',6300000,6750000,100),('A',6750000,7500000,125),('A',7500000,8550000,150),('A',8550000,9650000,175),('A',9650000,10050000,200),('A',10050000,10350000,225),('A',10350000,10700000,250),('A',10700000,11050000,300),('A',11050000,11600000,350),('A',11600000,12500000,400),('A',12500000,13750000,500),('A',13750000,15100000,600),('A',15100000,16950000,700),('A',16950000,19750000,800),('A',19750000,24150000,900),('A',24150000,26450000,1000),('A',26450000,28000000,1100),('A',28000000,30050000,1200),('A',30050000,32400000,1300),('A',32400000,35400000,1400),('A',35400000,39100000,1500),('A',39100000,43850000,1600),('A',43850000,47800000,1700),('A',47800000,51400000,1800),('A',51400000,56300000,1900),('A',56300000,62200000,2000),('A',62200000,68600000,2100),('A',68600000,77500000,2200),('A',77500000,89000000,2300),('A',89000000,103000000,2400),('A',103000000,125000000,2500),('A',125000000,157000000,2600),('A',157000000,206000000,2700),('A',206000000,337000000,2800),('A',337000000,454000000,2900),('A',454000000,550000000,3000),('A',550000000,695000000,3100),('A',695000000,910000000,3200),('A',910000000,1400000000,3300),('A',1400000000,NULL,3400),
('B',0,6200000,0),('B',6200000,6500000,25),('B',6500000,6850000,50),('B',6850000,7300000,75),('B',7300000,9200000,100),('B',9200000,10750000,150),('B',10750000,11250000,200),('B',11250000,11600000,250),('B',11600000,12600000,300),('B',12600000,13600000,400),('B',13600000,14950000,500),('B',14950000,16400000,600),('B',16400000,18450000,700),('B',18450000,21850000,800),('B',21850000,26000000,900),('B',26000000,27700000,1000),('B',27700000,29350000,1100),('B',29350000,31450000,1200),('B',31450000,33950000,1300),('B',33950000,37100000,1400),('B',37100000,41100000,1500),('B',41100000,45800000,1600),('B',45800000,49500000,1700),('B',49500000,53800000,1800),('B',53800000,58500000,1900),('B',58500000,64000000,2000),('B',64000000,71000000,2100),('B',71000000,80000000,2200),('B',80000000,93000000,2300),('B',93000000,109000000,2400),('B',109000000,129000000,2500),('B',129000000,163000000,2600),('B',163000000,211000000,2700),('B',211000000,374000000,2800),('B',374000000,459000000,2900),('B',459000000,555000000,3000),('B',555000000,704000000,3100),('B',704000000,957000000,3200),('B',957000000,1405000000,3300),('B',1405000000,NULL,3400),
('C',0,6600000,0),('C',6600000,6950000,25),('C',6950000,7350000,50),('C',7350000,7800000,75),('C',7800000,8850000,100),('C',8850000,9800000,125),('C',9800000,10950000,150),('C',10950000,11200000,175),('C',11200000,12050000,200),('C',12050000,12950000,300),('C',12950000,14150000,400),('C',14150000,15550000,500),('C',15550000,17050000,600),('C',17050000,19500000,700),('C',19500000,22700000,800),('C',22700000,26600000,900),('C',26600000,28100000,1000),('C',28100000,30100000,1100),('C',30100000,32600000,1200),('C',32600000,35400000,1300),('C',35400000,38900000,1400),('C',38900000,43000000,1500),('C',43000000,47400000,1600),('C',47400000,51200000,1700),('C',51200000,55800000,1800),('C',55800000,60400000,1900),('C',60400000,66700000,2000),('C',66700000,74500000,2100),('C',74500000,83200000,2200),('C',83200000,95600000,2300),('C',95600000,110000000,2400),('C',110000000,134000000,2500),('C',134000000,169000000,2600),('C',169000000,221000000,2700),('C',221000000,390000000,2800),('C',390000000,463000000,2900),('C',463000000,561000000,3000),('C',561000000,709000000,3100),('C',709000000,965000000,3200),('C',965000000,1419000000,3300),('C',1419000000,NULL,3400)
) x(category,lo,hi,rate) WHERE rv.code='DJP-PP58-2023';

INSERT INTO payroll_bpjs_rates(rule_version_id,program,risk_class,employee_rate_bps,employer_rate_bps,wage_cap,employer_taxable)
SELECT id,x.program,x.risk,x.employee,x.employer,x.cap,x.taxable FROM payroll_rule_versions CROSS JOIN (VALUES
('HEALTH','',100,400,12000000,TRUE),('JHT','',200,370,NULL,FALSE),('JP','',100,200,11086300,FALSE),
('JKK','VERY_LOW',0,24,NULL,TRUE),('JKK','LOW',0,54,NULL,TRUE),('JKK','MEDIUM',0,89,NULL,TRUE),('JKK','HIGH',0,127,NULL,TRUE),('JKK','VERY_HIGH',0,174,NULL,TRUE),('JKM','',0,30,NULL,TRUE)
) x(program,risk,employee,employer,cap,taxable) WHERE code='BPJS-2026-03';

INSERT INTO permissions(name,description) VALUES
 ('payroll.view','View payroll runs'),('payroll.process','Create, calculate, and submit payroll runs'),
 ('payroll.post','Post approved payroll and export payments'),('payroll.policy.admin','Manage payroll rules and account mappings'),
 ('payroll.payslip.own','View own payslips'),('payroll.payslip.manager','View authorized reports payslips')
ON CONFLICT(name) DO UPDATE SET description=EXCLUDED.description;
INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.name='Admin' AND p.name LIKE 'payroll.%' ON CONFLICT DO NOTHING;
