package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase08QMS seeds Non-Conformance Reports (NCRs), dispositions, CAPAs,
// ISO 9001 internal & supplier quality audits with findings, inspections with test results,
// supplier quality ratings, quality objectives with monthly measurements, and customer complaints.
func seedPhase08QMS(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 08: QMS Quality", func(tx pgx.Tx) error {
		companyID := sctx.CompanyNTPID
		if companyID == 0 {
			var err error
			companyID, err = LookupCompanyID(ctx, tx, "NTP-HQ")
			if err != nil {
				return err
			}
			sctx.CompanyNTPID = companyID
		}

		qaMgrID := sctx.UserIDs["ratna.sari@nusantarateknik.co.id"]
		if qaMgrID == 0 {
			var err error
			qaMgrID, err = LookupUserID(ctx, tx, "ratna.sari@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["ratna.sari@nusantarateknik.co.id"] = qaMgrID
		}

		prodMgrID := sctx.UserIDs["joko.prasetyo@nusantarateknik.co.id"]
		if prodMgrID == 0 {
			var err error
			prodMgrID, err = LookupUserID(ctx, tx, "joko.prasetyo@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["joko.prasetyo@nusantarateknik.co.id"] = prodMgrID
		}

		procMgrID := sctx.UserIDs["agus.setiawan@nusantarateknik.co.id"]
		if procMgrID == 0 {
			var err error
			procMgrID, err = LookupUserID(ctx, tx, "agus.setiawan@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["agus.setiawan@nusantarateknik.co.id"] = procMgrID
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

		maintLeadID := sctx.UserIDs["bambang.pamungkas@nusantarateknik.co.id"]
		if maintLeadID == 0 {
			var err error
			maintLeadID, err = LookupUserID(ctx, tx, "bambang.pamungkas@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["bambang.pamungkas@nusantarateknik.co.id"] = maintLeadID
		}

		// Helper to resolve supplier ID by code
		getSupID := func(code string) (int64, error) {
			if id, ok := sctx.SupplierIDs[code]; ok && id > 0 {
				return id, nil
			}
			id, err := LookupSupplierID(ctx, tx, code)
			if err != nil {
				return 0, err
			}
			sctx.SupplierIDs[code] = id
			return id, nil
		}

		// Helper to resolve customer ID by code
		getCustID := func(code string) (int64, error) {
			if id, ok := sctx.CustomerIDs[code]; ok && id > 0 {
				return id, nil
			}
			id, err := LookupCustomerID(ctx, tx, companyID, code)
			if err != nil {
				return 0, err
			}
			sctx.CustomerIDs[code] = id
			return id, nil
		}

		// -------------------------------------------------------------------------
		// 1. Non-Conformance Reports (6 NCRs Across MINOR, MAJOR, CRITICAL Severities)
		// -------------------------------------------------------------------------
		type ncrDef struct {
			number       string
			title        string
			description  string
			sourceType   string
			sourceRef    string
			category     string
			severity     string
			status       string
			detectedAt   string
			location     string
			targetClose  string
			actualClose  *string
			rootCause    string
			containment  string
			disposition  *string
			dispDesc     *string
		}

		actClose1 := "2026-03-24"
		actClose3 := "2026-05-27"
		actClose6 := "2026-08-14"

		dispRework1 := "REWORK"
		dispDesc1 := "Desolder solder bridge with hot air micro-pencil and verify 100% with 3D AOI inspection"
		dispScrap2 := "RETURN_TO_SUPPLIER"
		dispDesc2 := "Quarantine 500 pcs defective bare PCB and return to PT Jaya PCB Megatama for AP debit note replacement"
		dispRework3 := "REWORK"
		dispDesc3 := "Disassemble enclosure top cover, replace silicone O-ring gasket, re-torque to 1.2Nm and run IP67 helium leak test"
		dispUseAsIs6 := "USE_AS_IS"
		dispDesc6 := "Product hardware meets 100% calibration specs; updated carton external label with corrected 12-month calibration cert"

		ncrs := []ncrDef{
			{
				number:      "NCR-202603-001",
				title:       "Solder Bridging Defect on ESP32-S3 QFN Footprint during SMT Assembly",
				description: "Short circuit solder bridge detected between pin 14 (V_BAT) and pin 15 (GND) during automated optical inspection of Lot #202603-A.",
				sourceType:  "PRODUCTION",
				sourceRef:   "WO-202603-001",
				category:    "PROCESS",
				severity:    "MAJOR",
				status:      "CLOSED",
				detectedAt:  "2026-03-12 11:30:00",
				location:    "SMT Cleanroom Line 1 - Zone A",
				targetClose: "2026-03-25",
				actualClose: &actClose1,
				rootCause:   "Solder paste stencil aperture thickness (0.15mm) exceeded recommended 0.12mm spec, causing paste slump and bridging under 0.5mm pitch QFN package.",
				containment: "100% AOI optical re-inspection of Lot #202603-A and micro-rework on 14 affected PCBAs.",
				disposition: &dispRework1,
				dispDesc:    &dispDesc1,
			},
			{
				number:      "NCR-202604-002",
				title:       "Supplier Bare PCB Immersion Gold Layer Below Minimum Thickness",
				description: "Incoming XRF plating thickness verification of FR4 4-layer mainboards from PT Jaya PCB showed ENIG gold thickness 0.025um against IPC-4552 requirement >= 0.05um.",
				sourceType:  "SUPPLIER",
				sourceRef:   "GRN-202604-001",
				category:    "MATERIAL",
				severity:    "CRITICAL",
				status:      "DISPOSITIONED",
				detectedAt:  "2026-04-14 09:45:00",
				location:    "Incoming QC Receiving Bay - Zone B",
				targetClose: "2026-04-30",
				actualClose: nil,
				rootCause:   "Supplier chemical plating bath immersion time calibration drift caused ENIG gold thickness 0.025um against IPC Class 3 minimum of 0.05um.",
				containment: "Quarantined 500 pcs incoming bare boards in QA hold area; return shipment scheduled.",
				disposition: &dispScrap2,
				dispDesc:    &dispDesc2,
			},
			{
				number:      "NCR-202605-003",
				title:       "IP67 Die-Cast Enclosure Gasket Misalignment Causing Ingress Leakage",
				description: "Helium pressure decay test revealed 3 units out of 100 failed IP67 seal verification due to pinched silicone gasket during automated lid fastening.",
				sourceType:  "INTERNAL",
				sourceRef:   "WO-202604-002",
				category:    "PRODUCT",
				severity:    "MINOR",
				status:      "CLOSED",
				detectedAt:  "2026-05-18 14:15:00",
				location:    "Manual Assembly Workshop - Zone B",
				targetClose: "2026-05-28",
				actualClose: &actClose3,
				rootCause:   "Worn assembly torque driver bit caused uneven bolt clamping and silicone O-ring gasket pinch.",
				containment: "Replaced torque driver bits, established 1.2 Nm torque calibration check at start of each shift.",
				disposition: &dispRework3,
				dispDesc:    &dispDesc3,
			},
			{
				number:      "NCR-202606-004",
				title:       "Firmware Flash Write Failure on Quectel 4G LTE Cellular Modem Module",
				description: "Intermittent AT command timeout during automated production modem firmware flashing in RF burn-in chamber.",
				sourceType:  "INTERNAL",
				sourceRef:   "WO-202606-004",
				category:    "MATERIAL",
				severity:    "MAJOR",
				status:      "UNDER_REVIEW",
				detectedAt:  "2026-06-22 16:00:00",
				location:    "Testing & Burn-in Lab - Zone C",
				targetClose: "2026-07-15",
				actualClose: nil,
				rootCause:   "Transient voltage dip on 3.8V DC-DC power rail during high-power LTE band 3 transmission bursts under weak signal test condition.",
				containment: "Added 470uF low-ESR tantalum buffer capacitor on V_BAT supply rail.",
			},
			{
				number:      "NCR-202607-005",
				title:       "Customer Complaint: RS485 Communication Glitch in High-Voltage Substation",
				description: "PT PLN Nusantara Power reported intermittent telemetry loss on 3-Phase Smart Power Meter installed near 150kV transformer bay.",
				sourceType:  "CUSTOMER",
				sourceRef:   "CMP-202607-001",
				category:    "PRODUCT",
				severity:    "MAJOR",
				status:      "OPEN",
				detectedAt:  "2026-07-20 10:30:00",
				location:    "Customer Substation Site - Surabaya",
				targetClose: "2026-08-30",
				actualClose: nil,
				rootCause:   "Ground loop noise coupling into non-isolated RS485 differential lines in noisy substation switchgear environment.",
				containment: "Supplied external optocoupled RS485 isolation modules and updated installation technical bulletin.",
			},
			{
				number:      "NCR-202608-006",
				title:       "Incorrect Calibration Certificate Expiry Date Printed on Power Meter Packaging",
				description: "Label verification on 120 boxed units printed calibration expiry date 6 months from test date instead of standard 12 months.",
				sourceType:  "INTERNAL",
				sourceRef:   "WO-202605-003",
				category:    "DOCUMENTATION",
				severity:    "MINOR",
				status:      "CLOSED",
				detectedAt:  "2026-08-10 13:00:00",
				location:    "Final Packaging Line - Zone B",
				targetClose: "2026-08-15",
				actualClose: &actClose6,
				rootCause:   "Automated barcode packaging label printer script was configured with 6-month validity instead of 12-month interval.",
				containment: "Corrected label template parameters in ERP and relabeled 120 finished goods units.",
				disposition: &dispUseAsIs6,
				dispDesc:    &dispDesc6,
			},
		}

		ncrMap := make(map[string]int64)
		for _, n := range ncrs {
			detAt, _ := time.Parse("2006-01-02 15:04:05", n.detectedAt)
			tgtDate := ParseDate(n.targetClose)

			var actDate *time.Time
			if n.actualClose != nil {
				d := ParseDate(*n.actualClose)
				actDate = &d
			}

			var ncrID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO ncrs (
					company_id, number, title, description, source_type, source_reference,
					category, severity, status, detected_by, detected_at, detected_location,
					responsible_party_id, assigned_to, target_closure_date, actual_closure_date,
					root_cause, containment_action, created_by, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $10, $14, $15, $16, $17, $10, $11, NOW())
				ON CONFLICT (company_id, number) DO UPDATE SET
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					source_type = EXCLUDED.source_type,
					source_reference = EXCLUDED.source_reference,
					category = EXCLUDED.category,
					severity = EXCLUDED.severity,
					status = EXCLUDED.status,
					detected_location = EXCLUDED.detected_location,
					target_closure_date = EXCLUDED.target_closure_date,
					actual_closure_date = EXCLUDED.actual_closure_date,
					root_cause = EXCLUDED.root_cause,
					containment_action = EXCLUDED.containment_action,
					updated_at = NOW()
				RETURNING id`,
				companyID, n.number, n.title, n.description, n.sourceType, n.sourceRef,
				n.category, n.severity, n.status, qaMgrID, detAt, n.location,
				prodMgrID, tgtDate, actDate, n.rootCause, n.containment).Scan(&ncrID)
			if err != nil {
				return fmt.Errorf("upsert ncr %q: %w", n.number, err)
			}
			ncrMap[n.number] = ncrID

			// Insert NCR Disposition if specified
			if n.disposition != nil && n.dispDesc != nil {
				var dispExists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ncr_dispositions WHERE ncr_id = $1)`, ncrID).Scan(&dispExists)
				if !dispExists {
					if _, err := tx.Exec(ctx, `
						INSERT INTO ncr_dispositions (ncr_id, disposition_type, description, approved_by, approved_at, created_at)
						VALUES ($1, $2, $3, $4, $5, $5)`,
						ncrID, *n.disposition, *n.dispDesc, qaMgrID, detAt.Add(24*time.Hour)); err != nil {
						return fmt.Errorf("insert ncr_disposition for ncr %q: %w", n.number, err)
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 2. Corrective & Preventive Actions (4 CAPAs Linked to NCRs)
		// -------------------------------------------------------------------------
		type capaDef struct {
			number       string
			title        string
			description  string
			ncrNumber    string
			status       string
			priority     string
			method       string
			rootCause    string
			corrAction   string
			prevAction   string
			verifMethod  string
			verifResult  string
			targetDate   string
			compDate     *string
			effectDate   *string
		}

		capaComp1 := "2026-04-12"
		capaEffect1 := "2026-05-15"
		capaComp2 := "2026-05-25"
		capaEffect2 := "2026-06-30"

		capas := []capaDef{
			{
				number:      "CAPA-202603-001",
				title:       "SMT Solder Paste Stencil Design Standardization & Laser Cut Verification",
				description: "Standardize stainless steel laser-cut foil thickness from 0.15mm to 0.12mm for all fine-pitch QFN and 0402 component footprints.",
				ncrNumber:   "NCR-202603-001",
				status:      "CLOSED",
				priority:    "HIGH",
				method:      "FIVE_WHYS",
				rootCause:   "Engineering drawing revision for SMT stencil used legacy 0.15mm thickness without updating DFM guidelines for ESP32-S3 QFN package.",
				corrAction:  "Procured new 0.12mm electro-polished laser stencil with nano-coating for IoT Gateway PCB.",
				prevAction:  "Updated DFM Guideline WI-ENG-DFM-02 requiring mandatory peer review on all fine-pitch stencil CAD designs.",
				verifMethod: "3D AOI inspection of 3 consecutive production batches (300 units minimum).",
				verifResult: "Zero solder bridge defects detected across 500 units in April and May production runs.",
				targetDate:  "2026-04-15",
				compDate:    &capaComp1,
				effectDate:  &capaEffect1,
			},
			{
				number:      "CAPA-202604-002",
				title:       "Incoming PCB Supplier Quality Control Gate & XRF Plating Thickness Verification",
				description: "Implement mandatory Lot Acceptance XRF test gate for gold plating thickness on all incoming bare printed circuit boards.",
				ncrNumber:   "NCR-202604-002",
				status:      "EFFECTIVE",
				priority:    "CRITICAL",
				method:      "FISHBONE",
				rootCause:   "Supplier plating bath immersion control drift went undetected due to lack of receiving verification at NTP warehouse.",
				corrAction:  "Calibrated receiving XRF spectrometer and issued supplier corrective action request SCAR-2026-04 to PT Jaya PCB.",
				prevAction:  "Implemented mandatory Certificate of Analysis (CoA) verification and random XRF sampling gate on every bare board batch.",
				verifMethod: "XRF sampling of 5 incoming bare board lots from May to June 2026.",
				verifResult: "All 5 supplier batches demonstrated gold thickness >= 0.055um conforming to IPC-4552.",
				targetDate:  "2026-05-30",
				compDate:    &capaComp2,
				effectDate:  &capaEffect2,
			},
			{
				number:      "CAPA-202606-003",
				title:       "Power Rail Transient Stability & Decoupling Redesign for Cellular IoT Modules",
				description: "Hardware engineering redesign of power supply circuit to suppress voltage dips during 4G LTE transmission bursts.",
				ncrNumber:   "NCR-202606-004",
				status:      "IN_PROGRESS",
				priority:    "HIGH",
				method:      "FIVE_WHYS",
				rootCause:   "Original DC-DC buck converter circuit layout lacked sufficient localized bulk capacitance near the modem V_BAT power pins.",
				corrAction:  "Re-laid PCB power plane with 470uF low-ESR tantalum capacitor and upgraded power inductor to 4A saturation current.",
				prevAction:  "Updated Hardware Design Review Checklist HW-CHK-01 to require active transient load simulation on all RF power rails.",
				verifMethod: "Oscilloscope voltage ripple measurement during maximum LTE RF burst power transmission.",
				verifResult: "V_BAT voltage dip reduced from 420mV to 65mV on prototype PCBA Rev 1.1.",
				targetDate:  "2026-08-30",
				compDate:    nil,
				effectDate:  nil,
			},
			{
				number:      "CAPA-202607-004",
				title:       "RS485 Industrial EMC Immunity & Galvanic Isolation Hardware Rev 2.0",
				description: "Redesign 3-Phase Smart Power Meter RS485 communication front-end with 2.5kV galvanic optical isolation and TVS surge protection.",
				ncrNumber:   "NCR-202607-005",
				status:      "OPEN",
				priority:    "HIGH",
				method:      "FIVE_WHYS",
				rootCause:   "Non-isolated RS485 transceiver front-end was susceptible to common-mode ground transients in electrical power substations.",
				corrAction:  "Engineered daughterboard with isolated RS485 transceiver (ADM2587E) featuring integrated isolated DC-DC converter.",
				prevAction:  "Mandate IEC 61000-4-5 Level 4 surge and IEC 61000-4-4 EFT immunity testing for all industrial power meter products.",
				verifMethod: "EMC lab immunity test at 4kV surge and 2kV EFT fast transient burst.",
				verifResult: "Awaiting final certification test report from accredited test laboratory.",
				targetDate:  "2026-09-15",
				compDate:    nil,
				effectDate:  nil,
			},
		}

		for _, c := range capas {
			ncrID := ncrMap[c.ncrNumber]
			tgtDate := ParseDate(c.targetDate)

			var compDate, effDate *time.Time
			if c.compDate != nil {
				d := ParseDate(*c.compDate)
				compDate = &d
			}
			if c.effectDate != nil {
				d := ParseDate(*c.effectDate)
				effDate = &d
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO capas (
					company_id, number, title, description, source_type, source_id, source_reference,
					status, priority, owner_id, team_members, root_cause, root_cause_method,
					corrective_action, preventive_action, verification_method, verification_result,
					target_date, completion_date, effectiveness_date, created_by, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, 'NCR', $5, $6,
					$7, $8, $9, $10, $11, $12,
					$13, $14, $15, $16,
					$17, $18, $19, $9, NOW(), NOW()
				)
				ON CONFLICT (company_id, number) DO UPDATE SET
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					source_id = EXCLUDED.source_id,
					source_reference = EXCLUDED.source_reference,
					status = EXCLUDED.status,
					priority = EXCLUDED.priority,
					root_cause = EXCLUDED.root_cause,
					root_cause_method = EXCLUDED.root_cause_method,
					corrective_action = EXCLUDED.corrective_action,
					preventive_action = EXCLUDED.preventive_action,
					verification_method = EXCLUDED.verification_method,
					verification_result = EXCLUDED.verification_result,
					target_date = EXCLUDED.target_date,
					completion_date = EXCLUDED.completion_date,
					effectiveness_date = EXCLUDED.effectiveness_date,
					updated_at = NOW()`,
				companyID, c.number, c.title, c.description, ncrID, c.ncrNumber,
				c.status, c.priority, qaMgrID, []int64{qaMgrID, prodMgrID, procMgrID}, c.rootCause, c.method,
				c.corrAction, c.prevAction, c.verifMethod, c.verifResult,
				tgtDate, compDate, effDate); err != nil {
				return fmt.Errorf("upsert capa %q: %w", c.number, err)
			}
		}

		// -------------------------------------------------------------------------
		// 3. Quality Audits & Audit Findings (2 Quality Audits, 5 Findings)
		// -------------------------------------------------------------------------
		type findingDef struct {
			findingNum  string
			category    string
			clause      string
			description string
			evidence    string
			requirement string
			riskLevel   string
			status      string
			response    string
			respDueDate string
			respDate    *string
		}

		type auditDef struct {
			number       string
			title        string
			description  string
			auditType    string
			status       string
			standard     string
			scope        string
			leadAuditor  int64
			auditee      int64
			plannedStart string
			plannedEnd   string
			actualStart  string
			actualEnd    string
			reportNumber string
			reportDate   string
			findings     []findingDef
		}

		respDate1 := "2026-04-22"
		respDate2 := "2026-04-20"
		respDate4 := "2026-05-22"
		respDate5 := "2026-05-21"

		audits := []auditDef{
			{
				number:       "AUD-2026-001",
				title:        "ISO 9001:2015 Surveillance Audit — Manufacturing & Quality Operations",
				description:  "Annual surveillance internal audit verifying SMT production, calibration, traceability, and document control conformity.",
				auditType:    "INTERNAL",
				status:       "COMPLETED",
				standard:     "ISO9001",
				scope:        "SMT Assembly Cleanroom, Testing Laboratory, Calibration Registers, and Document Control.",
				leadAuditor:  qaMgrID,
				auditee:      prodMgrID,
				plannedStart: "2026-04-14",
				plannedEnd:   "2026-04-16",
				actualStart:  "2026-04-14",
				actualEnd:    "2026-04-16",
				reportNumber: "REP-ISO9001-2026-01",
				reportDate:   "2026-04-20",
				findings: []findingDef{
					{
						findingNum:  "FND-001",
						category:    "MAJOR",
						clause:      "Clause 8.5.1 Control of Production and Service Provision",
						description: "Digital torque driver on Manual Assembly Line 2 was uncalibrated past its 6-month calibration due date.",
						evidence:    "Torque driver tag #TL-TRQ-02 calibration label expired on 2026-03-31, but tool remained in active assembly use.",
						requirement: "All monitoring and measuring equipment must be calibrated at specified intervals per ISO 9001 Clause 7.1.5.",
						riskLevel:   "HIGH",
						status:      "CLOSED",
						response:    "Immediate tool quarantine, recalibrated to 1.2Nm ± 0.05Nm, added automated tool tracking in Odyssey CMMS.",
						respDueDate: "2026-04-30",
						respDate:    &respDate1,
					},
					{
						findingNum:  "FND-002",
						category:    "MINOR",
						clause:      "Clause 7.5.3 Control of Documented Information",
						description: "Work instruction WI-SMT-04 at pick-and-place station was Revision 1.1 while Master Document register listed Revision 1.2.",
						evidence:    "Laminated SOP at station displayed obsolete revision with old nozzle changer layout.",
						requirement: "Relevant versions of applicable documented information must be available at points of use.",
						riskLevel:   "MEDIUM",
						status:      "CLOSED",
						response:    "Obsolete hardcopy destroyed and replaced with current Revision 1.2 with digital document QR code.",
						respDueDate: "2026-04-25",
						respDate:    &respDate2,
					},
					{
						findingNum:  "FND-003",
						category:    "OBSERVATION",
						clause:      "Clause 7.1.5 Monitoring and Measuring Resources",
						description: "Daily ESD wrist strap test log sheets are currently recorded on paper binders rather than digitally in ERP.",
						evidence:    "Binder at Cleanroom Gowning Room entrance had 2 missing log entries during night shift.",
						requirement: "Ensure consistent recording and archiving of electrostatic discharge verification logs.",
						riskLevel:   "LOW",
						status:      "OPEN",
						response:    "Procuring wall-mounted barcode scanner tablet for digital ESD check-in at Cleanroom entry.",
						respDueDate: "2026-09-30",
						respDate:    nil,
					},
				},
			},
			{
				number:       "AUD-2026-002",
				title:        "Annual Strategic Vendor Quality System Audit — PT Jaya PCB Megatama",
				description:  "Comprehensive quality systems and process capability on-site audit of critical bare PCB fabrication partner.",
				auditType:    "SUPPLIER",
				status:       "COMPLETED",
				standard:     "ISO9001",
				scope:        "PCB Multi-Layer Lamination, Wet Chemistry Plating Tanks, Flying Probe AOI/E-Test, and Raw Material Traceability.",
				leadAuditor:  qaMgrID,
				auditee:      procMgrID,
				plannedStart: "2026-05-18",
				plannedEnd:   "2026-05-19",
				actualStart:  "2026-05-18",
				actualEnd:    "2026-05-19",
				reportNumber: "REP-SUP-JAYA-2026",
				reportDate:   "2026-05-25",
				findings: []findingDef{
					{
						findingNum:  "FND-004",
						category:    "MAJOR",
						clause:      "Clause 8.4 Control of Externally Provided Processes",
						description: "XRF plating thickness measurement calibration reference standard at supplier wet chemistry lab was overdue for annual recertification.",
						evidence:    "Gold reference standard foil block serial #REF-AU-88 had calibration certificate expired in February 2026.",
						requirement: "Measurement standards must be traceable to international or national measurement standards.",
						riskLevel:   "HIGH",
						status:      "CLOSED",
						response:    "Supplier provided accredited third-party calibration certificate from Balai Pengujian Standar dated 2026-05-22.",
						respDueDate: "2026-06-05",
						respDate:    &respDate4,
					},
					{
						findingNum:  "FND-005",
						category:    "MINOR",
						clause:      "Clause 8.5.4 Preservation of Outputs",
						description: "Vacuum-sealed packaging moisture barrier bags for finished bare boards lacked desiccant humidity indicator cards.",
						evidence:    "Lot #PCB-05 delivery box opened without standard 3-spot cobalt-free HIC card.",
						requirement: "Product packaging must preserve conformity throughout storage and transit per IPC/JEDEC J-STD-033.",
						riskLevel:   "MEDIUM",
						status:      "CLOSED",
						response:    "Supplier updated packaging SOP PKG-SOP-03 to mandate vacuum sealing with desiccant and HIC in every lot.",
						respDueDate: "2026-05-30",
						respDate:    &respDate5,
					},
				},
			},
		}

		for _, a := range audits {
			pStart := ParseDate(a.plannedStart)
			pEnd := ParseDate(a.plannedEnd)
			aStart := ParseDate(a.actualStart)
			aEnd := ParseDate(a.actualEnd)
			repDate := ParseDate(a.reportDate)

			var auditID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO audits (
					company_id, number, title, description, audit_type, status, standard,
					scope, lead_auditor_id, audit_team_ids, auditee_id,
					planned_start, planned_end, actual_start, actual_end,
					report_number, report_date, created_by, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7,
					$8, $9, $10, $11,
					$12, $13, $14, $15,
					$16, $17, $9, NOW(), NOW()
				)
				ON CONFLICT (company_id, number) DO UPDATE SET
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					audit_type = EXCLUDED.audit_type,
					status = EXCLUDED.status,
					standard = EXCLUDED.standard,
					scope = EXCLUDED.scope,
					planned_start = EXCLUDED.planned_start,
					planned_end = EXCLUDED.planned_end,
					actual_start = EXCLUDED.actual_start,
					actual_end = EXCLUDED.actual_end,
					report_number = EXCLUDED.report_number,
					report_date = EXCLUDED.report_date,
					updated_at = NOW()
				RETURNING id`,
				companyID, a.number, a.title, a.description, a.auditType, a.status, a.standard,
				a.scope, a.leadAuditor, []int64{qaMgrID, maintLeadID}, a.auditee,
				pStart, pEnd, aStart, aEnd,
				a.reportNumber, repDate).Scan(&auditID)
			if err != nil {
				return fmt.Errorf("upsert audit %q: %w", a.number, err)
			}

			// Insert Audit Findings
			for _, f := range a.findings {
				rDueDate := ParseDate(f.respDueDate)
				var rDate *time.Time
				if f.respDate != nil {
					d := ParseDate(*f.respDate)
					rDate = &d
				}

				var fExists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_findings WHERE audit_id = $1 AND finding_number = $2)`, auditID, f.findingNum).Scan(&fExists)
				if !fExists {
					if _, err := tx.Exec(ctx, `
						INSERT INTO audit_findings (
							audit_id, finding_number, category, clause, description,
							evidence, requirement, risk_level, status,
							response, response_due_date, response_date,
							assigned_to, verified_by, verified_at, created_at, updated_at
						) VALUES (
							$1, $2, $3, $4, $5,
							$6, $7, $8, $9,
							$10, $11, $12,
							$13, $14, $15, NOW(), NOW()
						)`,
						auditID, f.findingNum, f.category, f.clause, f.description,
						f.evidence, f.requirement, f.riskLevel, f.status,
						f.response, rDueDate, rDate,
						a.auditee, qaMgrID, rDate); err != nil {
						return fmt.Errorf("insert audit_finding %q for audit %q: %w", f.findingNum, a.number, err)
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 4. Quality Inspections & Characteristic Results (5 Discrete Inspections)
		// -------------------------------------------------------------------------
		type resultDef struct {
			charName   string
			expected   string
			actual     string
			conforming bool
			notes      string
		}

		type inspDef struct {
			name        string
			description string
			module      string
			status      string
			schedAt     string
			startAt     string
			compAt      *string
			results     []resultDef
		}

		compAt1 := "2026-03-08 11:30:00"
		compAt2 := "2026-04-12 14:00:00"
		compAt3 := "2026-05-10 16:00:00"
		compAt4 := "2026-06-21 10:00:00"
		compAt5 := "2026-07-22 12:00:00"

		inspections := []inspDef{
			{
				name:        "QC Masuk: Modul Seluler Quectel 4G LTE Lot #QCT-2026-03",
				description: "Inspeksi visual kemasan reel, integritas pin SMD, scan nomor IMEI, dan uji loopback RF 4G",
				module:      "PROCUREMENT",
				status:      "PASSED",
				schedAt:     "2026-03-08 08:30:00",
				startAt:     "2026-03-08 09:00:00",
				compAt:      &compAt1,
				results: []resultDef{
					{"Integritas Kemasan Tape & Reel Antistatik", "Kopla naritas pin < 0.10mm", "0.04mm rata-rata", true, "100/100 modul diperiksa visual OK"},
					{"Keterbacaan Barcode 2D Matrix IMEI & Serial", "100% terbaca pada barcode scanner", "100/100 lolos verifikasi dekode", true, "Sesuai standar GS1"},
					{"Uji Daya Pancar RF Loopback LTE Band 1/3/5/8", "Tx Power >= 21.0 dBm", "23.2 dBm terukur pada power meter", true, "Sinyal kuat dan stabil"},
				},
			},
			{
				name:        "QC Masuk: Papan Bare PCB Mainboard PT Jaya PCB Batch #PCB-04",
				description: "Verifikasi dimensi mekanikal, mask alignment, dan ketebalan lapisan emas ENIG",
				module:      "PROCUREMENT",
				status:      "FAILED",
				schedAt:     "2026-04-12 09:30:00",
				startAt:     "2026-04-12 10:00:00",
				compAt:      &compAt2,
				results: []resultDef{
					{"Dimensi Luar PCB & Toleransi Lubang Mounting", "120.0mm x 85.0mm ± 0.2mm", "120.08mm x 85.05mm", true, "Dimensi mekanis sesuai gambar CAD"},
					{"Keseragaman & Kesejajaran Solder Mask", "Bebas pinhole dan solder bridge", "Lolos kriteria IPC-A-600 Kelas 2", true, "Inspeksi optik mikroskop OK"},
					{"Ketebalan Lapisan Emas ENIG (XRF Test)", "Emas (Au) >= 0.050 um (50 nm)", "0.025 um (Gagal / Defisiensi Plat Emas)", false, "Ditolak - Terbit NCR-202604-002"},
				},
			},
			{
				name:        "QC In-Process: PCBA Gateway SMT Line 1 Lot #SMT-05",
				description: "Inspeksi volume pasta solder 3D SPI dan automated optical inspection (AOI) setelah reflow",
				module:      "MRP",
				status:      "PASSED",
				schedAt:     "2026-05-10 12:30:00",
				startAt:     "2026-05-10 13:00:00",
				compAt:      &compAt3,
				results: []resultDef{
					{"Volume & Ketinggian Deposisi Pasta Solder", "120 um ± 15 um", "118 um rata-rata SPI", true, "Deposisi pasta solder seragam"},
					{"Akurasi Penempatan Komponen & Polaritas", "Pergeseran komponen < 25 um", "12 um pergeseran maksimum", true, "Semua IC dan kapasitor polar sesuai"},
					{"Hasil Inspeksi Otomatis 3D AOI Reflow", "0 cacat solder bridge / tombstone", "0 bridge, 0 tombstone pada 150 unit", true, "100% lolos inspeksi AOI"},
				},
			},
			{
				name:        "QC Produk Jadi: Nusantara IoT Gateway Pro Uji Burn-in 24 Jam",
				description: "Pengujian keandalan termal 60°C selama 24 jam kontinu, sensitivitas LoRa, dan uji kedap air IP67",
				module:      "MRP",
				status:      "PASSED",
				schedAt:     "2026-06-20 07:30:00",
				startAt:     "2026-06-20 08:00:00",
				compAt:      &compAt4,
				results: []resultDef{
					{"Stabilitas Suhu Kamar Burn-in 60°C (24 Jam)", "0 restart / crash sistem", "100% uptime pada 300 unit uji", true, "Lolos uji keandalan termal"},
					{"Sensitivitas Penerima LoRaWAN 915MHz", "RSSI <= -136 dBm @ SF12", "-138.5 dBm terverifikasi", true, "Sensitivitas penerimaan tinggi"},
					{"Transmisi Telemetri Cloud 4G LTE", "Latency transmisi < 500 ms", "185 ms rata-rata ping ke server IoT", true, "Koneksi cloud stabil"},
					{"Uji Kebocoran Tekanan Gas Helium IP67", "Tingkat bocor < 1.0e-5 mbar*l/s", "2.4e-6 mbar*l/s", true, "Lolos standar IP67 enclosure seal"},
				},
			},
			{
				name:        "QC Pra-Pengiriman: Smart Power Meter 3-Fasa ke PT PLN Nusantara Power",
				description: "Inspeksi uji insulasi dielektrik 3kV, akurasi metering kelas 0.5S, dan segel kemasan",
				module:      "INVENTORY",
				status:      "PASSED",
				schedAt:     "2026-07-22 08:30:00",
				startAt:     "2026-07-22 09:00:00",
				compAt:      &compAt5,
				results: []resultDef{
					{"Uji Tegangan Tinggi Dielektrik 3.0kV AC (1 Menit)", "Arus bocor < 1.0 mA", "0.18 mA @ 3.0kV AC", true, "Isolasi kelistrikan sempurna"},
					{"Respon Protokol Komunikasi Modbus RS485", "100% frame CRC valid", "1000/1000 query dijawab akurat", true, "Respon telemetri instan"},
					{"Integritas Segel Garansi & Nomor Barcode", "Segel hologram utuh & terverifikasi", "100 unit tersegel rapi", true, "Siap kirim ke PT PLN"},
				},
			},
		}

		for _, insp := range inspections {
			schT, _ := time.Parse("2006-01-02 15:04:05", insp.schedAt)
			strT, _ := time.Parse("2006-01-02 15:04:05", insp.startAt)

			var cmpT *time.Time
			if insp.compAt != nil {
				t, _ := time.Parse("2006-01-02 15:04:05", *insp.compAt)
				cmpT = &t
			}

			var inspID int64
			err := tx.QueryRow(ctx, `SELECT id FROM qms_inspections WHERE company_id = $1 AND name = $2`, companyID, insp.name).Scan(&inspID)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `
					INSERT INTO qms_inspections (
						company_id, name, description, reference_module, status,
						inspector_id, scheduled_at, started_at, completed_at,
						created_by, updated_by, created_at, updated_at
					) VALUES (
						$1, $2, $3, $4, $5,
						$6, $7, $8, $9,
						$6, $6, $7, NOW()
					) RETURNING id`,
					companyID, insp.name, insp.description, insp.module, insp.status,
					qaMgrID, schT, strT, cmpT).Scan(&inspID)
			}
			if err != nil {
				return fmt.Errorf("upsert qms_inspection %q: %w", insp.name, err)
			}

			for _, res := range insp.results {
				var resExists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM qms_inspection_results WHERE inspection_id = $1 AND characteristic_name = $2)`, inspID, res.charName).Scan(&resExists)
				if !resExists {
					if _, err := tx.Exec(ctx, `
						INSERT INTO qms_inspection_results (
							company_id, inspection_id, characteristic_name, expected_value, actual_value,
							is_conforming, notes, created_by, created_at
						) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
						companyID, inspID, res.charName, res.expected, res.actual,
						res.conforming, res.notes, qaMgrID, strT); err != nil {
						return fmt.Errorf("insert qms_inspection_result for %q: %w", insp.name, err)
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 5. Supplier Quality Ratings & Vendor Audits (8 Approved Vendors)
		// -------------------------------------------------------------------------
		type supQualityDef struct {
			supCode   string
			status    string
			rating    float64
			riskLevel string
			approved  string
			expiry    string
			lastAudit string
			nextAudit string
			notes     string
		}

		supQualities := []supQualityDef{
			{"SUP-JAYA-PCB", "CONDITIONAL", 82.5, "MEDIUM", "2026-01-10", "2026-11-19", "2026-05-19", "2026-11-19", "Status kondisional pasca temuan ketebalan emas ENIG; audit requalification dijadwalkan Nov 2026"},
			{"SUP-ALU-IND", "APPROVED", 96.0, "LOW", "2026-01-10", "2027-04-10", "2026-04-10", "2027-04-10", "Kualitas die-cast enclosure aluminium sangat presisi dan lolos uji IP67"},
			{"SUP-BAT-NUS", "APPROVED", 94.5, "LOW", "2026-01-10", "2027-03-20", "2026-03-20", "2027-03-20", "Baterai Li-Ion 18650 memenuhi kapasitas mAh dan standar keselamatan UN 38.3"},
			{"SUP-KAB-MET", "APPROVED", 98.0, "LOW", "2026-01-10", "2027-02-15", "2026-02-15", "2027-02-15", "Konektor M12 tahan air dengan rating IP68 dan pin berlapis emas berkualitas tinggi"},
			{"SUP-PKG-IND", "APPROVED", 99.0, "LOW", "2026-01-10", "2027-01-10", "2026-01-10", "2027-01-10", "Kemasan karton kraft dan polyfoam berpresisi tinggi dengan daya redam jatuh kuat"},
			{"SUP-MOX-ID", "APPROVED", 97.5, "LOW", "2026-01-10", "2027-03-12", "2026-03-12", "2027-03-12", "Distributor resmi switch industrial Moxa dengan sertifikasi garansi manufaktur"},
			{"SUP-QCT-HK", "APPROVED", 98.5, "LOW", "2026-01-10", "2027-02-28", "2026-02-28", "2027-02-28", "Modul seluler global bersertifikasi FCC, CE, dan SDPPI Indonesia"},
			{"SUP-ADV-TW", "APPROVED", 99.2, "LOW", "2026-01-10", "2027-01-25", "2026-01-25", "2027-01-25", "Industrial edge computer dengan keandalan MTBF > 100,000 jam"},
		}

		for _, sq := range supQualities {
			supID, err := getSupID(sq.supCode)
			if err != nil {
				return err
			}
			appDate := ParseDate(sq.approved)
			expDate := ParseDate(sq.expiry)
			lastDate := ParseDate(sq.lastAudit)
			nextDate := ParseDate(sq.nextAudit)

			var sqID int64
			err = tx.QueryRow(ctx, `SELECT id FROM supplier_quality WHERE company_id = $1 AND supplier_id = $2`, companyID, supID).Scan(&sqID)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `
					INSERT INTO supplier_quality (
						company_id, supplier_id, status, quality_rating, risk_level,
						approved_date, expiry_date, last_audit_date, next_audit_date, notes,
						created_by, created_at, updated_at
					) VALUES (
						$1, $2, $3, $4, $5,
						$6, $7, $8, $9, $10,
						$11, NOW(), NOW()
					) RETURNING id`,
					companyID, supID, sq.status, sq.rating, sq.riskLevel,
					appDate, expDate, lastDate, nextDate, sq.notes, qaMgrID).Scan(&sqID)
			} else if err == nil {
				_, err = tx.Exec(ctx, `
					UPDATE supplier_quality
					SET status = $2, quality_rating = $3, risk_level = $4,
						approved_date = $5, expiry_date = $6, last_audit_date = $7, next_audit_date = $8, notes = $9, updated_at = NOW()
					WHERE id = $1`, sqID, sq.status, sq.rating, sq.riskLevel, appDate, expDate, lastDate, nextDate, sq.notes)
			}
			if err != nil {
				return fmt.Errorf("upsert supplier_quality for %q: %w", sq.supCode, err)
			}
		}

		// Supplier Audits
		supAudits := []struct {
			supCode  string
			auditNum string
			planDate string
			actDate  string
			score    float64
			repNum   string
		}{
			{"SUP-JAYA-PCB", "SA-2026-001", "2026-05-18", "2026-05-19", 82.5, "REP-SUP-JAYA-2026"},
			{"SUP-ALU-IND", "SA-2026-002", "2026-04-10", "2026-04-10", 96.0, "REP-SUP-ALU-2026"},
		}

		for _, sa := range supAudits {
			supID, err := getSupID(sa.supCode)
			if err != nil {
				return err
			}
			pDate := ParseDate(sa.planDate)
			aDate := ParseDate(sa.actDate)

			var saID int64
			err = tx.QueryRow(ctx, `SELECT id FROM supplier_audits WHERE company_id = $1 AND audit_number = $2`, companyID, sa.auditNum).Scan(&saID)
			if errors.Is(err, pgx.ErrNoRows) {
				_, err = tx.Exec(ctx, `
					INSERT INTO supplier_audits (
						company_id, supplier_id, audit_number, audit_type, status,
						standard, planned_date, actual_date, score, lead_auditor_id, report_number,
						created_by, created_at, updated_at
					) VALUES (
						$1, $2, $3, 'SURVEILLANCE', 'COMPLETED',
						'ISO9001', $4, $5, $6, $7, $8,
						$7, NOW(), NOW()
					)`,
					companyID, supID, sa.auditNum, pDate, aDate, sa.score, qaMgrID, sa.repNum)
			}
			if err != nil {
				return fmt.Errorf("upsert supplier_audit %q: %w", sa.auditNum, err)
			}
		}

		// -------------------------------------------------------------------------
		// 6. Quality Objectives & Monthly KPI Tracking (March – August 2026)
		// -------------------------------------------------------------------------
		type objectiveDef struct {
			name     string
			desc     string
			metric   string
			target   float64
			current  float64
			unit     string
			readings []struct {
				date  string
				value float64
				note  string
			}
		}

		objectives := []objectiveDef{
			{
				name:    "First Pass Yield (FPY) — Lini Produksi SMT & Assembly",
				desc:    "Persentase produk lolos uji pertama tanpa rework pada lini SMT dan perakitan",
				metric:  "FPY",
				target:  98.50,
				current: 98.90,
				unit:    "%",
				readings: []struct {
					date  string
					value float64
					note  string
				}{
					{"2026-03-31", 97.90, "Realisasi FPY Maret 2026"},
					{"2026-04-30", 98.20, "Realisasi FPY April 2026"},
					{"2026-05-31", 98.50, "Realisasi FPY Mei 2026"},
					{"2026-06-30", 98.70, "Realisasi FPY Juni 2026"},
					{"2026-07-31", 98.80, "Realisasi FPY Juli 2026"},
					{"2026-08-31", 98.90, "Realisasi FPY Agustus 2026"},
				},
			},
			{
				name:    "Defective Parts Per Million (DPPM) — Komponen Masuk",
				desc:    "Tingkat kecacatan rata-rata part per sejuta unit pada penerimaan bahan baku",
				metric:  "DPPM",
				target:  200.0,
				current: 145.0,
				unit:    "PPM",
				readings: []struct {
					date  string
					value float64
					note  string
				}{
					{"2026-03-31", 280.0, "Realisasi DPPM Maret 2026"},
					{"2026-04-30", 240.0, "Realisasi DPPM April 2026"},
					{"2026-05-31", 190.0, "Realisasi DPPM Mei 2026"},
					{"2026-06-30", 165.0, "Realisasi DPPM Juni 2026"},
					{"2026-07-31", 150.0, "Realisasi DPPM Juli 2026"},
					{"2026-08-31", 145.0, "Realisasi DPPM Agustus 2026"},
				},
			},
			{
				name:    "Customer RMA Return Rate & Biaya Kualitas Garansi",
				desc:    "Rasio klaim retur barang cacat dari pelanggan terhadap total volume pengiriman",
				metric:  "COQ",
				target:  0.50,
				current: 0.28,
				unit:    "%",
				readings: []struct {
					date  string
					value float64
					note  string
				}{
					{"2026-03-31", 0.45, "Rasio RMA Maret 2026"},
					{"2026-04-30", 0.40, "Rasio RMA April 2026"},
					{"2026-05-31", 0.35, "Rasio RMA Mei 2026"},
					{"2026-06-30", 0.32, "Rasio RMA Juni 2026"},
					{"2026-07-31", 0.30, "Rasio RMA Juli 2026"},
					{"2026-08-31", 0.28, "Rasio RMA Agustus 2026"},
				},
			},
			{
				name:    "Ketepatan Waktu Pengiriman Pelanggan (On-Time Delivery)",
				desc:    "Persentase pesanan penjualan yang dikirim tepat waktu sesuai komitmen tanggal pengiriman",
				metric:  "OTD",
				target:  95.00,
				current: 96.80,
				unit:    "%",
				readings: []struct {
					date  string
					value float64
					note  string
				}{
					{"2026-03-31", 94.20, "Realisasi OTD Maret 2026"},
					{"2026-04-30", 95.10, "Realisasi OTD April 2026"},
					{"2026-05-31", 95.80, "Realisasi OTD Mei 2026"},
					{"2026-06-30", 96.20, "Realisasi OTD Juni 2026"},
					{"2026-07-31", 96.50, "Realisasi OTD Juli 2026"},
					{"2026-08-31", 96.80, "Realisasi OTD Agustus 2026"},
				},
			},
		}

		startDate := ParseDate("2026-03-01")
		for _, obj := range objectives {
			var objID int64
			err := tx.QueryRow(ctx, `SELECT id FROM quality_objectives WHERE company_id = $1 AND name = $2`, companyID, obj.name).Scan(&objID)
			if errors.Is(err, pgx.ErrNoRows) {
				err = tx.QueryRow(ctx, `
					INSERT INTO quality_objectives (
						company_id, name, description, metric_type, target_value, current_value,
						unit, frequency, owner_id, status, start_date, created_at, updated_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, 'MONTHLY', $8, 'ACTIVE', $9, NOW(), NOW())
					RETURNING id`,
					companyID, obj.name, obj.desc, obj.metric, obj.target, obj.current,
					obj.unit, qaMgrID, startDate).Scan(&objID)
			} else if err == nil {
				_, err = tx.Exec(ctx, `
					UPDATE quality_objectives
					SET target_value = $2, current_value = $3, unit = $4, updated_at = NOW()
					WHERE id = $1`, objID, obj.target, obj.current, obj.unit)
			}
			if err != nil {
				return fmt.Errorf("upsert quality_objective %q: %w", obj.name, err)
			}

			for _, rd := range obj.readings {
				mDate := ParseDate(rd.date)
				var rdExists bool
				_ = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quality_objective_measurements WHERE objective_id = $1 AND measurement_date = $2)`, objID, mDate).Scan(&rdExists)
				if !rdExists {
					if _, err := tx.Exec(ctx, `
						INSERT INTO quality_objective_measurements (
							objective_id, value, measurement_date, notes, recorded_by, created_at
						) VALUES ($1, $2, $3, $4, $5, $6)`,
						objID, rd.value, mDate, rd.note, qaMgrID, mDate.Add(17*time.Hour)); err != nil {
						return fmt.Errorf("insert quality_objective_measurement for %q (%s): %w", obj.name, rd.date, err)
					}
				}
			}
		}

		// -------------------------------------------------------------------------
		// 7. Customer Complaints (2 Recorded Enterprise Quality Inquiries)
		// -------------------------------------------------------------------------
		plnCustID, err := getCustID("CUST-PLN-NUS")
		if err != nil {
			return err
		}
		pamCustID, err := getCustID("CUST-PAM-JAYA")
		if err != nil {
			return err
		}

		complaints := []struct {
			number      string
			customerID  int64
			title       string
			description string
			status      string
			severity    string
			recvAt      string
			closeAt     *string
			evidence    string
		}{
			{
				number:      "CMP-202607-001",
				customerID:  plnCustID,
				title:       "Gangguan Komunikasi RS485 pada Gardu Induk 150kV",
				description: "Terdapat kehilangan paket data telemetri intermiten pada Power Meter 3-Fasa yang dipasang berdekatan dengan trafo daya.",
				status:      "INVESTIGATING",
				severity:    "HIGH",
				recvAt:      "2026-07-18 14:00:00",
				closeAt:     nil,
				evidence:    "Hasil capture log RS485 menunjukkan noise spike 80Vpp saat pemutus tenaga (circuit breaker) beroperasi.",
			},
			{
				number:      "CMP-202608-002",
				customerID:  pamCustID,
				title:       "Kekencangan Ulir Cable Gland Sensor Air M12",
				description: "Umpan balik kemudahan instalasi di lapangan terkait torsi pengencangan baut gland konektor kabel waterproof M12.",
				status:      "CLOSED",
				severity:    "LOW",
				recvAt:      "2026-08-05 10:30:00",
				closeAt:     &actClose6,
				evidence:    "Panduan instalasi lapangan diperbarui dengan rekomendasi torsi tangan 0.8 Nm dan kunci pas 14mm.",
			},
		}

		for _, comp := range complaints {
			rAt, _ := time.Parse("2006-01-02 15:04:05", comp.recvAt)
			var cAt *time.Time
			if comp.closeAt != nil {
				d := ParseDate(*comp.closeAt)
				cAt = &d
			}

			if _, err := tx.Exec(ctx, `
				INSERT INTO customer_complaints (
					company_id, complaint_number, customer_id, title, description,
					status, severity, assigned_to, response_evidence, received_at, closed_at,
					created_by, updated_by, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9, $10, $11,
					$8, $8, $10, NOW()
				)
				ON CONFLICT (company_id, complaint_number) DO UPDATE SET
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					status = EXCLUDED.status,
					severity = EXCLUDED.severity,
					response_evidence = EXCLUDED.response_evidence,
					closed_at = EXCLUDED.closed_at,
					updated_at = NOW()`,
				companyID, comp.number, comp.customerID, comp.title, comp.description,
				comp.status, comp.severity, qaMgrID, comp.evidence, rAt, cAt); err != nil {
				return fmt.Errorf("upsert customer_complaint %q: %w", comp.number, err)
			}
		}

		// -------------------------------------------------------------------------
		// 8. QMS Inspection Plans & Active Quality Holds
		// -------------------------------------------------------------------------
		inspectionPlans := []struct {
			name   string
			desc   string
			module string
			steps  []struct {
				seq   int
				name  string
				instr string
			}
		}{
			{
				name:   "Rencana Inspeksi Standar Komponen Masuk Elektronik (Incoming QC)",
				desc:   "Prosedur baku verifikasi Certificate of Analysis, visual pin co-planarity, barcode, dan uji fungsional",
				module: "PROCUREMENT",
				steps: []struct {
					seq   int
					name  string
					instr string
				}{
					{1, "Verifikasi Dokumen Surat Jalan & CoA Pemasok", "Cocokkan nomor lot pabrik dan tanggal kadaluarsa moisture barrier bag"},
					{2, "Inspeksi Visual & Pengukuran Dimensi Kemasan", "Periksa pin coplanarity, tanda oksidasi, dan kebersihan lead frame"},
					{3, "Pengujian Karakteristik Kelistrikan / Plating", "Uji ketebalan plat emas XRF atau uji impedansi pin"},
				},
			},
			{
				name:   "Rencana Inspeksi Kualitas Perakitan PCBA Lini SMT (In-Process QC)",
				desc:   "Prosedur pengendalian kualitas pasta solder SPI, penempatan komponen, dan AOI pasca-reflow",
				module: "MRP",
				steps: []struct {
					seq   int
					name  string
					instr string
				}{
					{1, "Inspeksi 3D SPI Pasta Solder", "Pastikan volume pasta solder 100-135% dan ketinggian 120um"},
					{2, "Verifikasi Polaritas & Alignment Komponen", "Periksa polaritas kapasitor tantalum, IC notch, dan dioda"},
					{3, "Inspeksi Optik Otomatis (AOI) SMT", "Lakukan scanning 100% board untuk mendeteksi solder bridge atau tombstone"},
				},
			},
		}

		for _, ip := range inspectionPlans {
			var ipID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO qms_inspection_plans (company_id, name, description, reference_module, is_active, created_by, created_at, updated_at)
				VALUES ($1, $2, $3, $4, TRUE, $5, NOW(), NOW())
				RETURNING id`, companyID, ip.name, ip.desc, ip.module, qaMgrID).Scan(&ipID)
			if err != nil {
				return fmt.Errorf("insert qms_inspection_plan %q: %w", ip.name, err)
			}

			for _, st := range ip.steps {
				if _, err := tx.Exec(ctx, `
					INSERT INTO qms_inspection_plan_steps (plan_id, step_sequence, name, instructions, is_required, created_at)
					VALUES ($1, $2, $3, $4, TRUE, NOW())`,
					ipID, st.seq, st.name, st.instr); err != nil {
					return fmt.Errorf("insert qms_inspection_plan_step for %q seq %d: %w", ip.name, st.seq, err)
				}
			}
		}

		// Quality Hold for Quarantined Bare PCB Lot
		if _, err := tx.Exec(ctx, `
			INSERT INTO qms_holds (company_id, reference_module, reference_id, reason, status, created_by, created_at)
			VALUES ($1, 'PROCUREMENT', 4, 'Ketebalan emas ENIG di bawah standar spesifikasi (0.025um < 0.050um) pada Batch #PCB-04', 'ACTIVE', $2, '2026-04-12 14:30:00')`,
			companyID, qaMgrID); err != nil {
			return fmt.Errorf("insert qms_hold: %w", err)
		}

		return nil
	})
}
