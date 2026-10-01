package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase12Projects seeds R&D and engineering telemetry projects (1 ACTIVE, 1 PLANNING),
// work breakdown project tasks, assigned project members, and billable/non-billable timesheets
// across DRAFT and APPROVED lifecycles.
func seedPhase12Projects(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 12: Projects & Timesheets", func(tx pgx.Tx) error {
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

		engLeadID := sctx.UserIDs["bambang.pamungkas@nusantarateknik.co.id"]
		if engLeadID == 0 {
			var err error
			engLeadID, err = LookupUserID(ctx, tx, "bambang.pamungkas@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["bambang.pamungkas@nusantarateknik.co.id"] = engLeadID
		}

		salesMgrID := sctx.UserIDs["hendra.wijaya@nusantarateknik.co.id"]
		if salesMgrID == 0 {
			salesMgrID = adminID
		}

		prodMgrID := sctx.UserIDs["joko.prasetyo@nusantarateknik.co.id"]
		if prodMgrID == 0 {
			prodMgrID = adminID
		}

		qaMgrID := sctx.UserIDs["ratna.sari@nusantarateknik.co.id"]
		if qaMgrID == 0 {
			qaMgrID = adminID
		}

		procMgrID := sctx.UserIDs["agus.setiawan@nusantarateknik.co.id"]
		if procMgrID == 0 {
			procMgrID = adminID
		}

		whSpvID := sctx.UserIDs["dewi.lestari@nusantarateknik.co.id"]
		if whSpvID == 0 {
			whSpvID = adminID
		}

		// -------------------------------------------------------------------------
		// 1. Projects (1 ACTIVE R&D, 1 PLANNING Expansion)
		// -------------------------------------------------------------------------
		type prjDef struct {
			code        string
			name        string
			status      string
			managerID   int64
			createdAt   string
			memberIDs   []int64
		}

		projects := []prjDef{
			{
				code:      "PRJ-IOT-SMARTMETER",
				name:      "R&D Smart Meter Telemetri Daya Fase-3 & Integrasi SCADA PLN",
				status:    "OPEN",
				managerID: engLeadID,
				createdAt: "2026-03-01 09:00:00",
				memberIDs: []int64{engLeadID, prodMgrID, qaMgrID, adminID},
			},
			{
				code:      "PRJ-EXP-TELEMETRY",
				name:      "Ekspansi Jaringan Telemetri Gateway Kawasan Industri Jawa Barat",
				status:    "DRAFT",
				managerID: salesMgrID,
				createdAt: "2026-07-15 10:00:00",
				memberIDs: []int64{salesMgrID, procMgrID, whSpvID, adminID},
			},
		}

		projectIDMap := make(map[string]int64)
		for _, p := range projects {
			cTime, _ := time.Parse("2006-01-02 15:04:05", p.createdAt)

			var prjID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO projects (company_id, code, name, currency, status, manager_id, created_by, created_at)
				VALUES ($1, $2, $3, 'IDR', $4, $5, $6, $7)
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					status = EXCLUDED.status,
					manager_id = EXCLUDED.manager_id
				RETURNING id`,
				companyID, p.code, p.name, p.status, p.managerID, adminID, cTime).Scan(&prjID)
			if err != nil {
				return fmt.Errorf("upsert project %q: %w", p.code, err)
			}
			projectIDMap[p.code] = prjID

			// Assign Project Members
			for _, mID := range p.memberIDs {
				if _, err := tx.Exec(ctx, `
					INSERT INTO project_members (project_id, user_id)
					VALUES ($1, $2)
					ON CONFLICT DO NOTHING`,
					prjID, mID); err != nil {
					return fmt.Errorf("assign member %d to project %q: %w", mID, p.code, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 2. Project Tasks (10 Work Breakdown Tasks)
		// -------------------------------------------------------------------------
		type taskDef struct {
			prjCode string
			code    string
			name    string
			status  string
		}

		tasks := []taskDef{
			// PRJ-IOT-SMARTMETER Tasks
			{"PRJ-IOT-SMARTMETER", "TSK-ENG-01", "Perancangan Skematik & Layout PCB Rev 2.0 Galvanic Isolation Front-End", "DONE"},
			{"PRJ-IOT-SMARTMETER", "TSK-ENG-02", "Pengembangan Firmware Modbus RTU & Protokol Enkripsi TLS 1.3", "DONE"},
			{"PRJ-IOT-SMARTMETER", "TSK-ENG-03", "Pengujian EMC & Imunitas Surge 4kV di Balai Pengujian Akreditasi", "IN_PROGRESS"},
			{"PRJ-IOT-SMARTMETER", "TSK-ENG-04", "Integrasi Cloud Dashboard Telemetri Daya & REST API Gateway Connector", "IN_PROGRESS"},
			{"PRJ-IOT-SMARTMETER", "TSK-ENG-05", "Uji Lapangan Pilot Deployment di Gardu Distribusi PT PLN Jawa Timur", "OPEN"},
			{"PRJ-IOT-SMARTMETER", "TSK-ENG-06", "Penyusunan Dokumentasi User Manual & Kelayakan Sertifikasi SNI / SPM", "OPEN"},

			// PRJ-EXP-TELEMETRY Tasks
			{"PRJ-EXP-TELEMETRY", "TSK-PLN-01", "Studi Kelayakan RF Coverage & Site Survey Kawasan Industri Jababeka", "DONE"},
			{"PRJ-EXP-TELEMETRY", "TSK-PLN-02", "Penyusunan RAB Anggaran & Bill of Materials Tower Telemetri", "IN_PROGRESS"},
			{"PRJ-EXP-TELEMETRY", "TSK-PLN-03", "Pengadaan Perangkat Gateway 4G & Antena Base Station 915MHz", "OPEN"},
			{"PRJ-EXP-TELEMETRY", "TSK-PLN-04", "Instalasi Menara & Commissioning Jaringan LoRaWAN Jawa Barat", "OPEN"},
		}

		taskIDMap := make(map[string]int64)
		for _, t := range tasks {
			prjID := projectIDMap[t.prjCode]

			var tskID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO project_tasks (project_id, code, name, status)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (project_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					status = EXCLUDED.status
				RETURNING id`,
				prjID, t.code, t.name, t.status).Scan(&tskID)
			if err != nil {
				return fmt.Errorf("upsert project_task %q for project %q: %w", t.code, t.prjCode, err)
			}
			taskIDMap[t.code] = tskID
		}

		// -------------------------------------------------------------------------
		// 3. Timesheets (20 Timesheet Records: Billable/Non-Billable, APPROVED/DRAFT)
		// -------------------------------------------------------------------------
		type tsDef struct {
			prjCode   string
			taskCode  string
			empEmail  string
			workDate  string
			hours     float64
			desc      string
			billable  bool
			rate      float64
			status    string
		}

		timesheetRecords := []tsDef{
			// March 2026
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-01",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-03-16",
				hours:    8.0,
				desc:     "Desain skematik front-end ADC metering ADE7758 dan proteksi isolasi TVS surge",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-01",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-03-23",
				hours:    7.5,
				desc:     "Routing 4-layer PCB power plane dan simulasi impedance matching differential pair",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-02",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-03-30",
				hours:    6.0,
				desc:     "Implementasi FreeRTOS task scheduling untuk akuisisi telemetri energi 3-fasa",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},

			// April 2026
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-02",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-04-08",
				hours:    8.0,
				desc:     "Integrasi stack Modbus RTU slave engine dan register address map standard PLN",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-02",
				empEmail: "joko.prasetyo@nusantarateknik.co.id",
				workDate: "2026-04-15",
				hours:    5.0,
				desc:     "Verifikasi prototipe SMT DFM assembly dan pengujian uji nyala PCBA Rev 2.0",
				billable: false,
				rate:     0.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-03",
				empEmail: "ratna.sari@nusantarateknik.co.id",
				workDate: "2026-04-22",
				hours:    6.5,
				desc:     "Pengujian pra-kepatuhan emisi radiasi dan ESD contact discharge 8kV di lab internal",
				billable: true,
				rate:     300000.00,
				status:   "APPROVED",
			},

			// May 2026
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-03",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-05-12",
				hours:    8.0,
				desc:     "Uji kekebalan surge immunity IEC 61000-4-5 pada tegangan impuls 4kV diferensial",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-04",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-05-20",
				hours:    7.0,
				desc:     "Integrasi MQTT broker payload format JSON telemetri tegangan, arus, THD dan cos phi",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-04",
				empEmail: "budi.santoso@nusantarateknik.co.id",
				workDate: "2026-05-27",
				hours:    4.0,
				desc:     "Review arsitektur keamanan data telemetri dan integrasi dashboard eksekutif",
				billable: false,
				rate:     0.00,
				status:   "APPROVED",
			},

			// June 2026
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-04",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-06-10",
				hours:    8.0,
				desc:     "Pengujian stress testing transmisi 1000 message/menit ke backend server Odyssey",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-05",
				empEmail: "ratna.sari@nusantarateknik.co.id",
				workDate: "2026-06-18",
				hours:    5.5,
				desc:     "Penyusunan protokol pengujian lapangan (Factory Acceptance Test) bersama tim PLN",
				billable: true,
				rate:     300000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-05",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-06-25",
				hours:    8.0,
				desc:     "Instalasi unit uji pilot di panel distribusi gardu hubung trafo 150kVA",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},

			// July 2026
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-05",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-07-08",
				hours:    6.0,
				desc:     "Evaluasi data logging akurasi energi aktif Class 0.5S selama 14 hari kontinu",
				billable: true,
				rate:     350000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-IOT-SMARTMETER",
				taskCode: "TSK-ENG-06",
				empEmail: "ratna.sari@nusantarateknik.co.id",
				workDate: "2026-07-16",
				hours:    7.0,
				desc:     "Penyusunan draft laporan uji tipe dan dossier teknis sertifikasi SNI",
				billable: true,
				rate:     300000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-EXP-TELEMETRY",
				taskCode: "TSK-PLN-01",
				empEmail: "hendra.wijaya@nusantarateknik.co.id",
				workDate: "2026-07-22",
				hours:    8.0,
				desc:     "Site survey dan pengukuran sinyal RSSI/SNR LoRaWAN 915MHz di 5 lokasi tenant Cikarang",
				billable: true,
				rate:     400000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-EXP-TELEMETRY",
				taskCode: "TSK-PLN-01",
				empEmail: "hendra.wijaya@nusantarateknik.co.id",
				workDate: "2026-07-29",
				hours:    6.5,
				desc:     "Simulasi pemetaan RF propagasi radio link budget dengan software CloudRF",
				billable: true,
				rate:     400000.00,
				status:   "APPROVED",
			},

			// August 2026
			{
				prjCode:  "PRJ-EXP-TELEMETRY",
				taskCode: "TSK-PLN-02",
				empEmail: "hendra.wijaya@nusantarateknik.co.id",
				workDate: "2026-08-05",
				hours:    7.0,
				desc:     "Penyusunan rincian anggaran biaya (RAB) infrastruktur menara dan catu daya surya",
				billable: true,
				rate:     400000.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-EXP-TELEMETRY",
				taskCode: "TSK-PLN-02",
				empEmail: "agus.setiawan@nusantarateknik.co.id",
				workDate: "2026-08-12",
				hours:    5.0,
				desc:     "Klarifikasi penawaran vendor antena fiberglass dan enclosure weatherproof IP67",
				billable: false,
				rate:     0.00,
				status:   "APPROVED",
			},
			{
				prjCode:  "PRJ-EXP-TELEMETRY",
				taskCode: "TSK-PLN-03",
				empEmail: "hendra.wijaya@nusantarateknik.co.id",
				workDate: "2026-08-19",
				hours:    6.0,
				desc:     "Finalisasi spesifikasi teknis procurement order perangkat gateway outdoor",
				billable: true,
				rate:     400000.00,
				status:   "SUBMITTED",
			},
			{
				prjCode:  "PRJ-EXP-TELEMETRY",
				taskCode: "TSK-PLN-04",
				empEmail: "bambang.pamungkas@nusantarateknik.co.id",
				workDate: "2026-08-26",
				hours:    4.5,
				desc:     "Penyusunan jadwal instalasi lapangan dan safety hazard assessment (JSA)",
				billable: true,
				rate:     350000.00,
				status:   "DRAFT",
			},
		}

		for _, ts := range timesheetRecords {
			prjID := projectIDMap[ts.prjCode]
			tskID := taskIDMap[ts.taskCode]
			wDate := ParseDate(ts.workDate)

			var empUID int64
			if id, ok := sctx.UserIDs[ts.empEmail]; ok && id > 0 {
				empUID = id
			} else {
				var fetchedUID int64
				err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, ts.empEmail).Scan(&fetchedUID)
				if err != nil {
					return fmt.Errorf("lookup user %q for timesheet: %w", ts.empEmail, err)
				}
				empUID = fetchedUID
				sctx.UserIDs[ts.empEmail] = fetchedUID
			}

			baseAmount := ts.hours * ts.rate

			var tsExists bool
			_ = tx.QueryRow(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM timesheets
					WHERE company_id = $1 AND project_id = $2 AND task_id = $3 AND employee_id = $4 AND work_date = $5
				)`, companyID, prjID, tskID, empUID, wDate).Scan(&tsExists)

			if !tsExists {
				if _, err := tx.Exec(ctx, `
					INSERT INTO timesheets (
						company_id, project_id, task_id, employee_id, work_date,
						hours, description, billable, status, created_at,
						billable_rate, base_currency, base_amount, fx_rate, fx_rate_source, fx_rate_locked_at
					) VALUES (
						$1, $2, $3, $4, $5,
						$6, $7, $8, $9, $10,
						$11, 'IDR', $12, 1.0, 'BASE', $10
					)`,
					companyID, prjID, tskID, empUID, wDate,
					ts.hours, ts.desc, ts.billable, ts.status, wDate.Add(17*time.Hour),
					ts.rate, baseAmount); err != nil {
					return fmt.Errorf("insert timesheet on %s for %q: %w", ts.workDate, ts.empEmail, err)
				}
			}
		}

		return nil
	})
}
