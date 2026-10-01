package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase09HRPayroll seeds HR departments, positions, employees, leave types,
// leave balances, leave requests, attendance logs, and Indonesian payroll configuration
// (rules, company policy, periods, components, compensation assignments, and a posted payroll run).
func seedPhase09HRPayroll(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 09: HR & Payroll", func(tx pgx.Tx) error {
		companyID := sctx.CompanyNTPID
		if companyID == 0 {
			var err error
			companyID, err = LookupCompanyID(ctx, tx, "NTP-HQ")
			if err != nil {
				return err
			}
			sctx.CompanyNTPID = companyID
		}

		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			var err error
			adminID, err = LookupUserID(ctx, tx, "budi.santoso@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["budi.santoso@nusantarateknik.co.id"] = adminID
		}

		hrMgrID := sctx.UserIDs["rina.wulandari@nusantarateknik.co.id"]
		if hrMgrID == 0 {
			var err error
			hrMgrID, err = LookupUserID(ctx, tx, "rina.wulandari@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["rina.wulandari@nusantarateknik.co.id"] = hrMgrID
		}

		// -------------------------------------------------------------------------
		// 1. HR Departments (9 Departments)
		// -------------------------------------------------------------------------
		type deptDef struct {
			code string
			name string
		}

		departments := []deptDef{
			{"HR-DEPT-EXEC", "Manajemen Eksekutif & Direksi"},
			{"HR-DEPT-FIN", "Keuangan, Akuntansi & Perpajakan"},
			{"HR-DEPT-HR", "Human Resources & General Affairs"},
			{"HR-DEPT-SLS", "Penjualan, Komersial & Pemasaran"},
			{"HR-DEPT-PROC", "Pengadaan & Strategic Sourcing"},
			{"HR-DEPT-MFG", "Manufaktur & Produksi SMT"},
			{"HR-DEPT-LOG", "Logistik & Manajemen Pergudangan"},
			{"HR-DEPT-QA", "Quality Assurance & Kontrol Mutu"},
			{"HR-DEPT-ENG", "Engineering, R&D & Pemeliharaan"},
		}

		hrDeptMap := make(map[string]int64)
		for _, d := range departments {
			var deptID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO hr_departments (company_id, code, name, created_at, updated_at)
				VALUES ($1, $2, $3, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					updated_at = NOW()
				RETURNING id`, companyID, d.code, d.name).Scan(&deptID)
			if err != nil {
				return fmt.Errorf("upsert hr_department %q: %w", d.code, err)
			}
			hrDeptMap[d.code] = deptID
		}

		// -------------------------------------------------------------------------
		// 2. HR Positions (15 Positions)
		// -------------------------------------------------------------------------
		type posDef struct {
			code string
			name string
		}

		positions := []posDef{
			{"POS-DIR", "Direktur Utama (Managing Director)"},
			{"POS-FIN-MGR", "Finance & Accounting Manager"},
			{"POS-ACC-SPV", "Senior Accountant & Tax Specialist"},
			{"POS-HR-MGR", "Human Resources & GA Manager"},
			{"POS-HR-OFF", "HR Generalist & Payroll Officer"},
			{"POS-SLS-MGR", "Commercial & Sales Manager"},
			{"POS-SLS-EXEC", "Senior Enterprise Sales Executive"},
			{"POS-PROC-MGR", "Procurement & Sourcing Manager"},
			{"POS-BUYER", "Purchasing & Sourcing Officer"},
			{"POS-MFG-MGR", "Manufacturing & Plant Manager"},
			{"POS-SMT-OP", "SMT Line Lead Operator & Technician"},
			{"POS-WH-SPV", "Warehouse & Inventory Supervisor"},
			{"POS-QA-MGR", "Quality Assurance & Compliance Manager"},
			{"POS-QC-INSP", "Quality Control Lead Inspector"},
			{"POS-ENG-LEAD", "Lead Hardware & Firmware R&D Engineer"},
		}

		hrPosMap := make(map[string]int64)
		for _, p := range positions {
			var posID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO hr_positions (company_id, code, name, created_at, updated_at)
				VALUES ($1, $2, $3, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					updated_at = NOW()
				RETURNING id`, companyID, p.code, p.name).Scan(&posID)
			if err != nil {
				return fmt.Errorf("upsert hr_position %q: %w", p.code, err)
			}
			hrPosMap[p.code] = posID
		}

		// -------------------------------------------------------------------------
		// 3. HR Employees (14 Indonesian Employees)
		// -------------------------------------------------------------------------
		type empDef struct {
			empNumber string
			name      string
			email     string
			deptCode  string
			posCode   string
			userEmail string
			hireDate  string
			ptkpCode  string
			salary    float64
			allowance float64
			bankCode  string
			bankAcc   string
		}

		employees := []empDef{
			{
				empNumber: "EMP-2026-001",
				name:      "Budi Santoso",
				email:     "budi.santoso@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-EXEC",
				posCode:   "POS-DIR",
				userEmail: "budi.santoso@nusantarateknik.co.id",
				hireDate:  "2026-01-01",
				ptkpCode:  "K/2",
				salary:    25000000.00,
				allowance: 3500000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3301",
			},
			{
				empNumber: "EMP-2026-002",
				name:      "Siti Aminah",
				email:     "siti.aminah@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-FIN",
				posCode:   "POS-FIN-MGR",
				userEmail: "siti.aminah@nusantarateknik.co.id",
				hireDate:  "2026-01-10",
				ptkpCode:  "K/1",
				salary:    18000000.00,
				allowance: 2500000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3302",
			},
			{
				empNumber: "EMP-2026-003",
				name:      "Arifin Nur",
				email:     "arifin.nur@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-FIN",
				posCode:   "POS-ACC-SPV",
				userEmail: "",
				hireDate:  "2026-02-01",
				ptkpCode:  "TK/0",
				salary:    10500000.00,
				allowance: 1500000.00,
				bankCode:  "MANDIRI",
				bankAcc:   "1200-4455-6601",
			},
			{
				empNumber: "EMP-2026-004",
				name:      "Rina Wulandari",
				email:     "rina.wulandari@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-HR",
				posCode:   "POS-HR-MGR",
				userEmail: "rina.wulandari@nusantarateknik.co.id",
				hireDate:  "2026-01-05",
				ptkpCode:  "K/0",
				salary:    16500000.00,
				allowance: 2000000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3304",
			},
			{
				empNumber: "EMP-2026-005",
				name:      "Maya Kartika",
				email:     "maya.kartika@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-HR",
				posCode:   "POS-HR-OFF",
				userEmail: "",
				hireDate:  "2026-02-15",
				ptkpCode:  "TK/0",
				salary:    8500000.00,
				allowance: 1200000.00,
				bankCode:  "MANDIRI",
				bankAcc:   "1200-4455-6602",
			},
			{
				empNumber: "EMP-2026-006",
				name:      "Hendra Wijaya",
				email:     "hendra.wijaya@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-SLS",
				posCode:   "POS-SLS-MGR",
				userEmail: "hendra.wijaya@nusantarateknik.co.id",
				hireDate:  "2026-01-10",
				ptkpCode:  "K/2",
				salary:    17500000.00,
				allowance: 3000000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3306",
			},
			{
				empNumber: "EMP-2026-007",
				name:      "Doni Firmansyah",
				email:     "doni.firmansyah@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-SLS",
				posCode:   "POS-SLS-EXEC",
				userEmail: "",
				hireDate:  "2026-02-01",
				ptkpCode:  "TK/1",
				salary:    9500000.00,
				allowance: 2000000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3307",
			},
			{
				empNumber: "EMP-2026-008",
				name:      "Agus Setiawan",
				email:     "agus.setiawan@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-PROC",
				posCode:   "POS-PROC-MGR",
				userEmail: "agus.setiawan@nusantarateknik.co.id",
				hireDate:  "2026-01-10",
				ptkpCode:  "K/1",
				salary:    16000000.00,
				allowance: 2000000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3308",
			},
			{
				empNumber: "EMP-2026-009",
				name:      "Joko Prasetyo",
				email:     "joko.prasetyo@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-MFG",
				posCode:   "POS-MFG-MGR",
				userEmail: "joko.prasetyo@nusantarateknik.co.id",
				hireDate:  "2026-01-05",
				ptkpCode:  "K/2",
				salary:    18500000.00,
				allowance: 2500000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3309",
			},
			{
				empNumber: "EMP-2026-010",
				name:      "Eko Supriyanto",
				email:     "eko.supriyanto@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-MFG",
				posCode:   "POS-SMT-OP",
				userEmail: "",
				hireDate:  "2026-02-10",
				ptkpCode:  "TK/0",
				salary:    7500000.00,
				allowance: 1000000.00,
				bankCode:  "BRI",
				bankAcc:   "3321-0100-2211",
			},
			{
				empNumber: "EMP-2026-011",
				name:      "Dewi Lestari",
				email:     "dewi.lestari@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-LOG",
				posCode:   "POS-WH-SPV",
				userEmail: "dewi.lestari@nusantarateknik.co.id",
				hireDate:  "2026-01-15",
				ptkpCode:  "TK/0",
				salary:    12000000.00,
				allowance: 1500000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3311",
			},
			{
				empNumber: "EMP-2026-012",
				name:      "Ratna Sari",
				email:     "ratna.sari@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-QA",
				posCode:   "POS-QA-MGR",
				userEmail: "ratna.sari@nusantarateknik.co.id",
				hireDate:  "2026-01-05",
				ptkpCode:  "K/0",
				salary:    16500000.00,
				allowance: 2000000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3312",
			},
			{
				empNumber: "EMP-2026-013",
				name:      "Tri Wahyuni",
				email:     "tri.wahyuni@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-QA",
				posCode:   "POS-QC-INSP",
				userEmail: "",
				hireDate:  "2026-02-20",
				ptkpCode:  "TK/0",
				salary:    7800000.00,
				allowance: 1000000.00,
				bankCode:  "BRI",
				bankAcc:   "3321-0100-2213",
			},
			{
				empNumber: "EMP-2026-014",
				name:      "Bambang Pamungkas",
				email:     "bambang.pamungkas@nusantarateknik.co.id",
				deptCode:  "HR-DEPT-ENG",
				posCode:   "POS-ENG-LEAD",
				userEmail: "bambang.pamungkas@nusantarateknik.co.id",
				hireDate:  "2026-01-10",
				ptkpCode:  "K/3",
				salary:    19000000.00,
				allowance: 2500000.00,
				bankCode:  "BCA",
				bankAcc:   "0088-1122-3314",
			},
		}

		empIDMap := make(map[string]int64)
		var bSantosoEmpID int64
		for _, e := range employees {
			deptID := hrDeptMap[e.deptCode]
			posID := hrPosMap[e.posCode]
			hDate := ParseDate(e.hireDate)

			var uID *int64
			if e.userEmail != "" {
				if id, ok := sctx.UserIDs[e.userEmail]; ok && id > 0 {
					uID = &id
				} else {
					var fetchedUID int64
					err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, e.userEmail).Scan(&fetchedUID)
					if err == nil && fetchedUID > 0 {
						uID = &fetchedUID
						sctx.UserIDs[e.userEmail] = fetchedUID
					}
				}
			}

			var empID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO hr_employees (
					user_id, company_id, employee_number, name, email, department_id, position_id, hire_date, status, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'ACTIVE', NOW(), NOW())
				ON CONFLICT (company_id, employee_number) DO UPDATE SET
					user_id = EXCLUDED.user_id,
					name = EXCLUDED.name,
					email = EXCLUDED.email,
					department_id = EXCLUDED.department_id,
					position_id = EXCLUDED.position_id,
					hire_date = EXCLUDED.hire_date,
					status = 'ACTIVE',
					updated_at = NOW()
				RETURNING id`,
				uID, companyID, e.empNumber, e.name, e.email, deptID, posID, hDate).Scan(&empID)
			if err != nil {
				return fmt.Errorf("upsert hr_employee %q: %w", e.empNumber, err)
			}
			empIDMap[e.empNumber] = empID
			if e.empNumber == "EMP-2026-001" {
				bSantosoEmpID = empID
			}
		}

		// Update managers hierarchy (Budi Santoso is MD, others report to department heads or MD)
		for _, e := range employees {
			empID := empIDMap[e.empNumber]
			if e.empNumber != "EMP-2026-001" {
				var mgrID = bSantosoEmpID
				if e.deptCode == "HR-DEPT-FIN" && e.empNumber != "EMP-2026-002" {
					mgrID = empIDMap["EMP-2026-002"]
				} else if e.deptCode == "HR-DEPT-HR" && e.empNumber != "EMP-2026-004" {
					mgrID = empIDMap["EMP-2026-004"]
				} else if e.deptCode == "HR-DEPT-SLS" && e.empNumber != "EMP-2026-006" {
					mgrID = empIDMap["EMP-2026-006"]
				} else if e.deptCode == "HR-DEPT-MFG" && e.empNumber != "EMP-2026-009" {
					mgrID = empIDMap["EMP-2026-009"]
				} else if e.deptCode == "HR-DEPT-QA" && e.empNumber != "EMP-2026-012" {
					mgrID = empIDMap["EMP-2026-012"]
				}

				if _, err := tx.Exec(ctx, `UPDATE hr_employees SET manager_id = $1 WHERE id = $2`, mgrID, empID); err != nil {
					return fmt.Errorf("set manager for employee %q: %w", e.empNumber, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 4. Leave Types (Annual, Sick, Maternity, Unpaid)
		// -------------------------------------------------------------------------
		leaveTypes := []struct {
			code        string
			name        string
			defaultDays float64
		}{
			{"ANNUAL", "Cuti Tahunan", 12.0},
			{"SICK", "Cuti Sakit Resmi", 14.0},
			{"MATERNITY", "Cuti Melahirkan", 90.0},
			{"UNPAID", "Cuti Di Luar Tanggungan", 0.0},
		}

		leaveTypeMap := make(map[string]int64)
		for _, lt := range leaveTypes {
			var ltID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO hr_leave_types (company_id, code, name, default_days, is_active)
				VALUES ($1, $2, $3, $4, TRUE)
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					default_days = EXCLUDED.default_days,
					is_active = TRUE
				RETURNING id`, companyID, lt.code, lt.name, lt.defaultDays).Scan(&ltID)
			if err != nil {
				return fmt.Errorf("upsert hr_leave_type %q: %w", lt.code, err)
			}
			leaveTypeMap[lt.code] = ltID
		}

		// -------------------------------------------------------------------------
		// 5. Leave Balances for 2026
		// -------------------------------------------------------------------------
		for _, e := range employees {
			empID := empIDMap[e.empNumber]
			for _, lt := range leaveTypes {
				ltID := leaveTypeMap[lt.code]
				entitled := lt.defaultDays
				used := 0.0
				pending := 0.0

				if lt.code == "ANNUAL" {
					if e.empNumber == "EMP-2026-002" || e.empNumber == "EMP-2026-008" {
						used = 2.0
					} else if e.empNumber == "EMP-2026-006" {
						pending = 3.0
					}
				} else if lt.code == "SICK" && (e.empNumber == "EMP-2026-010" || e.empNumber == "EMP-2026-014") {
					used = 1.0
				}

				if _, err := tx.Exec(ctx, `
					INSERT INTO hr_leave_balances (employee_id, leave_type_id, year, entitled, used, pending, updated_at)
					VALUES ($1, $2, 2026, $3, $4, $5, NOW())
					ON CONFLICT (employee_id, leave_type_id, year) DO UPDATE SET
						entitled = EXCLUDED.entitled,
						used = EXCLUDED.used,
						pending = EXCLUDED.pending,
						updated_at = NOW()`, empID, ltID, entitled, used, pending); err != nil {
					return fmt.Errorf("upsert leave balance for emp %q leave %q: %w", e.empNumber, lt.code, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 6. Leave Requests (8 Requests across APPROVED, PENDING, REJECTED, DRAFT)
		// -------------------------------------------------------------------------
		type leaveReqDef struct {
			empNumber string
			leaveCode string
			startDate string
			endDate   string
			days      float64
			reason    string
			status    string
		}

		leaveRequests := []leaveReqDef{
			{"EMP-2026-002", "ANNUAL", "2026-04-06", "2026-04-07", 2.0, "Cuti tahunan keperluan keluarga di Bandung", "APPROVED"},
			{"EMP-2026-008", "ANNUAL", "2026-05-14", "2026-05-15", 2.0, "Cuti perpanjangan libur Kenaikan Isa Almasih", "APPROVED"},
			{"EMP-2026-010", "SICK", "2026-06-08", "2026-06-08", 1.0, "Sakit demam & istirahat dokter (Surat Keterangan Dokter terlampir)", "APPROVED"},
			{"EMP-2026-014", "SICK", "2026-07-02", "2026-07-02", 1.0, "Pemeriksaan kesehatan rutin & flu", "APPROVED"},
			{"EMP-2026-006", "ANNUAL", "2026-08-20", "2026-08-22", 3.0, "Rencana cuti tahunan liburan keluarga", "PENDING"},
			{"EMP-2026-007", "ANNUAL", "2026-06-15", "2026-06-18", 4.0, "Pengajuan cuti saat peak closing kuartal-2", "REJECTED"},
			{"EMP-2026-003", "ANNUAL", "2026-08-28", "2026-08-28", 1.0, "Draft permohonan cuti pengurusan dokumen kependudukan", "DRAFT"},
			{"EMP-2026-011", "ANNUAL", "2026-07-20", "2026-07-21", 2.0, "Cuti tahunan keperluan keluarga mendesak", "APPROVED"},
		}

		for _, lr := range leaveRequests {
			empID := empIDMap[lr.empNumber]
			ltID := leaveTypeMap[lr.leaveCode]
			sDate := ParseDate(lr.startDate)
			eDate := ParseDate(lr.endDate)

			var exists bool
			_ = tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM hr_leave_requests
					WHERE employee_id = $1 AND start_date = $2 AND leave_type_id = $3
				)`, empID, sDate, ltID).Scan(&exists)

			if !exists {
				if _, err := tx.Exec(ctx, `
					INSERT INTO hr_leave_requests (employee_id, leave_type_id, start_date, end_date, days, reason, status, created_at, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
					empID, ltID, sDate, eDate, lr.days, lr.reason, lr.status, sDate.Add(8*time.Hour)); err != nil {
					return fmt.Errorf("insert leave request for %q: %w", lr.empNumber, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 7. Attendance Records (20 Working Days in August 2026)
		// -------------------------------------------------------------------------
		// 20 Working Days in August 2026: Aug 3-7, Aug 10-14, Aug 18-21 (Aug 17 is Independence Day), Aug 24-28
		augWorkingDays := []string{
			"2026-08-03", "2026-08-04", "2026-08-05", "2026-08-06", "2026-08-07",
			"2026-08-10", "2026-08-11", "2026-08-12", "2026-08-13", "2026-08-14",
			"2026-08-18", "2026-08-19", "2026-08-20", "2026-08-21",
			"2026-08-24", "2026-08-25", "2026-08-26", "2026-08-27", "2026-08-28",
			"2026-08-31",
		}

		for _, dateStr := range augWorkingDays {
			attDate := ParseDate(dateStr)
			for _, e := range employees {
				empID := empIDMap[e.empNumber]
				checkIn := attDate.Add(7*time.Hour + 55*time.Minute)   // 07:55 AM
				checkOut := attDate.Add(17*time.Hour + 05*time.Minute) // 17:05 PM

				if _, err := tx.Exec(ctx, `
					INSERT INTO hr_attendance (employee_id, attendance_date, check_in, check_out, status, source, created_at, updated_at)
					VALUES ($1, $2, $3, $4, 'PRESENT', 'BIOMETRIC', $3, $4)
					ON CONFLICT (employee_id, attendance_date) DO UPDATE SET
						check_in = EXCLUDED.check_in,
						check_out = EXCLUDED.check_out,
						status = 'PRESENT',
						updated_at = NOW()`,
					empID, attDate, checkIn, checkOut); err != nil {
					return fmt.Errorf("upsert attendance for %q date %s: %w", e.empNumber, dateStr, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 8. Payroll Engine Configuration (Tax & BPJS Rules, Policies, Components)
		// -------------------------------------------------------------------------
		// Look up existing rule versions inserted by migration 000046
		var taxRuleVersionID, bpjsRuleVersionID int64
		err := tx.QueryRow(ctx, `SELECT id FROM payroll_rule_versions WHERE code = 'DJP-PP58-2023'`).Scan(&taxRuleVersionID)
		if err != nil {
			return fmt.Errorf("lookup tax rule version DJP-PP58-2023: %w", err)
		}

		err = tx.QueryRow(ctx, `SELECT id FROM payroll_rule_versions WHERE code = 'BPJS-2026-03'`).Scan(&bpjsRuleVersionID)
		if err != nil {
			return fmt.Errorf("lookup bpjs rule version BPJS-2026-03: %w", err)
		}

		// Payroll Company Policy
		effPolicyDate := ParseDate("2026-01-01")
		var policyID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO payroll_company_policies (
				rule_version_id, company_id, overtime_divisor, first_hour_multiplier_bps,
				subsequent_hour_multiplier_bps, currency, rounding_unit, jkk_risk_class,
				effective_from, effective_to
			) VALUES ($1, $2, 173, 15000, 20000, 'IDR', 1, 'LOW', $3, NULL)
			ON CONFLICT (company_id, effective_from) DO UPDATE SET
				rule_version_id = EXCLUDED.rule_version_id,
				overtime_divisor = EXCLUDED.overtime_divisor,
				jkk_risk_class = EXCLUDED.jkk_risk_class
			RETURNING id`, bpjsRuleVersionID, companyID, effPolicyDate).Scan(&policyID)
		if err != nil {
			return fmt.Errorf("upsert payroll_company_policy: %w", err)
		}

		// Payroll Periods (March–August 2026)
		payrollPeriodMap := make(map[string]int64)
		for m := 3; m <= 8; m++ {
			pCode := fmt.Sprintf("2026-%02d", m)
			sDate := time.Date(2026, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
			eDate := sDate.AddDate(0, 1, -1)
			payDate := time.Date(2026, time.Month(m), 28, 0, 0, 0, 0, time.UTC)

			status := "CLOSED"
			if m == 8 {
				status = "OPEN"
			}

			var ppID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO payroll_periods (company_id, code, starts_on, ends_on, pay_date, status, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					starts_on = EXCLUDED.starts_on,
					ends_on = EXCLUDED.ends_on,
					pay_date = EXCLUDED.pay_date,
					status = EXCLUDED.status,
					updated_at = NOW()
				RETURNING id`, companyID, pCode, sDate, eDate, payDate, status).Scan(&ppID)
			if err != nil {
				return fmt.Errorf("upsert payroll_period %q: %w", pCode, err)
			}
			payrollPeriodMap[pCode] = ppID
		}

		// Payroll Components
		components := []struct {
			code     string
			name     string
			kind     string
			taxable  bool
			bpjsBase bool
		}{
			{"BASE_SALARY", "Gaji Pokok", "EARNING", true, true},
			{"POS_ALLOWANCE", "Tunjangan Jabatan & Keahlian", "EARNING", true, false},
			{"TRANS_ALLOWANCE", "Tunjangan Transport & Makan", "EARNING", true, false},
			{"COMM_ALLOWANCE", "Tunjangan Komunikasi & Pulsa", "EARNING", true, false},
			{"BPJS_HEALTH_EMP", "Iuran BPJS Kesehatan Karyawan (1%)", "DEDUCTION", false, false},
			{"BPJS_JHT_EMP", "Iuran BPJS JHT Karyawan (2%)", "DEDUCTION", false, false},
			{"BPJS_JP_EMP", "Iuran BPJS JP Karyawan (1%)", "DEDUCTION", false, false},
			{"PPH21_TAX", "Pemotongan Pajak PPh 21 TER", "DEDUCTION", false, false},
		}

		for _, c := range components {
			if _, err := tx.Exec(ctx, `
				INSERT INTO payroll_components (company_id, code, name, kind, taxable, bpjs_base, recurring, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, TRUE, NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					kind = EXCLUDED.kind,
					taxable = EXCLUDED.taxable,
					bpjs_base = EXCLUDED.bpjs_base`,
				companyID, c.code, c.name, c.kind, c.taxable, c.bpjsBase); err != nil {
				return fmt.Errorf("upsert payroll_component %q: %w", c.code, err)
			}
		}

		// Payroll Account Mappings (GL Accounts from Phase 02)
		salaryExpID := sctx.AccountIDs["5200"]
		bpjsExpID := sctx.AccountIDs["5210"]
		payrollPayableID := sctx.AccountIDs["2310"]
		pph21PayableID := sctx.AccountIDs["2220"]
		bpjsPayableID := sctx.AccountIDs["2320"]

		accMaps := []struct {
			mapType string
			accID   int64
		}{
			{"SALARY_EXPENSE", salaryExpID},
			{"EMPLOYER_BPJS_EXPENSE", bpjsExpID},
			{"PAYROLL_PAYABLE", payrollPayableID},
			{"PPH21_PAYABLE", pph21PayableID},
			{"BPJS_PAYABLE", bpjsPayableID},
		}

		for _, am := range accMaps {
			if am.accID > 0 {
				if _, err := tx.Exec(ctx, `
					INSERT INTO payroll_account_mappings (company_id, mapping_type, account_id)
					VALUES ($1, $2, $3)
					ON CONFLICT (company_id, mapping_type) DO UPDATE SET account_id = EXCLUDED.account_id`,
					companyID, am.mapType, am.accID); err != nil {
					return fmt.Errorf("upsert payroll_account_mapping %q: %w", am.mapType, err)
				}
			}
		}

		// Compensation Assignments per Employee
		effCompDate := ParseDate("2026-01-01")
		compAssignMap := make(map[string]int64)
		for _, e := range employees {
			empID := empIDMap[e.empNumber]
			var caID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO payroll_compensation_assignments (
					employee_id, base_salary, ptkp_code, bpjs_health, bpjs_employment,
					bank_code, bank_account_number, bank_account_name, effective_from, effective_to, created_at
				) VALUES ($1, $2, $3, TRUE, TRUE, $4, $5, $6, $7, NULL, NOW())
				ON CONFLICT (employee_id, effective_from) DO UPDATE SET
					base_salary = EXCLUDED.base_salary,
					ptkp_code = EXCLUDED.ptkp_code,
					bank_code = EXCLUDED.bank_code,
					bank_account_number = EXCLUDED.bank_account_number,
					bank_account_name = EXCLUDED.bank_account_name
				RETURNING id`,
				empID, e.salary, e.ptkpCode, e.bankCode, e.bankAcc, e.name, effCompDate).Scan(&caID)
			if err != nil {
				return fmt.Errorf("upsert compensation_assignment for %q: %w", e.empNumber, err)
			}
			compAssignMap[e.empNumber] = caID
		}

		// -------------------------------------------------------------------------
		// 9. Completed & POSTED Payroll Run (July 2026)
		// -------------------------------------------------------------------------
		julyPeriodID := payrollPeriodMap["2026-07"]
		runUUID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

		var runID int64
		var isPosted bool
		err = tx.QueryRow(ctx, `
			SELECT id, (status = 'POSTED') FROM payroll_runs
			WHERE company_id = $1 AND period_id = $2 AND run_type = 'REGULAR'`,
			companyID, julyPeriodID).Scan(&runID, &isPosted)

		if errors.Is(err, pgx.ErrNoRows) {
			// Insert as DRAFT first, add child lines, then transition to POSTED to satisfy immutability trigger
			err = tx.QueryRow(ctx, `
				INSERT INTO payroll_runs (
					run_uuid, company_id, period_id, run_type, tax_rule_version_id,
					bpjs_rule_version_id, company_policy_id, status, created_by,
					submitted_at, posted_at, created_at, updated_at
				) VALUES (
					$1, $2, $3, 'REGULAR', $4,
					$5, $6, 'DRAFT', $7,
					'2026-07-27 10:00:00+07', '2026-07-28 14:00:00+07', '2026-07-27 09:00:00+07', NOW()
				) RETURNING id`,
				runUUID, companyID, julyPeriodID, taxRuleVersionID, bpjsRuleVersionID, policyID, adminID).Scan(&runID)
			if err != nil {
				return fmt.Errorf("insert payroll_run for July 2026: %w", err)
			}

			// Helper for TER Category from PTKP Code
			getTERCategory := func(ptkp string) string {
				switch ptkp {
				case "TK/0", "TK/1", "K/0":
					return "A"
				case "TK/2", "K/1", "TK/3", "K/2":
					return "B"
				default:
					return "C"
				}
			}

			// Helper for TER Rate lookup in bps
			getTERRateBps := func(cat string, gross float64) int {
				if cat == "A" {
					if gross <= 5400000 {
						return 0
					} else if gross <= 5650000 {
						return 25
					} else if gross <= 5950000 {
						return 50
					} else if gross <= 6300000 {
						return 75
					} else if gross <= 6750000 {
						return 100
					} else if gross <= 7500000 {
						return 125
					} else if gross <= 8550000 {
						return 150
					} else if gross <= 9650000 {
						return 175
					} else if gross <= 10050000 {
						return 200
					} else if gross <= 10350000 {
						return 225
					} else if gross <= 10700000 {
						return 250
					} else if gross <= 11050000 {
						return 300
					} else if gross <= 11600000 {
						return 350
					} else if gross <= 12500000 {
						return 400
					} else if gross <= 13750000 {
						return 500
					} else if gross <= 15100000 {
						return 600
					} else if gross <= 16950000 {
						return 700
					} else if gross <= 19750000 {
						return 800
					} else if gross <= 24150000 {
						return 900
					} else if gross <= 26450000 {
						return 1000
					} else if gross <= 28000000 {
						return 1100
					} else {
						return 1200
					}
				} else if cat == "B" {
					if gross <= 6200000 {
						return 0
					} else if gross <= 6500000 {
						return 25
					} else if gross <= 6850000 {
						return 50
					} else if gross <= 7300000 {
						return 75
					} else if gross <= 9200000 {
						return 100
					} else if gross <= 10750000 {
						return 150
					} else if gross <= 11250000 {
						return 200
					} else if gross <= 11600000 {
						return 250
					} else if gross <= 12600000 {
						return 300
					} else if gross <= 13600000 {
						return 400
					} else if gross <= 14950000 {
						return 500
					} else if gross <= 16400000 {
						return 600
					} else if gross <= 18450000 {
						return 700
					} else if gross <= 21850000 {
						return 800
					} else if gross <= 26000000 {
						return 900
					} else if gross <= 27700000 {
						return 1000
					} else {
						return 1100
					}
				} else { // Category C
					if gross <= 6600000 {
						return 0
					} else if gross <= 6950000 {
						return 25
					} else if gross <= 7350000 {
						return 50
					} else if gross <= 7800000 {
						return 75
					} else if gross <= 8850000 {
						return 100
					} else if gross <= 9800000 {
						return 125
					} else if gross <= 10950000 {
						return 150
					} else if gross <= 11200000 {
						return 175
					} else if gross <= 12050000 {
						return 200
					} else if gross <= 12950000 {
						return 300
					} else if gross <= 14150000 {
						return 400
					} else if gross <= 15550000 {
						return 500
					} else if gross <= 17050000 {
						return 600
					} else if gross <= 19500000 {
						return 700
					} else if gross <= 22700000 {
						return 800
					} else if gross <= 26600000 {
						return 900
					} else {
						return 1000
					}
				}
			}

			var totalNetPay float64

			// Insert 14 Run Lines
			for _, e := range employees {
				empID := empIDMap[e.empNumber]
				deptID := hrDeptMap[e.deptCode]
				terCat := getTERCategory(e.ptkpCode)
				gross := e.salary + e.allowance

				// BPJS Employee Calculation
				// Health: 1% max cap 12,000,000
				healthCap := 12000000.0
				healthWage := e.salary
				if healthWage > healthCap {
					healthWage = healthCap
				}
				empHealth := healthWage * 0.01

				// JHT: 2% of base
				empJHT := e.salary * 0.02

				// JP: 1% max cap 11,086,300
				jpCap := 11086300.0
				jpWage := e.salary
				if jpWage > jpCap {
					jpWage = jpCap
				}
				empJP := jpWage * 0.01

				empBPJSTotal := empHealth + empJHT + empJP

				// Employer BPJS
				emprHealth := healthWage * 0.04
				emprJHT := e.salary * 0.037
				emprJP := jpWage * 0.02
				emprJKK := e.salary * 0.0054 // LOW risk class
				emprJKM := e.salary * 0.0030
				emprBPJSTotal := emprHealth + emprJHT + emprJP + emprJKK + emprJKM

				// PPh 21 TER
				rateBps := getTERRateBps(terCat, gross)
				pph21 := gross * float64(rateBps) / 10000.0

				netPay := gross - empBPJSTotal - pph21
				totalNetPay += netPay

				breakdownJSON, _ := json.Marshal([]map[string]interface{}{
					{"code": "BASE_SALARY", "name": "Gaji Pokok", "amount": e.salary},
					{"code": "ALLOWANCE", "name": "Tunjangan Tetap", "amount": e.allowance},
					{"code": "BPJS_HEALTH_EMP", "name": "BPJS Kesehatan (1%)", "amount": empHealth},
					{"code": "BPJS_JHT_EMP", "name": "BPJS Ketenagakerjaan JHT (2%)", "amount": empJHT},
					{"code": "BPJS_JP_EMP", "name": "BPJS Ketenagakerjaan JP (1%)", "amount": empJP},
					{"code": "PPH21_TER", "name": fmt.Sprintf("PPh 21 TER Kategori %s (%d bps)", terCat, rateBps), "amount": pph21},
				})

				var lineID int64
				err := tx.QueryRow(ctx, `
					INSERT INTO payroll_run_lines (
						run_id, employee_id, department_id, ptkp_code, ter_category,
						base_salary, allowances, overtime, thr, gross,
						employee_bpjs, employer_bpjs, pph21, other_deductions, net_pay,
						breakdown, created_at
					) VALUES (
						$1, $2, $3, $4, $5,
						$6, $7, 0, 0, $8,
						$9, $10, $11, 0, $12,
						$13, '2026-07-27 10:00:00+07'
					) RETURNING id`,
					runID, empID, deptID, e.ptkpCode, terCat,
					e.salary, e.allowance, gross,
					empBPJSTotal, emprBPJSTotal, pph21, netPay,
					breakdownJSON).Scan(&lineID)
				if err != nil {
					return fmt.Errorf("insert payroll_run_line for emp %q: %w", e.empNumber, err)
				}

				// Insert deductions detail
				deductions := []struct {
					code   string
					name   string
					amount float64
				}{
					{"BPJS_HEALTH", "BPJS Kesehatan (1%)", empHealth},
					{"BPJS_JHT", "BPJS JHT (2%)", empJHT},
					{"BPJS_JP", "BPJS Jaminan Pensiun (1%)", empJP},
					{"PPH21", "PPh 21 TER", pph21},
				}

				for _, d := range deductions {
					if d.amount > 0 {
						if _, err := tx.Exec(ctx, `
							INSERT INTO payroll_run_deductions (run_line_id, code, name, amount, employee_paid)
							VALUES ($1, $2, $3, $4, TRUE)`,
							lineID, d.code, d.name, d.amount); err != nil {
							return fmt.Errorf("insert run deduction %q for line %d: %w", d.code, lineID, err)
						}
					}
				}

				// Insert Payslip
				docKey := fmt.Sprintf("PAYSLIP-202607-%s", e.empNumber)
				hashInput := fmt.Sprintf("%d-%s-%f", lineID, docKey, netPay)
				checksum := sha256.Sum256([]byte(hashInput))
				checksumHex := hex.EncodeToString(checksum[:])

				genAt := ParseDate("2026-07-28").Add(10 * time.Hour)
				delAt := ParseDate("2026-07-28").Add(15 * time.Hour)

				if _, err := tx.Exec(ctx, `
					INSERT INTO payroll_payslips (run_line_id, document_key, checksum, generated_at, delivered_at)
					VALUES ($1, $2, $3, $4, $5)`,
					lineID, docKey, checksumHex, genAt, delAt); err != nil {
					return fmt.Errorf("insert payslip for line %d: %w", lineID, err)
				}
			}

			// Insert Payment Batch
			if _, err := tx.Exec(ctx, `
				INSERT INTO payroll_payment_batches (run_id, format, status, instruction_count, total_amount, created_at)
				VALUES ($1, 'CSV', 'PAID', $2, $3, '2026-07-28 14:30:00+07')`,
				runID, len(employees), totalNetPay); err != nil {
				return fmt.Errorf("insert payment batch for run %d: %w", runID, err)
			}

			// Update run status to POSTED
			if _, err := tx.Exec(ctx, `
				UPDATE payroll_runs
				SET status = 'POSTED', posted_at = '2026-07-28 14:00:00+07', updated_at = NOW()
				WHERE id = $1`, runID); err != nil {
				return fmt.Errorf("update payroll_run %d to POSTED: %w", runID, err)
			}
		} else if err != nil {
			return fmt.Errorf("query payroll_run: %w", err)
		}

		return nil
	})
}
