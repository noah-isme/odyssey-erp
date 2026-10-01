package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase03CRM seeds CRM pipeline stages, leads, contacts, opportunities, activities, and timeline events.
func seedPhase03CRM(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 03: CRM", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		salesManagerID := sctx.UserIDs["hendra.wijaya@nusantarateknik.co.id"]
		if salesManagerID == 0 {
			salesManagerID = adminID
		}

		// -------------------------------------------------------------------------
		// 1. Pipeline Stages (6 Core Stages for NTP-HQ and NDM-SUB)
		// -------------------------------------------------------------------------
		type stageDef struct {
			name        string
			position    int
			stageType   string
			probability int
		}

		stages := []stageDef{
			{"Qualified", 1, "OPEN", 20},
			{"Discovery", 2, "OPEN", 40},
			{"Proposal", 3, "OPEN", 65},
			{"Negotiation", 4, "OPEN", 85},
			{"Won", 5, "WON", 100},
			{"Lost", 6, "LOST", 0},
		}

		stageIDs := make(map[int64]map[string]int64) // companyID -> stageName -> stageID

		companies := []int64{sctx.CompanyNTPID, sctx.CompanyNDMID}
		for _, compID := range companies {
			if compID == 0 {
				continue
			}
			stageIDs[compID] = make(map[string]int64)
			for _, st := range stages {
				var stageID int64
				err := tx.QueryRow(ctx, `
					INSERT INTO crm_pipeline_stages (company_id, name, position, stage_type, probability, created_at)
					VALUES ($1, $2, $3, $4, $5, '2026-03-01 08:00:00+07')
					ON CONFLICT (company_id, name) DO UPDATE SET
						position = EXCLUDED.position,
						stage_type = EXCLUDED.stage_type,
						probability = EXCLUDED.probability
					RETURNING id`,
					compID, st.name, st.position, st.stageType, st.probability).Scan(&stageID)
				if err != nil {
					return fmt.Errorf("upsert stage %q (company %d): %w", st.name, compID, err)
				}
				stageIDs[compID][st.name] = stageID
			}
		}

		// -------------------------------------------------------------------------
		// 2. Contacts (Executive & Procurement Contacts for Customers & Leads)
		// -------------------------------------------------------------------------
		type contactData struct {
			custCode string
			name     string
			email    string
			phone    string
			title    string
		}

		customerContacts := []contactData{
			{"CUST-TELKOM", "Ir. Rahmat Hidayat, M.T.", "rahmat.hidayat@telkom.co.id", "+628111987654", "VP IoT Solutions & Network Infrastructure"},
			{"CUST-PLN-NUS", "Dr. Arif Wicaksono", "arif.wicaksono@plnnusantarapower.co.id", "+628122345678", "Head of Plant Automation & SCADA"},
			{"CUST-PAM-JAYA", "Tri Wahyuni, S.T.", "tri.wahyuni@pamjaya.id", "+628133456789", "Head of SCM & Smart Metering Division"},
			{"CUST-ADARO", "Gunawan Wibisono, M.Sc.", "gunawan.wibisono@adaro.com", "+628144567890", "Director of Mining Fleet Telematics"},
			{"CUST-INDOFOOD", "Antonius Teddy Kusuma", "teddy.kusuma@icbp.indofood.co.id", "+628155678901", "Senior Procurement Manager Engineering"},
			{"CUST-ASTRA", "Hendra Saputra", "hendra.saputra@component.astra.co.id", "+628166789012", "Chief Engineer Plant 4 Automation"},
			{"CUST-JAK-PRO", "Maya Anggraini, S.T.", "maya.anggraini@jakpro.co.id", "+628177890123", "Infrastructure Procurement Lead"},
			{"CUST-PETRO", "Danang Kusumo, M.T.", "danang.kusumo@petrokimia-gresik.com", "+628188901234", "Head of Process Instrumentation"},
			{"CUST-SMART-FARM", "Suryadi Pratama", "suryadi@smartagri.co.id", "+628199012345", "Chief Technology Officer"},
			{"CUST-LOG-TRANS", "Budi Hermawan", "budi.hermawan@megatrans.co.id", "+628123456780", "Fleet Telematics & Maintenance Lead"},
		}

		contactIDsByEmail := make(map[string]int64)

		for _, c := range customerContacts {
			custID := sctx.CustomerIDs[c.custCode]
			var contactID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO crm_contacts (company_id, customer_id, name, email, phone, title, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, '2026-03-02 09:00:00+07', '2026-03-02 09:00:00+07')
				ON CONFLICT (company_id, LOWER(email)) WHERE email <> '' DO UPDATE SET
					customer_id = EXCLUDED.customer_id,
					name = EXCLUDED.name,
					phone = EXCLUDED.phone,
					title = EXCLUDED.title,
					updated_at = NOW()
				RETURNING id`,
				sctx.CompanyNTPID, custID, c.name, c.email, c.phone, c.title, salesManagerID).Scan(&contactID)
			if err != nil {
				return fmt.Errorf("upsert contact %q: %w", c.email, err)
			}
			contactIDsByEmail[c.email] = contactID
		}

		// -------------------------------------------------------------------------
		// 3. Leads (13 Leads across NEW, QUALIFIED, CONVERTED, DISQUALIFIED)
		// -------------------------------------------------------------------------
		type leadData struct {
			name         string
			organization string
			email        string
			phone        string
			source       string
			status       string
			notes        string
			custCode     string // for CONVERTED
			contactEmail string
			createdAt    string
		}

		leads := []leadData{
			// Converted Leads (Linked to real customers)
			{
				name:         "Ir. Rahmat Hidayat, M.T.",
				organization: "PT Telkom Indonesia (Persero) Tbk",
				email:        "rahmat.lead@telkom.co.id",
				phone:        "+628111987654",
				source:       "PARTNER",
				status:       "CONVERTED",
				notes:        "Inquiry for nationwide 4G/LoRaWAN IoT gateway rollout across 500 edge sub-stations.",
				custCode:     "CUST-TELKOM",
				contactEmail: "rahmat.hidayat@telkom.co.id",
				createdAt:    "2026-03-02 08:30:00",
			},
			{
				name:         "Dr. Arif Wicaksono",
				organization: "PT PLN Nusantara Power",
				email:        "arif.lead@plnnusantarapower.co.id",
				phone:        "+628122345678",
				source:       "REFERRAL",
				status:       "CONVERTED",
				notes:        "Requirement for 3-phase smart power meters and environmental telemetry in power generation plants.",
				custCode:     "CUST-PLN-NUS",
				contactEmail: "arif.wicaksono@plnnusantarapower.co.id",
				createdAt:    "2026-03-04 09:15:00",
			},
			{
				name:         "Tri Wahyuni, S.T.",
				organization: "Perumda Air Minum Jaya (PAM JAYA)",
				email:        "tri.lead@pamjaya.id",
				phone:        "+628133456789",
				source:       "WEBSITE",
				status:       "CONVERTED",
				notes:        "Water level telemetry transmitters for reservoir management and flood monitoring sensors.",
				custCode:     "CUST-PAM-JAYA",
				contactEmail: "tri.wahyuni@pamjaya.id",
				createdAt:    "2026-03-10 11:00:00",
			},
			{
				name:         "Gunawan Wibisono, M.Sc.",
				organization: "PT Adaro Energy Indonesia Tbk",
				email:        "gunawan.lead@adaro.com",
				phone:        "+628144567890",
				source:       "EXHIBITION",
				status:       "CONVERTED",
				notes:        "GPS telematics OBD-II trackers for heavy coal haulage fleets and rugged edge compute nodes.",
				custCode:     "CUST-ADARO",
				contactEmail: "gunawan.wibisono@adaro.com",
				createdAt:    "2026-03-15 14:20:00",
			},

			// Qualified Leads (Active pipeline prospects)
			{
				name:         "Dimas Anggoro, S.Pt.",
				organization: "PT Japfa Comfeed Indonesia Tbk",
				email:        "dimas.anggoro@japfacomfeed.co.id",
				phone:        "+6281234567891",
				source:       "REFERRAL",
				status:       "QUALIFIED",
				notes:        "Smart poultry telemetry with temp/humidity and ammonia gas sensors across 40 commercial closed houses.",
				contactEmail: "dimas.anggoro@japfacomfeed.co.id",
				createdAt:    "2026-04-05 10:00:00",
			},
			{
				name:         "Eko Purnomo, S.T.",
				organization: "PT Wijaya Karya (Persero) Tbk",
				email:        "eko.purnomo@wika.co.id",
				phone:        "+6281345678912",
				source:       "EXHIBITION",
				status:       "QUALIFIED",
				notes:        "Structural health telemetry node integration with solar power for precast highway flyovers.",
				contactEmail: "eko.purnomo@wika.co.id",
				createdAt:    "2026-04-12 13:45:00",
			},
			{
				name:         "Faisal Basri, M.Eng.",
				organization: "PT Pertamina Gas Negara (PGN) Tbk",
				email:        "faisal.basri@pgn.co.id",
				phone:        "+6281456789123",
				source:       "PARTNER",
				status:       "QUALIFIED",
				notes:        "Cathodic protection potential monitoring and ultrasonic level sensing for distribution pipes.",
				contactEmail: "faisal.basri@pgn.co.id",
				createdAt:    "2026-04-20 15:30:00",
			},

			// New Leads (Fresh incoming inquiries)
			{
				name:         "Irwan Prasetyo",
				organization: "PT Tri Adi Bersama (Anteraja)",
				email:        "irwan.prasetyo@anteraja.id",
				phone:        "+6281567890234",
				source:       "WEBSITE",
				status:       "NEW",
				notes:        "IoT smart locker cellular lock controllers and delivery hub fleet gateway integration.",
				contactEmail: "irwan.prasetyo@anteraja.id",
				createdAt:    "2026-07-02 09:10:00",
			},
			{
				name:         "Bambang Sujatmo, M.T.",
				organization: "PT Krakatau Steel Tbk",
				email:        "bambang.sujatmo@krakatausteel.com",
				phone:        "+6281678901345",
				source:       "EXHIBITION",
				status:       "NEW",
				notes:        "High-voltage electrical substation power analyzer and vibration sensors for hot strip mill.",
				contactEmail: "bambang.sujatmo@krakatausteel.com",
				createdAt:    "2026-07-15 11:25:00",
			},
			{
				name:         "Linda Kusuma Wardhani",
				organization: "PT Sumber Alfaria Trijaya Tbk",
				email:        "linda.kusuma@alfamart.co.id",
				phone:        "+6281789012456",
				source:       "COLD_CALL",
				status:       "NEW",
				notes:        "Energy monitoring and cold chain chiller temp data loggers for 200 distribution hub stores.",
				contactEmail: "linda.kusuma@alfamart.co.id",
				createdAt:    "2026-08-01 14:00:00",
			},

			// Disqualified Leads (Real-world filtering)
			{
				name:         "Hendra Mulyadi",
				organization: "CV Maju Mundur Elektronik",
				email:        "hendra@majumundur.com",
				phone:        "+6281890123567",
				source:       "WEBSITE",
				status:       "DISQUALIFIED",
				notes:        "Seeking DIY consumer hobbyist components in single piece quantities. Outside enterprise B2B scope.",
				contactEmail: "hendra@majumundur.com",
				createdAt:    "2026-05-10 16:00:00",
			},
			{
				name:         "Denny Setiawan",
				organization: "PT Global Solusi Digital",
				email:        "denny@globalsolusi.co.id",
				phone:        "+6281901234678",
				source:       "COLD_CALL",
				status:       "DISQUALIFIED",
				notes:        "Budget allocation cancelled due to internal corporate restructuring.",
				contactEmail: "denny@globalsolusi.co.id",
				createdAt:    "2026-05-18 10:45:00",
			},
			{
				name:         "Agus Priyono",
				organization: "PT Agro Mandiri Abadi",
				email:        "agus@agromandiri.co.id",
				phone:        "+6281123456789",
				source:       "REFERRAL",
				status:       "DISQUALIFIED",
				notes:        "Demanded legacy 2G analog hardware that is discontinued and unsupported by national telcos.",
				contactEmail: "agus@agromandiri.co.id",
				createdAt:    "2026-06-05 13:15:00",
			},
		}

		leadIDs := make(map[string]int64)

		for _, l := range leads {
			createdTime := ParseDate(l.createdAt[:10])

			// Create lead-specific contact if not existing
			var leadContactID *int64
			if cid, ok := contactIDsByEmail[l.contactEmail]; ok {
				leadContactID = &cid
			} else if l.contactEmail != "" {
				var newCID int64
				err := tx.QueryRow(ctx, `
					INSERT INTO crm_contacts (company_id, name, email, phone, title, created_by, created_at, updated_at)
					VALUES ($1, $2, $3, $4, 'Lead Contact', $5, $6, $6)
					ON CONFLICT (company_id, LOWER(email)) WHERE email <> '' DO UPDATE SET
						name = EXCLUDED.name,
						phone = EXCLUDED.phone,
						updated_at = NOW()
					RETURNING id`,
					sctx.CompanyNTPID, l.name, l.contactEmail, l.phone, salesManagerID, createdTime).Scan(&newCID)
				if err != nil {
					return fmt.Errorf("upsert lead contact %q: %w", l.contactEmail, err)
				}
				contactIDsByEmail[l.contactEmail] = newCID
				leadContactID = &newCID
			}

			var convertedCustID *int64
			if l.custCode != "" {
				if cid, ok := sctx.CustomerIDs[l.custCode]; ok {
					convertedCustID = &cid
				}
			}

			var leadID int64
			// Check if lead with this email exists
			err := tx.QueryRow(ctx, `
				SELECT id FROM crm_leads WHERE company_id = $1 AND email = $2`,
				sctx.CompanyNTPID, l.email).Scan(&leadID)
			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO crm_leads (
						company_id, owner_id, source, name, organization, email, phone,
						status, notes, converted_customer_id, converted_contact_id, created_by, created_at, updated_at
					) VALUES (
						$1, $2, $3, $4, $5, $6, $7,
						$8, $9, $10, $11, $12, $13, $13
					) RETURNING id`,
					sctx.CompanyNTPID, salesManagerID, l.source, l.name, l.organization, l.email, l.phone,
					l.status, l.notes, convertedCustID, leadContactID, salesManagerID, createdTime).Scan(&leadID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE crm_leads SET
						owner_id = $2, source = $3, name = $4, organization = $5, phone = $6,
						status = $7, notes = $8, converted_customer_id = $9, converted_contact_id = $10, updated_at = NOW()
					WHERE id = $1`,
					leadID, salesManagerID, l.source, l.name, l.organization, l.phone,
					l.status, l.notes, convertedCustID, leadContactID)
			}
			if err != nil {
				return fmt.Errorf("upsert lead %q: %w", l.email, err)
			}
			leadIDs[l.email] = leadID
		}

		// -------------------------------------------------------------------------
		// 4. Opportunities (10 Opportunities across OPEN, WON, LOST: Rp 50M - 800M)
		// -------------------------------------------------------------------------
		type oppData struct {
			name          string
			leadEmail     string
			custCode      string
			contactEmail  string
			stageName     string
			status        string
			expectedValue float64
			closeDate     string
			source        string
			winLossReason string
			createdAt     string
		}

		opps := []oppData{
			// WON Deals (Linked to converted customers)
			{
				name:          "PLN Nusantara Power Substation Vibration & Telemetry Package",
				leadEmail:     "arif.lead@plnnusantarapower.co.id",
				custCode:      "CUST-PLN-NUS",
				contactEmail:  "arif.wicaksono@plnnusantarapower.co.id",
				stageName:     "Won",
				status:        "WON",
				expectedValue: 580000000,
				closeDate:     "2026-05-20",
				source:        "REFERRAL",
				winLossReason: "Superior LoRaWAN penetration and PSAK compliance certification.",
				createdAt:     "2026-03-05",
			},
			{
				name:          "PAM Jaya District Metering Area Ultrasonic Water Level Monitoring",
				leadEmail:     "tri.lead@pamjaya.id",
				custCode:      "CUST-PAM-JAYA",
				contactEmail:  "tri.wahyuni@pamjaya.id",
				stageName:     "Won",
				status:        "WON",
				expectedValue: 420000000,
				closeDate:     "2026-04-15",
				source:        "WEBSITE",
				winLossReason: "Met IP68 immersion specs and fast 14-day delivery lead time.",
				createdAt:     "2026-03-12",
			},
			{
				name:          "Adaro Heavy Haulage Coal Fleet GPS & OBD Telematics Rollout",
				leadEmail:     "gunawan.lead@adaro.com",
				custCode:      "CUST-ADARO",
				contactEmail:  "gunawan.wibisono@adaro.com",
				stageName:     "Won",
				status:        "WON",
				expectedValue: 750000000,
				closeDate:     "2026-06-10",
				source:        "EXHIBITION",
				winLossReason: "Ruggedized die-cast enclosure survived harsh Kalimantan mine tests.",
				createdAt:     "2026-03-18",
			},

			// OPEN Deals (Active Pipeline)
			{
				name:          "Telkom Nationwide Smart Grid Edge Gateway Expansion Phase 2",
				leadEmail:     "rahmat.lead@telkom.co.id",
				custCode:      "CUST-TELKOM",
				contactEmail:  "rahmat.hidayat@telkom.co.id",
				stageName:     "Negotiation",
				status:        "OPEN",
				expectedValue: 650000000,
				closeDate:     "2026-08-31",
				source:        "PARTNER",
				winLossReason: "",
				createdAt:     "2026-04-01",
			},
			{
				name:          "Japfa Closed-House Commercial Farm Climate Control Telemetry",
				leadEmail:     "dimas.anggoro@japfacomfeed.co.id",
				contactEmail:  "dimas.anggoro@japfacomfeed.co.id",
				stageName:     "Proposal",
				status:        "OPEN",
				expectedValue: 320000000,
				closeDate:     "2026-08-25",
				source:        "REFERRAL",
				winLossReason: "",
				createdAt:     "2026-04-10",
			},
			{
				name:          "WIKA Toll Bridge Structural Vibration & Tilt Telemetry",
				leadEmail:     "eko.purnomo@wika.co.id",
				contactEmail:  "eko.purnomo@wika.co.id",
				stageName:     "Discovery",
				status:        "OPEN",
				expectedValue: 450000000,
				closeDate:     "2026-08-30",
				source:        "EXHIBITION",
				winLossReason: "",
				createdAt:     "2026-04-15",
			},
			{
				name:          "PGN Transmission Pipeline Cathodic Monitoring & Radar Level",
				leadEmail:     "faisal.basri@pgn.co.id",
				contactEmail:  "faisal.basri@pgn.co.id",
				stageName:     "Qualified",
				status:        "OPEN",
				expectedValue: 280000000,
				closeDate:     "2026-08-31",
				source:        "PARTNER",
				winLossReason: "",
				createdAt:     "2026-04-22",
			},
			{
				name:          "Astra Otoparts Plant 4 Line 2 Smart Power Metering System",
				custCode:      "CUST-ASTRA",
				contactEmail:  "hendra.saputra@component.astra.co.id",
				stageName:     "Proposal",
				status:        "OPEN",
				expectedValue: 175000000,
				closeDate:     "2026-08-20",
				source:        "EXISTING_CUSTOMER",
				winLossReason: "",
				createdAt:     "2026-05-15",
			},

			// LOST Deals (Real-world sales analysis)
			{
				name:          "Krakatau Steel Substation Power Analyzer Upgrade",
				leadEmail:     "bambang.sujatmo@krakatausteel.com",
				contactEmail:  "bambang.sujatmo@krakatausteel.com",
				stageName:     "Lost",
				status:        "LOST",
				expectedValue: 520000000,
				closeDate:     "2026-05-30",
				source:        "EXHIBITION",
				winLossReason: "Client chose existing European incumbent with 180-day vendor financing.",
				createdAt:     "2026-04-02",
			},
			{
				name:          "Indofood Factory 3 Environmental Compliance Sensor Mesh",
				custCode:      "CUST-INDOFOOD",
				contactEmail:  "teddy.kusuma@icbp.indofood.co.id",
				stageName:     "Lost",
				status:        "LOST",
				expectedValue: 95000000,
				closeDate:     "2026-07-10",
				source:        "EXISTING_CUSTOMER",
				winLossReason: "Client postponed CAPEX automation budget to FY2027.",
				createdAt:     "2026-05-02",
			},
		}

		oppIDs := make(map[string]int64)

		for _, o := range opps {
			stageID := stageIDs[sctx.CompanyNTPID][o.stageName]
			var leadID *int64
			if lid, ok := leadIDs[o.leadEmail]; ok {
				leadID = &lid
			}
			var custID *int64
			if cid, ok := sctx.CustomerIDs[o.custCode]; ok {
				custID = &cid
			}
			var contactID *int64
			if cid, ok := contactIDsByEmail[o.contactEmail]; ok {
				contactID = &cid
			}

			closeTime := ParseDate(o.closeDate)
			createdTime := ParseDate(o.createdAt)

			var oppID int64
			err := tx.QueryRow(ctx, `
				SELECT id FROM crm_opportunities WHERE company_id = $1 AND name = $2`,
				sctx.CompanyNTPID, o.name).Scan(&oppID)
			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO crm_opportunities (
						company_id, lead_id, contact_id, customer_id, owner_id, stage_id,
						name, source, expected_value, close_date, status, win_loss_reason,
						created_by, created_at, updated_at
					) VALUES (
						$1, $2, $3, $4, $5, $6,
						$7, $8, $9, $10, $11, $12,
						$13, $14, $14
					) RETURNING id`,
					sctx.CompanyNTPID, leadID, contactID, custID, salesManagerID, stageID,
					o.name, o.source, o.expectedValue, closeTime, o.status, o.winLossReason,
					salesManagerID, createdTime).Scan(&oppID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE crm_opportunities SET
						lead_id = $2, contact_id = $3, customer_id = $4, owner_id = $5, stage_id = $6,
						source = $7, expected_value = $8, close_date = $9, status = $10, win_loss_reason = $11, updated_at = NOW()
					WHERE id = $1`,
					oppID, leadID, contactID, custID, salesManagerID, stageID,
					o.source, o.expectedValue, closeTime, o.status, o.winLossReason)
			}
			if err != nil {
				return fmt.Errorf("upsert opportunity %q: %w", o.name, err)
			}
			oppIDs[o.name] = oppID
		}

		// -------------------------------------------------------------------------
		// 5. Activities (18 Activities: CALL, MEETING, EMAIL, TASK, NOTE)
		// -------------------------------------------------------------------------
		type actData struct {
			oppName      string
			leadEmail    string
			contactEmail string
			actType      string
			subject      string
			body         string
			dueDate      string
			completed    bool
		}

		activities := []actData{
			{
				oppName:      "Telkom Nationwide Smart Grid Edge Gateway Expansion Phase 2",
				contactEmail: "rahmat.hidayat@telkom.co.id",
				actType:      "MEETING",
				subject:      "Technical Architecture Review with Telkom Enterprise SCM",
				body:         "Discussed 4G fallback latency, LoRaWAN 915MHz channel plans, and DJP e-Faktur integration.",
				dueDate:      "2026-04-10 10:00:00",
				completed:    true,
			},
			{
				oppName:      "Telkom Nationwide Smart Grid Edge Gateway Expansion Phase 2",
				contactEmail: "rahmat.hidayat@telkom.co.id",
				actType:      "EMAIL",
				subject:      "Revised IoT Gateway Pro Bill of Materials & Volume Quotation",
				body:         "Sent official proposal for 500 units with 5-year hardware replacement warranty.",
				dueDate:      "2026-04-18 14:00:00",
				completed:    true,
			},
			{
				oppName:      "Telkom Nationwide Smart Grid Edge Gateway Expansion Phase 2",
				contactEmail: "rahmat.hidayat@telkom.co.id",
				actType:      "TASK",
				subject:      "Finalize SLA terms & advance payment banking guarantees",
				body:         "Coordinate with Finance (Siti Aminah) for BCA bank guarantee issuance.",
				dueDate:      "2026-08-28 16:00:00",
				completed:    false,
			},
			{
				oppName:      "PLN Nusantara Power Substation Vibration & Telemetry Package",
				contactEmail: "arif.wicaksono@plnnusantarapower.co.id",
				actType:      "MEETING",
				subject:      "On-site Demo & Field Trial at Ketintang Substation",
				body:         "Live demonstration of 3-Phase Power Meter Modbus RS485 telemetry transmission.",
				dueDate:      "2026-03-25 09:30:00",
				completed:    true,
			},
			{
				oppName:      "PLN Nusantara Power Substation Vibration & Telemetry Package",
				contactEmail: "arif.wicaksono@plnnusantarapower.co.id",
				actType:      "CALL",
				subject:      "Tender Award & Technical Acceptance Confirmation",
				body:         "Confirmed tender victory and finalized contract execution schedule.",
				dueDate:      "2026-05-18 11:15:00",
				completed:    true,
			},
			{
				oppName:      "PAM Jaya District Metering Area Ultrasonic Water Level Monitoring",
				contactEmail: "tri.wahyuni@pamjaya.id",
				actType:      "MEETING",
				subject:      "Water Level Telemetry Calibration & Ingress Protection Review",
				body:         "Reviewed IP68 testing reports from QA manager (Ratna Sari).",
				dueDate:      "2026-03-20 13:00:00",
				completed:    true,
			},
			{
				oppName:      "PAM Jaya District Metering Area Ultrasonic Water Level Monitoring",
				contactEmail: "tri.wahyuni@pamjaya.id",
				actType:      "TASK",
				subject:      "Deliver 80 Units Ultrasonic Water Level Sensors to Pejompongan Hub",
				body:         "Ensure delivery order signed and stamped by PAM Jaya receiving team.",
				dueDate:      "2026-04-14 15:00:00",
				completed:    true,
			},
			{
				oppName:      "Adaro Heavy Haulage Coal Fleet GPS & OBD Telematics Rollout",
				contactEmail: "gunawan.wibisono@adaro.com",
				actType:      "MEETING",
				subject:      "Kalimantan Open Pit Mine Environmental Stress Test Results",
				body:         "Presented vibration and temperature telemetry logs from 30-day haul truck trial.",
				dueDate:      "2026-05-05 10:30:00",
				completed:    true,
			},
			{
				oppName:      "Adaro Heavy Haulage Coal Fleet GPS & OBD Telematics Rollout",
				contactEmail: "gunawan.wibisono@adaro.com",
				actType:      "EMAIL",
				subject:      "Contract Signing & PO Issuance Notification",
				body:         "Received formal PO from Adaro Vendor Management for 200 telematics nodes.",
				dueDate:      "2026-06-08 16:45:00",
				completed:    true,
			},
			{
				oppName:      "Japfa Closed-House Commercial Farm Climate Control Telemetry",
				contactEmail: "dimas.anggoro@japfacomfeed.co.id",
				actType:      "CALL",
				subject:      "Initial Requirement Discovery & Gas Sensor Range Verification",
				body:         "Discussed ammonia gas threshold limits (0-100 ppm) and IP65 washdown durability.",
				dueDate:      "2026-04-15 14:00:00",
				completed:    true,
			},
			{
				oppName:      "Japfa Closed-House Commercial Farm Climate Control Telemetry",
				contactEmail: "dimas.anggoro@japfacomfeed.co.id",
				actType:      "EMAIL",
				subject:      "Formal Quotation for 40 Smart Environmental Farm Nodes",
				body:         "Submitted pricing breakdown with BME680 sensors and solar charging kits.",
				dueDate:      "2026-05-02 09:00:00",
				completed:    true,
			},
			{
				oppName:      "WIKA Toll Bridge Structural Vibration & Tilt Telemetry",
				contactEmail: "eko.purnomo@wika.co.id",
				actType:      "MEETING",
				subject:      "Structural Engineering Workshop & Modbus Gateway Specs",
				body:         "WIKA engineering team approved the dual-core ESP32-S3 telemetry architecture.",
				dueDate:      "2026-05-12 11:00:00",
				completed:    true,
			},
			{
				oppName:      "PGN Transmission Pipeline Cathodic Monitoring & Radar Level",
				contactEmail: "faisal.basri@pgn.co.id",
				actType:      "CALL",
				subject:      "Explosion-Proof ATEX Certification Scope Inquiry",
				body:         "Clarified ATEX Zone 2 rating requirements for gas distribution valve stations.",
				dueDate:      "2026-05-20 15:30:00",
				completed:    true,
			},
			{
				oppName:      "Astra Otoparts Plant 4 Line 2 Smart Power Metering System",
				contactEmail: "hendra.saputra@component.astra.co.id",
				actType:      "MEETING",
				subject:      "Plant 4 Energy Audit & RS485 Network Topology Walkthrough",
				body:         "Inspected 80 feeder panels in Kelapa Gading plant with plant electrical team.",
				dueDate:      "2026-06-01 10:00:00",
				completed:    true,
			},
			{
				leadEmail:    "irwan.prasetyo@anteraja.id",
				contactEmail: "irwan.prasetyo@anteraja.id",
				actType:      "CALL",
				subject:      "Initial Follow-up on Anteraja Smart Locker Controller RFP",
				body:         "Introduced NTP-GW01 gateway capabilities and remote OTA firmware updates.",
				dueDate:      "2026-07-08 14:00:00",
				completed:    true,
			},
			{
				leadEmail:    "bambang.sujatmo@krakatausteel.com",
				contactEmail: "bambang.sujatmo@krakatausteel.com",
				actType:      "CALL",
				subject:      "Krakatau Steel RFP Debriefing & Competitor Pricing Review",
				body:         "Conducted win-loss review; noted client need for extended credit terms.",
				dueDate:      "2026-06-02 11:30:00",
				completed:    true,
			},
			{
				leadEmail:    "linda.kusuma@alfamart.co.id",
				contactEmail: "linda.kusuma@alfamart.co.id",
				actType:      "TASK",
				subject:      "Prepare Cold Chain Temperature Logger POC Hardware Kit",
				body:         "Assemble 5 prototype BLE/Cellular temp beacons for Tangerang DC testing.",
				dueDate:      "2026-08-25 17:00:00",
				completed:    false,
			},
			{
				oppName:      "Indofood Factory 3 Environmental Compliance Sensor Mesh",
				contactEmail: "teddy.kusuma@icbp.indofood.co.id",
				actType:      "NOTE",
				subject:      "Project Postponement Log - Indofood FY2027 CAPEX",
				body:         "Client informed that CAPEX committee deferred all non-critical plant upgrades to Q1 2027.",
				dueDate:      "2026-07-12 16:00:00",
				completed:    true,
			},
		}

		for _, a := range activities {
			var oppID *int64
			if id, ok := oppIDs[a.oppName]; ok {
				oppID = &id
			}
			var leadID *int64
			if id, ok := leadIDs[a.leadEmail]; ok {
				leadID = &id
			}
			var contactID *int64
			if id, ok := contactIDsByEmail[a.contactEmail]; ok {
				contactID = &id
			}

			dueTime, _ := time.Parse("2006-01-02 15:04:05", a.dueDate)
			var completedAt *time.Time
			if a.completed {
				compTime := dueTime.Add(time.Hour)
				completedAt = &compTime
			}

			var actID int64
			err := tx.QueryRow(ctx, `
				SELECT id FROM crm_activities
				WHERE company_id = $1 AND subject = $2`,
				sctx.CompanyNTPID, a.subject).Scan(&actID)
			if err != nil {
				_, err = tx.Exec(ctx, `
					INSERT INTO crm_activities (
						company_id, lead_id, opportunity_id, contact_id, owner_id,
						activity_type, subject, body, due_at, completed_at,
						created_by, created_at, updated_at
					) VALUES (
						$1, $2, $3, $4, $5,
						$6, $7, $8, $9, $10,
						$11, $12, $12
					)`,
					sctx.CompanyNTPID, leadID, oppID, contactID, salesManagerID,
					a.actType, a.subject, a.body, dueTime, completedAt,
					salesManagerID, dueTime.Add(-24*time.Hour))
			}
			if err != nil {
				return fmt.Errorf("upsert activity %q: %w", a.subject, err)
			}
		}

		// -------------------------------------------------------------------------
		// 6. CRM Timeline Events (Audit & History Tracking)
		// -------------------------------------------------------------------------
		type eventData struct {
			entityType string
			entityName string
			eventType  string
			details    map[string]any
			createdAt  string
		}

		events := []eventData{
			{
				entityType: "lead",
				entityName: "rahmat.lead@telkom.co.id",
				eventType:  "LEAD_CREATED",
				details:    map[string]any{"source": "PARTNER", "notes": "Initial Telkom Smart Grid lead"},
				createdAt:  "2026-03-02 08:30:00",
			},
			{
				entityType: "lead",
				entityName: "rahmat.lead@telkom.co.id",
				eventType:  "LEAD_CONVERTED",
				details:    map[string]any{"customer_code": "CUST-TELKOM", "opportunity": "Telkom Phase 2 Expansion"},
				createdAt:  "2026-03-10 14:00:00",
			},
			{
				entityType: "opportunity",
				entityName: "PLN Nusantara Power Substation Vibration & Telemetry Package",
				eventType:  "OPPORTUNITY_CREATED",
				details:    map[string]any{"expected_value": 580000000, "stage": "Discovery"},
				createdAt:  "2026-03-05 09:00:00",
			},
			{
				entityType: "opportunity",
				entityName: "PLN Nusantara Power Substation Vibration & Telemetry Package",
				eventType:  "STAGE_CHANGED",
				details:    map[string]any{"from": "Discovery", "to": "Proposal", "probability": 65},
				createdAt:  "2026-04-01 10:00:00",
			},
			{
				entityType: "opportunity",
				entityName: "PLN Nusantara Power Substation Vibration & Telemetry Package",
				eventType:  "DEAL_WON",
				details:    map[string]any{"final_amount": 580000000, "reason": "Tender award"},
				createdAt:  "2026-05-20 16:30:00",
			},
			{
				entityType: "opportunity",
				entityName: "PAM Jaya District Metering Area Ultrasonic Water Level Monitoring",
				eventType:  "DEAL_WON",
				details:    map[string]any{"final_amount": 420000000, "lead_time_days": 14},
				createdAt:  "2026-04-15 15:00:00",
			},
			{
				entityType: "opportunity",
				entityName: "Adaro Heavy Haulage Coal Fleet GPS & OBD Telematics Rollout",
				eventType:  "DEAL_WON",
				details:    map[string]any{"final_amount": 750000000, "fleet_size": 200},
				createdAt:  "2026-06-10 11:00:00",
			},
			{
				entityType: "opportunity",
				entityName: "Krakatau Steel Substation Power Analyzer Upgrade",
				eventType:  "DEAL_LOST",
				details:    map[string]any{"competitor": "European OEM", "reason": "Payment terms 180 days"},
				createdAt:  "2026-05-30 17:00:00",
			},
		}

		for _, ev := range events {
			var entityID int64
			if ev.entityType == "lead" {
				entityID = leadIDs[ev.entityName]
			} else if ev.entityType == "opportunity" {
				entityID = oppIDs[ev.entityName]
			}
			if entityID == 0 {
				continue
			}

			payload, _ := json.Marshal(ev.details)
			evTime, _ := time.Parse("2006-01-02 15:04:05", ev.createdAt)

			_, err := tx.Exec(ctx, `
				INSERT INTO crm_events (company_id, entity_type, entity_id, event_type, actor_id, details, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				sctx.CompanyNTPID, ev.entityType, entityID, ev.eventType, salesManagerID, payload, evTime)
			if err != nil {
				return fmt.Errorf("insert crm_event: %w", err)
			}
		}

		return nil
	})
}
