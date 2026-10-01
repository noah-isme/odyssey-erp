package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase07CMMS seeds physical plant locations, maintainable assets, task templates,
// preventive maintenance schedules, spare parts catalog, work orders, tasks, spare parts usage,
// and historical meter readings.
func seedPhase07CMMS(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 07: CMMS Maintenance", func(tx pgx.Tx) error {
		companyID := sctx.CompanyNTPID
		if companyID == 0 {
			var err error
			companyID, err = LookupCompanyID(ctx, tx, "NTP-HQ")
			if err != nil {
				return err
			}
			sctx.CompanyNTPID = companyID
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

		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			var err error
			adminID, err = LookupUserID(ctx, tx, "budi.santoso@nusantarateknik.co.id")
			if err != nil {
				return err
			}
			sctx.UserIDs["budi.santoso@nusantarateknik.co.id"] = adminID
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

		// -------------------------------------------------------------------------
		// 1. Physical Plant Locations Hierarchy
		// -------------------------------------------------------------------------
		type locationDef struct {
			code        string
			name        string
			description string
			parentCode  string
			address     string
			lat         float64
			lng         float64
		}

		locations := []locationDef{
			{
				code:        "LOC-JKT-PLANT",
				name:        "Pabrik & Kantor Sentral Pulogadung",
				description: "Kawasan Industri Pulogadung Main Manufacturing Plant & HQ",
				parentCode:  "",
				address:     "Jl. Rawa Gelam IV No. 8, Jakarta Timur 13930",
				lat:         -6.1950,
				lng:         106.9120,
			},
			{
				code:        "LOC-JKT-SMT",
				name:        "Lantai Produksi SMT (Cleanroom ISO 8)",
				description: "SMT Surface Mount Cleanroom Floor - Zone A",
				parentCode:  "LOC-JKT-PLANT",
				address:     "Jl. Rawa Gelam IV No. 8, Gedung A Lt. 1, Jakarta Timur",
				lat:         -6.1951,
				lng:         106.9121,
			},
			{
				code:        "LOC-JKT-ASY",
				name:        "Workshop Sub-Perakitan & Penyolderan",
				description: "Manual Assembly, Wave Soldering & CNC Machining Workshop - Zone B",
				parentCode:  "LOC-JKT-PLANT",
				address:     "Jl. Rawa Gelam IV No. 8, Gedung A Lt. 2, Jakarta Timur",
				lat:         -6.1952,
				lng:         106.9122,
			},
			{
				code:        "LOC-JKT-LAB",
				name:        "Laboratorium Pengujian QA & Kamar Burn-in",
				description: "Environmental Testing & 24h Burn-in Chamber Lab - Zone C",
				parentCode:  "LOC-JKT-PLANT",
				address:     "Jl. Rawa Gelam IV No. 8, Gedung B Lt. 1, Jakarta Timur",
				lat:         -6.1953,
				lng:         106.9123,
			},
			{
				code:        "LOC-JKT-UTL",
				name:        "Ruang Utilitas, HVAC Chiller & Gardu Trafo",
				description: "Plant Utilities, Power Substation & Chiller Plant Room",
				parentCode:  "LOC-JKT-PLANT",
				address:     "Jl. Rawa Gelam IV No. 8, Ruang Utilitas Selatan, Jakarta Timur",
				lat:         -6.1954,
				lng:         106.9124,
			},
			{
				code:        "LOC-JKT-FLT",
				name:        "Loading Dock & Pool Kendaraan Logistik",
				description: "Warehouse Dispatch Bay & Logistics Fleet Staging",
				parentCode:  "LOC-JKT-PLANT",
				address:     "Jl. Rawa Gelam IV No. 8, Area Parkir Barat, Jakarta Timur",
				lat:         -6.1955,
				lng:         106.9125,
			},
			{
				code:        "LOC-SBY-DIST",
				name:        "Fasilitas Distribusi Surabaya Rungkut",
				description: "East Java Distribution Hub & Regional Spares Depot",
				parentCode:  "",
				address:     "Kawasan Industri Rungkut, Jl. Rungkut Industri Raya No. 45, Surabaya",
				lat:         -7.3320,
				lng:         112.7680,
			},
			{
				code:        "LOC-CKR-DIST",
				name:        "Gudang Sentral Distribusi Cikarang",
				description: "Jababeka Logistics Hub & Component Buffer Depot",
				parentCode:  "",
				address:     "Kawasan Industri Jababeka II, Jl. Industri Selatan Blok JJ No. 12, Cikarang",
				lat:         -6.3120,
				lng:         107.1420,
			},
		}

		locMap := make(map[string]int64)
		for _, l := range locations {
			var parentID *int64
			if l.parentCode != "" {
				if pid, ok := locMap[l.parentCode]; ok {
					parentID = &pid
				}
			}

			var locID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO locations (company_id, code, name, description, parent_id, address, gps_lat, gps_lng, active, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					description = EXCLUDED.description,
					parent_id = EXCLUDED.parent_id,
					address = EXCLUDED.address,
					gps_lat = EXCLUDED.gps_lat,
					gps_lng = EXCLUDED.gps_lng,
					active = TRUE,
					updated_at = NOW()
				RETURNING id`, companyID, l.code, l.name, l.description, parentID, l.address, l.lat, l.lng).Scan(&locID)
			if err != nil {
				return fmt.Errorf("upsert location %q: %w", l.code, err)
			}
			locMap[l.code] = locID
		}

		// -------------------------------------------------------------------------
		// 2. Maintainable Assets (9 Maintainable Assets with Criticality Ratings)
		// -------------------------------------------------------------------------
		type assetDef struct {
			code         string
			name         string
			description  string
			assetType    string
			criticality  string
			manufacturer string
			model        string
			serialNumber string
			tagNumber    string
			installDate  string
			warrantyExp  string
			locCode      string
		}

		assets := []assetDef{
			{
				code:         "AST-SMT-PNP01",
				name:         "Mesin SMT Pick-and-Place Yamaha YSM20R",
				description:  "High-Speed Modular Surface Mount Placement Machine 90,000 CPH",
				assetType:    "EQUIPMENT",
				criticality:  "A",
				manufacturer: "Yamaha Motor Co., Ltd.",
				model:        "YSM20R",
				serialNumber: "YMH-2023-8821",
				tagNumber:    "TAG-SMT-001",
				installDate:  "2024-01-15",
				warrantyExp:  "2027-01-15",
				locCode:      "LOC-JKT-SMT",
			},
			{
				code:         "AST-SMT-RFO01",
				name:         "Oven Reflow Nitrogen Heller 1809 MK5",
				description:  "9-Zone Forced Convection Nitrogen Reflow Soldering Oven",
				assetType:    "EQUIPMENT",
				criticality:  "A",
				manufacturer: "Heller Industries, Inc.",
				model:        "1809 MK5",
				serialNumber: "HLR-2023-4412",
				tagNumber:    "TAG-SMT-002",
				installDate:  "2024-01-20",
				warrantyExp:  "2027-01-20",
				locCode:      "LOC-JKT-SMT",
			},
			{
				code:         "AST-MCH-CNC01",
				name:         "Mesin CNC Milling Haas VF-2SS Super-Speed",
				description:  "4-Axis High Precision CNC Vertical Machining Center for Enclosures",
				assetType:    "EQUIPMENT",
				criticality:  "B",
				manufacturer: "Haas Automation, Inc.",
				model:        "VF-2SS",
				serialNumber: "HAA-2022-9931",
				tagNumber:    "TAG-MCH-001",
				installDate:  "2023-06-10",
				warrantyExp:  "2025-06-10",
				locCode:      "LOC-JKT-ASY",
			},
			{
				code:         "AST-SMT-WAV01",
				name:         "Mesin Wave Soldering Electrovert Electra",
				description:  "Dual Wave Lead-Free Soldering System with Nitrogen Tunnel",
				assetType:    "EQUIPMENT",
				criticality:  "A",
				manufacturer: "ITW EAE Electrovert",
				model:        "Electra 600",
				serialNumber: "WAV-2023-1102",
				tagNumber:    "TAG-ASY-001",
				installDate:  "2024-02-01",
				warrantyExp:  "2026-02-01",
				locCode:      "LOC-JKT-ASY",
			},
			{
				code:         "AST-TST-OSC01",
				name:         "Digital Oscilloscope Keysight Infiniium MXR",
				description:  "8-Channel 2GHz 16GSa/s Real-Time Digital Storage Oscilloscope",
				assetType:    "TOOL",
				criticality:  "B",
				manufacturer: "Keysight Technologies",
				model:        "MXR608A",
				serialNumber: "KYS-2024-7719",
				tagNumber:    "TAG-LAB-001",
				installDate:  "2024-03-01",
				warrantyExp:  "2027-03-01",
				locCode:      "LOC-JKT-LAB",
			},
			{
				code:         "AST-FAC-CHL01",
				name:         "Chiller Sentrifugal Daikin 150TR & HVAC",
				description:  "Industrial Water-Cooled Centrifugal Chiller with Cleanroom HVAC",
				assetType:    "FACILITY",
				criticality:  "A",
				manufacturer: "Daikin Applied",
				model:        "EWWD150VZ",
				serialNumber: "DKN-2022-5501",
				tagNumber:    "TAG-UTL-001",
				installDate:  "2023-04-15",
				warrantyExp:  "2028-04-15",
				locCode:      "LOC-JKT-UTL",
			},
			{
				code:         "AST-FLT-TRK01",
				name:         "Truk Box Isuzu Giga FVR 240PS",
				description:  "Heavy-Duty Box Cargo Commercial Truck (Plat: B 9182 UEV)",
				assetType:    "VEHICLE",
				criticality:  "B",
				manufacturer: "PT Isuzu Astra Motor Indonesia",
				model:        "Giga FVR 34 P",
				serialNumber: "B-9182-UEV",
				tagNumber:    "TAG-FLT-001",
				installDate:  "2024-05-10",
				warrantyExp:  "2027-05-10",
				locCode:      "LOC-JKT-FLT",
			},
			{
				code:         "AST-FLT-VAN01",
				name:         "Mobil Van Toyota HiAce Premio 2.8L",
				description:  "Regional Fast-Delivery Cargo Van (Plat: B 2345 NTP)",
				assetType:    "VEHICLE",
				criticality:  "C",
				manufacturer: "PT Toyota-Astra Motor",
				model:        "HiAce Premio",
				serialNumber: "B-2345-NTP",
				tagNumber:    "TAG-FLT-002",
				installDate:  "2024-06-01",
				warrantyExp:  "2027-06-01",
				locCode:      "LOC-JKT-FLT",
			},
			{
				code:         "AST-FAC-GEN01",
				name:         "Genset Diesel Standby Cummins 500kVA",
				description:  "Emergency Standby Power Generator Set with Automatic Transfer Switch",
				assetType:    "INFRASTRUCTURE",
				criticality:  "A",
				manufacturer: "Cummins Inc.",
				model:        "C500 D5e",
				serialNumber: "CUM-2023-3321",
				tagNumber:    "TAG-UTL-002",
				installDate:  "2023-08-20",
				warrantyExp:  "2028-08-20",
				locCode:      "LOC-JKT-UTL",
			},
		}

		assetMap := make(map[string]int64)
		for _, a := range assets {
			locID := locMap[a.locCode]
			instDate := ParseDate(a.installDate)
			warrDate := ParseDate(a.warrantyExp)

			var assetID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO assets (
					company_id, code, name, description, asset_type, location_id,
					manufacturer, model, serial_number, tag_number, install_date, warranty_expiry,
					status, criticality, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'ACTIVE', $13, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					description = EXCLUDED.description,
					asset_type = EXCLUDED.asset_type,
					location_id = EXCLUDED.location_id,
					manufacturer = EXCLUDED.manufacturer,
					model = EXCLUDED.model,
					serial_number = EXCLUDED.serial_number,
					tag_number = EXCLUDED.tag_number,
					install_date = EXCLUDED.install_date,
					warranty_expiry = EXCLUDED.warranty_expiry,
					status = 'ACTIVE',
					criticality = EXCLUDED.criticality,
					updated_at = NOW()
				RETURNING id`,
				companyID, a.code, a.name, a.description, a.assetType, locID,
				a.manufacturer, a.model, a.serialNumber, a.tagNumber, instDate, warrDate, a.criticality).Scan(&assetID)
			if err != nil {
				return fmt.Errorf("upsert asset %q: %w", a.code, err)
			}
			assetMap[a.code] = assetID
		}

		// -------------------------------------------------------------------------
		// 3. Task Templates & Standard Maintenance Steps
		// -------------------------------------------------------------------------
		type taskTemplateDef struct {
			name        string
			description string
			category    string
			estHours    float64
			steps       []struct {
				seq      int
				title    string
				desc     string
				estHours float64
			}
		}

		taskTemplates := []taskTemplateDef{
			{
				name:        "SOP Pembersihan & Kalibrasi Nozzle SMT Harian",
				description: "Pemeriksaan harian vakum nozzle, feeder tension, dan lensa kamera optik SMT",
				category:    "PREVENTIVE",
				estHours:    0.75,
				steps: []struct {
					seq      int
					title    string
					desc     string
					estHours float64
				}{
					{1, "Pemeriksaan Kebocoran Tekanan Vakum Nozzle", "Uji tekanan diferensial vakum pada head pick-and-place", 0.25},
					{2, "Pembersihan Ultrasonik Nozzle Tip", "Rendam nozzle tip dalam cairan pembersih khusus selama 10 menit", 0.25},
					{3, "Verifikasi Kalibrasi Kamera Optik Fiducial", "Jalankan auto-calibration target fiducial mark pada nozzle changer", 0.25},
				},
			},
			{
				name:        "SOP Thermal Profiling & Pembersihan Flux Trap Oven Mingguan",
				description: "Pengukuran kurva suhu 9-zona reflow oven dan pembersihan sisa kondensasi flux",
				category:    "PREVENTIVE",
				estHours:    1.50,
				steps: []struct {
					seq      int
					title    string
					desc     string
					estHours float64
				}{
					{1, "Pembersihan Mesh Filter & Flux Trap Box", "Lepas dan cuci filter perangkap flux pada zona pemanas 4-7", 0.50},
					{2, "Pemasangan Thermocouple Multi-Channel Profiler", "Pasang 9 probe thermocouple pada papan uji kalibrasi reflow", 0.50},
					{3, "Eksekusi Thermal Profile Run & Verifikasi Delta T", "Jalankan profiler dan pastikan Peak Temp 245°C ± 3°C", 0.50},
				},
			},
			{
				name:        "SOP Perawatan Bulanan Chiller HVAC Cleanroom",
				description: "Inspeksi kompresor chiller sentrifugal, water treatment, dan filter udara cleanroom",
				category:    "PREVENTIVE",
				estHours:    2.00,
				steps: []struct {
					seq      int
					title    string
					desc     string
					estHours float64
				}{
					{1, "Pemeriksaan Tekanan Refrigerant R-134a", "Cek suction pressure dan discharge temperature kompresor", 0.50},
					{2, "Pengukuran TDS & Dosis Water Treatment Cooling Tower", "Uji kadar kimia anti-korosi dan kerak pada loop air pendingin", 0.75},
					{3, "Pembersihan Strainer & Pompa Sirkulasi", "Backwash strainer filter dan cek mechanical seal pompa", 0.75},
				},
			},
			{
				name:        "SOP Overhaul Spindle & Kalibrasi Sumbu CNC Triwulanan",
				description: "Inspeksi runout spindle, pelumasan guideway, dan backlash compensation CNC Haas",
				category:    "PREVENTIVE",
				estHours:    3.50,
				steps: []struct {
					seq      int
					title    string
					desc     string
					estHours float64
				}{
					{1, "Pengukuran Spindle Runout Menggunakan Dial Test Indicator", "Uji radial dan axial runout pada taper 40 spindle (< 3 um)", 1.00},
					{2, "Penggantian Way-Lube Oil & Filter Udara Pneumatik", "Kuras oli pelumas rel dan isi ulang Mobil Vactra No. 2", 1.00},
					{3, "Verifikasi Akurasi Positioning Sumbu X/Y/Z", "Jalankan program uji ballbar test untuk verifikasi kelurusan sumbu", 1.50},
				},
			},
			{
				name:        "SOP Uji Beban Penuh (Load Bank) & Analisis Oli Genset Tahunan",
				description: "Pengujian kapasitas genset 500kVA 100% load bank dan pengambilan sampel oli mesin",
				category:    "PREVENTIVE",
				estHours:    4.00,
				steps: []struct {
					seq      int
					title    string
					desc     string
					estHours float64
				}{
					{1, "Penggantian Filter Oli, Filter Solar & Coolant Radiator", "Ganti filter Fleetguard dan periksa kekencangan v-belt alternator", 1.50},
					{2, "Uji Beban Bertahap Load Bank 25%, 50%, 75%, 100%", "Jalankan uji beban penuh selama 2 jam kontinu pada genset", 2.00},
					{3, "Pengambilan Sampel Oli Mesin untuk Spektrometri Laboratorium", "Ambil 500ml oli bekas untuk uji keausan partikel logam lab", 0.50},
				},
			},
		}

		templateMap := make(map[string]int64)
		for _, tt := range taskTemplates {
			var ttID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO task_templates (company_id, name, description, category, estimated_hours, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
				RETURNING id`, companyID, tt.name, tt.description, tt.category, tt.estHours).Scan(&ttID)
			if err != nil {
				return fmt.Errorf("insert task_template %q: %w", tt.name, err)
			}
			templateMap[tt.name] = ttID

			for _, st := range tt.steps {
				if _, err := tx.Exec(ctx, `
					INSERT INTO task_template_steps (task_template_id, sequence, title, description, estimated_hours, created_at)
					VALUES ($1, $2, $3, $4, $5, NOW())`, ttID, st.seq, st.title, st.desc, st.estHours); err != nil {
					return fmt.Errorf("insert task_template_step for %q seq %d: %w", tt.name, st.seq, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 4. Preventive Maintenance Schedules (5 PM Schedules Across Frequencies)
		// -------------------------------------------------------------------------
		type pmScheduleDef struct {
			assetCode    string
			name         string
			desc         string
			freqType     string
			freqValue    int
			templateName string
			nextDue      string
		}

		pmSchedules := []pmScheduleDef{
			{
				assetCode:    "AST-SMT-PNP01",
				name:         "PM Harian: Kalibrasi Nozzle & Head Pick-and-Place Yamaha",
				desc:         "Pemeriksaan harian vakum nozzle dan pembersihan lensa kamera optik",
				freqType:     "DAILY",
				freqValue:    1,
				templateName: "SOP Pembersihan & Kalibrasi Nozzle SMT Harian",
				nextDue:      "2026-08-31",
			},
			{
				assetCode:    "AST-SMT-RFO01",
				name:         "PM Mingguan: Thermal Profiling & Filter Flux Oven Heller",
				desc:         "Verifikasi kurva suhu 9-zona reflow oven dan pembersihan filter trap",
				freqType:     "WEEKLY",
				freqValue:    1,
				templateName: "SOP Thermal Profiling & Pembersihan Flux Trap Oven Mingguan",
				nextDue:      "2026-08-31",
			},
			{
				assetCode:    "AST-FAC-CHL01",
				name:         "PM Bulanan: Descaling Koil & Water Treatment Chiller Daikin",
				desc:         "Inspeksi kompresor sentrifugal dan pemantauan kualitas air pendingin",
				freqType:     "MONTHLY",
				freqValue:    1,
				templateName: "SOP Perawatan Bulanan Chiller HVAC Cleanroom",
				nextDue:      "2026-08-31",
			},
			{
				assetCode:    "AST-MCH-CNC01",
				name:         "PM Triwulanan: Overhaul Spindle & Pelumasan Rel CNC Haas",
				desc:         "Uji dial runout spindle, penggantian way-lube oil, dan ballbar test",
				freqType:     "QUARTERLY",
				freqValue:    1,
				templateName: "SOP Overhaul Spindle & Kalibrasi Sumbu CNC Triwulanan",
				nextDue:      "2026-09-15",
			},
			{
				assetCode:    "AST-FAC-GEN01",
				name:         "PM Tahunan: Uji Beban Penuh 500kVA Load Bank Genset Cummins",
				desc:         "Ganti filter, fluida pendingin, dan uji beban 100% generator darurat",
				freqType:     "ANNUAL",
				freqValue:    1,
				templateName: "SOP Uji Beban Penuh (Load Bank) & Analisis Oli Genset Tahunan",
				nextDue:      "2026-08-20",
			},
		}

		pmScheduleMap := make(map[string]int64)
		for _, pm := range pmSchedules {
			assetID := assetMap[pm.assetCode]
			ttID := templateMap[pm.templateName]
			nextDueDate := ParseDate(pm.nextDue)

			var pmID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO pm_schedules (
					company_id, asset_id, name, description, frequency_type, frequency_value,
					task_template_id, next_due_date, active, status, created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE, 'ACTIVE', NOW(), NOW())
				RETURNING id`, companyID, assetID, pm.name, pm.desc, pm.freqType, pm.freqValue, ttID, nextDueDate).Scan(&pmID)
			if err != nil {
				return fmt.Errorf("insert pm_schedule for %q: %w", pm.assetCode, err)
			}
			pmScheduleMap[pm.assetCode] = pmID
		}

		// -------------------------------------------------------------------------
		// 5. Spare Parts Catalog (12 Items: Min Quantity, Reorder Point & Unit Costs)
		// -------------------------------------------------------------------------
		type sparePartDef struct {
			code        string
			name        string
			description string
			category    string
			uom         string
			minQty      float64
			maxQty      float64
			reorderPt   float64
			leadTime    int32
			unitCost    float64
			critical    bool
		}

		spareParts := []sparePartDef{
			{
				code:        "SP-SMT-NOZ01",
				name:        "SMT Feeder Pick-and-Place Vacuum Nozzle 301A/302A",
				description: "Yamaha YSM20R Precision Ceramic High-Speed Vacuum Nozzle",
				category:    "SMT_CONSUMABLE",
				uom:         "PCS",
				minQty:      10.0,
				maxQty:      50.0,
				reorderPt:   15.0,
				leadTime:    14,
				unitCost:    450000.0,
				critical:    true,
			},
			{
				code:        "SP-SMT-BELT01",
				name:        "Timing Drive Belt Antistatik Konveyor SMT 450mm",
				description: "Conductive Rubber Timing Belt for Yamaha SMT PCB Feeder Conveyor",
				category:    "MECHANICAL",
				uom:         "PCS",
				minQty:      5.0,
				maxQty:      20.0,
				reorderPt:   8.0,
				leadTime:    7,
				unitCost:    280000.0,
				critical:    false,
			},
			{
				code:        "SP-RFO-FLTR01",
				name:        "Stainless Mesh Filter Flux Trap Reflow Oven",
				description: "High-Efficiency Stainless Steel Flux Condensation Filter Cartridge",
				category:    "FILTRATION",
				uom:         "PCS",
				minQty:      4.0,
				maxQty:      16.0,
				reorderPt:   6.0,
				leadTime:    10,
				unitCost:    850000.0,
				critical:    true,
			},
			{
				code:        "SP-RFO-THRM01",
				name:        "K-Type 9-Channel Thermocouple Temperature Probe Wire",
				description: "High-Temperature Glass Braid Thermocouple Sensor Probe Set",
				category:    "ELECTRICAL",
				uom:         "SET",
				minQty:      6.0,
				maxQty:      24.0,
				reorderPt:   10.0,
				leadTime:    5,
				unitCost:    320000.0,
				critical:    true,
			},
			{
				code:        "SP-CNC-COOL01",
				name:        "Coolant Pemotongan Logam Semi-Sintetik Drum 20L",
				description: "High-Pressure Soluble Metalworking Cutting Fluid for Haas CNC",
				category:    "CHEMICAL",
				uom:         "DRUM",
				minQty:      3.0,
				maxQty:      12.0,
				reorderPt:   5.0,
				leadTime:    3,
				unitCost:    1250000.0,
				critical:    false,
			},
			{
				code:        "SP-CNC-LUBE01",
				name:        "Mobil Vactra Oil No. 2 ISO VG 68 Slideway Lube (5L)",
				description: "Premium Quality Slideway Lubricant for CNC Linear Guideways",
				category:    "LUBRICANT",
				uom:         "CAN",
				minQty:      4.0,
				maxQty:      15.0,
				reorderPt:   6.0,
				leadTime:    4,
				unitCost:    650000.0,
				critical:    false,
			},
			{
				code:        "SP-WAV-NOZ01",
				name:        "Nozzle Solder Pot & Impeller Titanium Wave Soldering",
				description: "Corrosion-Resistant Titanium Wave Solder Chute and Impeller Tip",
				category:    "SMT_CONSUMABLE",
				uom:         "PCS",
				minQty:      2.0,
				maxQty:      8.0,
				reorderPt:   3.0,
				leadTime:    21,
				unitCost:    3500000.0,
				critical:    true,
			},
			{
				code:        "SP-CHL-PUMP01",
				name:        "Mechanical Seal Kit Pompa Sirkulasi Chiller 2.5 Inch",
				description: "Carbon-Silicon Carbide Mechanical Water Pump Seal Kit for Daikin Chiller",
				category:    "MECHANICAL",
				uom:         "SET",
				minQty:      2.0,
				maxQty:      6.0,
				reorderPt:   3.0,
				leadTime:    14,
				unitCost:    1850000.0,
				critical:    true,
			},
			{
				code:        "SP-GEN-FLTR01",
				name:        "Set Filter Oli & Solar Genset Cummins Fleetguard",
				description: "Primary/Secondary Fuel Water Separator & Lube Filter Element Set",
				category:    "FILTRATION",
				uom:         "SET",
				minQty:      4.0,
				maxQty:      12.0,
				reorderPt:   5.0,
				leadTime:    7,
				unitCost:    1100000.0,
				critical:    true,
			},
			{
				code:        "SP-OSC-PRB01",
				name:        "Probe Pasif Oscilloscope Keysight 500MHz 10:1 10Mohm",
				description: "High-Impedance Precision Modular Oscilloscope Voltage Probe Set",
				category:    "ELECTRICAL",
				uom:         "SET",
				minQty:      4.0,
				maxQty:      10.0,
				reorderPt:   5.0,
				leadTime:    14,
				unitCost:    2400000.0,
				critical:    false,
			},
			{
				code:        "SP-TRK-BRK01",
				name:        "Kampas Rem Tromol Heavy-Duty Isuzu Giga FVR",
				description: "Heavy-Duty Asbestos-Free Front/Rear Brake Lining Shoe Kit",
				category:    "VEHICLE_PARTS",
				uom:         "SET",
				minQty:      2.0,
				maxQty:      8.0,
				reorderPt:   4.0,
				leadTime:    5,
				unitCost:    1650000.0,
				critical:    false,
			},
			{
				code:        "SP-ELE-FUSE01",
				name:        "Sekring Industri Bussmann 600V 30A Time-Delay",
				description: "Class CC High-Interrupting Capacity Industrial Fuse for Power Panels",
				category:    "ELECTRICAL",
				uom:         "PCS",
				minQty:      20.0,
				maxQty:      100.0,
				reorderPt:   30.0,
				leadTime:    3,
				unitCost:    95000.0,
				critical:    false,
			},
		}

		sparePartMap := make(map[string]int64)
		for _, sp := range spareParts {
			var spID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO spare_parts (
					company_id, code, name, description, category, unit_of_measure,
					min_quantity, max_quantity, reorder_point, lead_time_days, unit_cost, critical_spare,
					created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					description = EXCLUDED.description,
					category = EXCLUDED.category,
					unit_of_measure = EXCLUDED.unit_of_measure,
					min_quantity = EXCLUDED.min_quantity,
					max_quantity = EXCLUDED.max_quantity,
					reorder_point = EXCLUDED.reorder_point,
					lead_time_days = EXCLUDED.lead_time_days,
					unit_cost = EXCLUDED.unit_cost,
					critical_spare = EXCLUDED.critical_spare,
					updated_at = NOW()
				RETURNING id`,
				companyID, sp.code, sp.name, sp.description, sp.category, sp.uom,
				sp.minQty, sp.maxQty, sp.reorderPt, sp.leadTime, sp.unitCost, sp.critical).Scan(&spID)
			if err != nil {
				return fmt.Errorf("upsert spare_part %q: %w", sp.code, err)
			}
			sparePartMap[sp.code] = spID
		}

		// -------------------------------------------------------------------------
		// 6. Maintenance Work Orders (7 Work Orders Covering Multiple Statuses & Categories)
		// -------------------------------------------------------------------------
		type mwoDef struct {
			number    string
			title     string
			desc      string
			assetCode string
			locCode   string
			priority  string
			status    string
			category  string
			pmAsset   string
			estHours  float64
			actHours  float64
			planStart string
			planEnd   string
			actStart  *string
			actEnd    *string
			tasks     []struct {
				seq      int
				title    string
				desc     string
				status   string
				estHours float64
				actHours float64
			}
			parts []struct {
				partCode string
				qty      float64
			}
		}

		actStart1 := "2026-03-18 08:30:00"
		actEnd1 := "2026-03-18 12:45:00"
		actStart2 := "2026-04-05 09:15:00"
		actEnd2 := "2026-04-05 13:30:00"
		actStart3 := "2026-05-12 08:00:00"
		actEnd3 := "2026-05-12 11:30:00"
		actStart4 := "2026-06-20 13:00:00"
		actEnd4 := "2026-06-20 16:00:00"
		actStart5 := "2026-07-28 09:00:00"

		workOrders := []mwoDef{
			{
				number:    "MWO-202603-001",
				title:     "Uji Beban Penuh 500kVA Load Bank & Analisis Oli Genset Cummins",
				desc:     "Perawatan tahunan genset darurat dan pengujian kapasitas daya beban penuh",
				assetCode: "AST-FAC-GEN01",
				locCode:   "LOC-JKT-UTL",
				priority:  "HIGH",
				status:    "COMPLETED",
				category:  "PREVENTIVE",
				pmAsset:   "AST-FAC-GEN01",
				estHours:  4.0,
				actHours:  4.25,
				planStart: "2026-03-18 08:00:00",
				planEnd:   "2026-03-18 12:30:00",
				actStart:  &actStart1,
				actEnd:    &actEnd1,
				tasks: []struct {
					seq      int
					title    string
					desc     string
					status   string
					estHours float64
					actHours float64
				}{
					{1, "Penggantian Filter Fleetguard & Coolant", "Ganti filter oli dan kuras radiator", "COMPLETED", 1.5, 1.5},
					{2, "Uji Beban Bertahap Load Bank 25%-100%", "Jalankan pengujian beban penuh 2 jam", "COMPLETED", 2.0, 2.25},
					{3, "Pengambilan Sampel Oli Mesin ke Lab", "Ambil 500ml oli untuk spektrometri", "COMPLETED", 0.5, 0.5},
				},
				parts: []struct {
					partCode string
					qty      float64
				}{
					{"SP-GEN-FLTR01", 1.0},
				},
			},
			{
				number:    "MWO-202604-002",
				title:     "Perbaikan Darurat: Macet Konveyor Feeder SMT & Ganti Timing Belt",
				desc:     "Sabuk timing drive konveyor aus menyebabkan macet pada pengumpan komponen PCB",
				assetCode: "AST-SMT-PNP01",
				locCode:   "LOC-JKT-SMT",
				priority:  "CRITICAL",
				status:    "COMPLETED",
				category:  "CORRECTIVE",
				estHours:  3.5,
				actHours:  4.25,
				planStart: "2026-04-05 09:00:00",
				planEnd:   "2026-04-05 13:00:00",
				actStart:  &actStart2,
				actEnd:    &actEnd2,
				tasks: []struct {
					seq      int
					title    string
					desc     string
					status   string
					estHours float64
					actHours float64
				}{
					{1, "Isolasi Area & Bongkar Cover Konveyor SMT", "Lakukan LOTO dan buka pelindung antistatik", "COMPLETED", 1.0, 1.0},
					{2, "Ganti Timing Drive Belt & Nozzle Suction Tip", "Pasang belt antistatik baru dan ganti 2 nozzle aus", "COMPLETED", 1.5, 2.0},
					{3, "Uji Kecepatan Umpan & Kalibrasi Fiducial", "Jalankan siklus uji 50 PCB tanpa error penempatan", "COMPLETED", 1.0, 1.25},
				},
				parts: []struct {
					partCode string
					qty      float64
				}{
					{"SP-SMT-BELT01", 2.0},
					{"SP-SMT-NOZ01", 2.0},
				},
			},
			{
				number:    "MWO-202605-003",
				title:     "Perawatan Triwulanan: Overhaul Spindle & Penggantian Way-Lube CNC Haas",
				desc:     "Pemeriksaan spindle runout, pelumasan rel pemandu, dan penggantian cairan coolant",
				assetCode: "AST-MCH-CNC01",
				locCode:   "LOC-JKT-ASY",
				priority:  "MEDIUM",
				status:    "COMPLETED",
				category:  "PREVENTIVE",
				pmAsset:   "AST-MCH-CNC01",
				estHours:  3.5,
				actHours:  3.5,
				planStart: "2026-05-12 08:00:00",
				planEnd:   "2026-05-12 12:00:00",
				actStart:  &actStart3,
				actEnd:    &actEnd3,
				tasks: []struct {
					seq      int
					title    string
					desc     string
					status   string
					estHours float64
					actHours float64
				}{
					{1, "Pengukuran Spindle Runout Menggunakan DTI", "Dial indicator mengonfirmasi runout 2.2 um (OK)", "COMPLETED", 1.0, 1.0},
					{2, "Penggantian Way-Lube Mobil Vactra & Coolant", "Kuras tangki coolant dan isi ulang 20L coolant baru", "COMPLETED", 1.5, 1.5},
					{3, "Verifikasi Akurasi Kalibrasi Sumbu XYZ", "Jalankan uji program pemotongan balok referensi", "COMPLETED", 1.0, 1.0},
				},
				parts: []struct {
					partCode string
					qty      float64
				}{
					{"SP-CNC-LUBE01", 1.0},
					{"SP-CNC-COOL01", 1.0},
				},
			},
			{
				number:    "MWO-202606-004",
				title:     "Analisis Getaran Prediktif & Penggantian Seal Pompa Chiller Daikin",
				desc:     "Sensor IoT mencatat anomali getaran bearing pompa pendingin sekunder",
				assetCode: "AST-FAC-CHL01",
				locCode:   "LOC-JKT-UTL",
				priority:  "MEDIUM",
				status:    "COMPLETED",
				category:  "PREDICTIVE",
				estHours:  3.0,
				actHours:  3.0,
				planStart: "2026-06-20 13:00:00",
				planEnd:   "2026-06-20 16:30:00",
				actStart:  &actStart4,
				actEnd:    &actEnd4,
				tasks: []struct {
					seq      int
					title    string
					desc     string
					status   string
					estHours float64
					actHours float64
				}{
					{1, "Pengukuran Spektrum Getaran FFT Pompa", "Analisis frekuensi puncak getaran pada 120Hz", "COMPLETED", 1.0, 1.0},
					{2, "Penggantian Mechanical Seal Pompa 2.5 Inch", "Bongkar casing pompa dan ganti seal keramik baru", "COMPLETED", 1.5, 1.5},
					{3, "Uji Tekanan Sirkulasi & Re-Analisis Getaran", "Tekanan normal 4.2 bar, amplitudo getaran turun 80%", "COMPLETED", 0.5, 0.5},
				},
				parts: []struct {
					partCode string
					qty      float64
				}{
					{"SP-CHL-PUMP01", 1.0},
				},
			},
			{
				number:    "MWO-202607-005",
				title:     "Kalibrasi Berkala & Verifikasi Traceability Digital Oscilloscope Keysight",
				desc:     "Kalibrasi standar acuan tegangan DC dan bandwidth frekuensi tinggi 2GHz",
				assetCode: "AST-TST-OSC01",
				locCode:   "LOC-JKT-LAB",
				priority:  "HIGH",
				status:    "IN_PROGRESS",
				category:  "CALIBRATION",
				estHours:  2.5,
				actHours:  1.5,
				planStart: "2026-07-28 09:00:00",
				planEnd:   "2026-07-28 12:00:00",
				actStart:  &actStart5,
				actEnd:    nil,
				tasks: []struct {
					seq      int
					title    string
					desc     string
					status   string
					estHours float64
					actHours float64
				}{
					{1, "Kalibrasi Gain Saluran 1-8 Menggunakan Fluke 5522A", "Verifikasi akurasi vertikal skala 1mV/div sampai 10V/div", "COMPLETED", 1.5, 1.5},
					{2, "Uji Bandwidth & Rise Time Sinyal Frekuensi Tinggi", "Pengujian pulsa ultrafast 50ps rise time", "IN_PROGRESS", 1.0, 0.0},
				},
			},
			{
				number:    "MWO-202608-006",
				title:     "Perawatan Mingguan: Thermal Profiling Reflow Oven Heller 9-Zona",
				desc:     "Jadwal rutin pengecekan kurva suhu profil solder bebas timbal SAC305",
				assetCode: "AST-SMT-RFO01",
				locCode:   "LOC-JKT-SMT",
				priority:  "MEDIUM",
				status:    "PLANNED",
				category:  "PREVENTIVE",
				pmAsset:   "AST-SMT-RFO01",
				estHours:  1.5,
				actHours:  0.0,
				planStart: "2026-08-31 08:00:00",
				planEnd:   "2026-08-31 10:00:00",
				tasks: []struct {
					seq      int
					title    string
					desc     string
					status   string
					estHours float64
					actHours float64
				}{
					{1, "Pembersihan Mesh Flux Trap Zona 5-7", "Cuci dan bersihkan sisa kondensasi flux", "PLANNED", 0.5, 0.0},
					{2, "Pengukuran Kurva Suhu 9 Probe Thermocouple", "Jalankan KIC thermal profiler pada board dummy", "PLANNED", 1.0, 0.0},
				},
			},
			{
				number:    "MWO-202608-007",
				title:     "Inspeksi Sistem Pengereman & Servis Berkala Truk Box Isuzu Giga",
				desc:     "Pemeriksaan ketebalan kampas rem tromol dan penggantian fluida minyak rem",
				assetCode: "AST-FLT-TRK01",
				locCode:   "LOC-JKT-FLT",
				priority:  "LOW",
				status:    "DRAFT",
				category:  "INSPECTION",
				estHours:  2.0,
				actHours:  0.0,
				planStart: "2026-08-31 14:00:00",
				planEnd:   "2026-08-31 16:30:00",
				tasks: []struct {
					seq      int
					title    string
					desc     string
					status   string
					estHours float64
					actHours float64
				}{
					{1, "Pemeriksaan Ketebalan Kampas Rem Tromol Roda", "Cek ketebalan kampas rem roda depan dan belakang", "DRAFT", 1.0, 0.0},
					{2, "Bleeding Minyak Rem DOT-4 & Uji Pengereman Jalan", "Kuras dan ganti minyak rem hidrolik", "DRAFT", 1.0, 0.0},
				},
			},
		}

		for _, wo := range workOrders {
			assetID := assetMap[wo.assetCode]
			locID := locMap[wo.locCode]
			planStartT, _ := time.Parse("2006-01-02 15:04:05", wo.planStart)
			planEndT, _ := time.Parse("2006-01-02 15:04:05", wo.planEnd)

			var actStartT, actEndT *time.Time
			if wo.actStart != nil {
				t, _ := time.Parse("2006-01-02 15:04:05", *wo.actStart)
				actStartT = &t
			}
			if wo.actEnd != nil {
				t, _ := time.Parse("2006-01-02 15:04:05", *wo.actEnd)
				actEndT = &t
			}

			var pmID *int64
			if wo.pmAsset != "" {
				if id, ok := pmScheduleMap[wo.pmAsset]; ok {
					pmID = &id
				}
			}

			var woID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO work_orders (
					company_id, number, title, description, asset_id, location_id,
					priority, status, category, requester_id, assignee_id,
					planned_start, planned_end, actual_start, actual_end,
					estimated_hours, actual_hours, pm_schedule_id, created_by,
					created_at, updated_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $12, NOW())
				ON CONFLICT (company_id, number) DO UPDATE SET
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					asset_id = EXCLUDED.asset_id,
					location_id = EXCLUDED.location_id,
					priority = EXCLUDED.priority,
					status = EXCLUDED.status,
					category = EXCLUDED.category,
					assignee_id = EXCLUDED.assignee_id,
					planned_start = EXCLUDED.planned_start,
					planned_end = EXCLUDED.planned_end,
					actual_start = EXCLUDED.actual_start,
					actual_end = EXCLUDED.actual_end,
					estimated_hours = EXCLUDED.estimated_hours,
					actual_hours = EXCLUDED.actual_hours,
					pm_schedule_id = EXCLUDED.pm_schedule_id,
					updated_at = NOW()
				RETURNING id`,
				companyID, wo.number, wo.title, wo.desc, assetID, locID,
				wo.priority, wo.status, wo.category, prodMgrID, maintLeadID,
				planStartT, planEndT, actStartT, actEndT,
				wo.estHours, wo.actHours, pmID, maintLeadID).Scan(&woID)
			if err != nil {
				return fmt.Errorf("upsert CMMS work_order %q: %w", wo.number, err)
			}

			// Insert Checklist Tasks
			for _, tsk := range wo.tasks {
				var completedAt *time.Time
				if tsk.status == "COMPLETED" && actEndT != nil {
					completedAt = actEndT
				}

				if _, err := tx.Exec(ctx, `
					INSERT INTO work_order_tasks (
						work_order_id, sequence, title, description, status,
						assignee_id, estimated_hours, actual_hours, completed_at,
						created_at, updated_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())`,
					woID, tsk.seq, tsk.title, tsk.desc, tsk.status,
					maintLeadID, tsk.estHours, tsk.actHours, completedAt, planStartT); err != nil {
					return fmt.Errorf("insert work_order_task for wo %q seq %d: %w", wo.number, tsk.seq, err)
				}
			}

			// Record Spare Part Consumptions
			for _, prt := range wo.parts {
				spID := sparePartMap[prt.partCode]
				var unitCost float64
				for _, sp := range spareParts {
					if sp.code == prt.partCode {
						unitCost = sp.unitCost
						break
					}
				}
				totalCost := unitCost * prt.qty

				if _, err := tx.Exec(ctx, `
					INSERT INTO work_order_spare_parts (
						work_order_id, spare_part_id, quantity, unit_cost, total_cost,
						issued_at, issued_by, created_at
					) VALUES ($1, $2, $3, $4, $5, $6, $7, $6)`,
					woID, spID, prt.qty, unitCost, totalCost, planStartT, maintLeadID); err != nil {
					return fmt.Errorf("insert work_order_spare_part for wo %q part %q: %w", wo.number, prt.partCode, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 7. Equipment Meter Readings (March 1, 2026 – August 31, 2026)
		// -------------------------------------------------------------------------
		type meterLogDef struct {
			assetCode string
			reading   string
			value     float64
			dateStr   string
			note      string
		}

		meterLogs := []meterLogDef{
			// Yamaha Pick-and-Place (HOURS)
			{"AST-SMT-PNP01", "HOURS", 1250.0, "2026-03-31", "Log akhir bulan Maret jam operasi SMT Line 1"},
			{"AST-SMT-PNP01", "HOURS", 1420.0, "2026-04-30", "Log akhir bulan April jam operasi SMT Line 1"},
			{"AST-SMT-PNP01", "HOURS", 1610.0, "2026-05-31", "Log akhir bulan Mei jam operasi SMT Line 1"},
			{"AST-SMT-PNP01", "HOURS", 1800.0, "2026-06-30", "Log akhir bulan Juni jam operasi SMT Line 1"},
			{"AST-SMT-PNP01", "HOURS", 1980.0, "2026-07-31", "Log akhir bulan Juli jam operasi SMT Line 1"},
			{"AST-SMT-PNP01", "HOURS", 2150.0, "2026-08-31", "Log akhir bulan Agustus jam operasi SMT Line 1"},

			// Heller Reflow Oven (HOURS)
			{"AST-SMT-RFO01", "HOURS", 1120.0, "2026-03-31", "Log kumulatif jam pemanasan oven zona 1-9"},
			{"AST-SMT-RFO01", "HOURS", 1290.0, "2026-04-30", "Log kumulatif jam pemanasan oven zona 1-9"},
			{"AST-SMT-RFO01", "HOURS", 1470.0, "2026-05-31", "Log kumulatif jam pemanasan oven zona 1-9"},
			{"AST-SMT-RFO01", "HOURS", 1650.0, "2026-06-30", "Log kumulatif jam pemanasan oven zona 1-9"},
			{"AST-SMT-RFO01", "HOURS", 1820.0, "2026-07-31", "Log kumulatif jam pemanasan oven zona 1-9"},
			{"AST-SMT-RFO01", "HOURS", 1990.0, "2026-08-31", "Log kumulatif jam pemanasan oven zona 1-9"},

			// Daikin Industrial Chiller (HOURS & TEMPERATURE)
			{"AST-FAC-CHL01", "HOURS", 2800.0, "2026-03-31", "Jam kompresor sentrifugal chiller HVAC"},
			{"AST-FAC-CHL01", "TEMPERATURE", 6.2, "2026-03-31", "Suhu air pasokan chilled water (°C)"},
			{"AST-FAC-CHL01", "HOURS", 3100.0, "2026-04-30", "Jam kompresor sentrifugal chiller HVAC"},
			{"AST-FAC-CHL01", "TEMPERATURE", 6.4, "2026-04-30", "Suhu air pasokan chilled water (°C)"},
			{"AST-FAC-CHL01", "HOURS", 3450.0, "2026-05-31", "Jam kompresor sentrifugal chiller HVAC"},
			{"AST-FAC-CHL01", "TEMPERATURE", 6.1, "2026-05-31", "Suhu air pasokan chilled water (°C)"},
			{"AST-FAC-CHL01", "HOURS", 3800.0, "2026-06-30", "Jam kompresor sentrifugal chiller HVAC"},
			{"AST-FAC-CHL01", "TEMPERATURE", 6.3, "2026-06-30", "Suhu air pasokan chilled water (°C)"},
			{"AST-FAC-CHL01", "HOURS", 4150.0, "2026-07-31", "Jam kompresor sentrifugal chiller HVAC"},
			{"AST-FAC-CHL01", "TEMPERATURE", 6.2, "2026-07-31", "Suhu air pasokan chilled water (°C)"},
			{"AST-FAC-CHL01", "HOURS", 4500.0, "2026-08-31", "Jam kompresor sentrifugal chiller HVAC"},
			{"AST-FAC-CHL01", "TEMPERATURE", 6.1, "2026-08-31", "Suhu air pasokan chilled water (°C)"},

			// Haas CNC Milling Machine (HOURS)
			{"AST-MCH-CNC01", "HOURS", 950.0, "2026-03-31", "Jam putar spindle CNC milling workshop"},
			{"AST-MCH-CNC01", "HOURS", 1100.0, "2026-04-30", "Jam putar spindle CNC milling workshop"},
			{"AST-MCH-CNC01", "HOURS", 1260.0, "2026-05-31", "Jam putar spindle CNC milling workshop"},
			{"AST-MCH-CNC01", "HOURS", 1420.0, "2026-06-30", "Jam putar spindle CNC milling workshop"},
			{"AST-MCH-CNC01", "HOURS", 1580.0, "2026-07-31", "Jam putar spindle CNC milling workshop"},
			{"AST-MCH-CNC01", "HOURS", 1720.0, "2026-08-31", "Jam putar spindle CNC milling workshop"},

			// Isuzu Giga Cargo Box Truck (DISTANCE in KM)
			{"AST-FLT-TRK01", "DISTANCE", 15200.0, "2026-03-31", "Odometer pengiriman rute Jabodetabek-Surabaya"},
			{"AST-FLT-TRK01", "DISTANCE", 17100.0, "2026-04-30", "Odometer pengiriman rute Jabodetabek-Surabaya"},
			{"AST-FLT-TRK01", "DISTANCE", 19300.0, "2026-05-31", "Odometer pengiriman rute Jabodetabek-Surabaya"},
			{"AST-FLT-TRK01", "DISTANCE", 21500.0, "2026-06-30", "Odometer pengiriman rute Jabodetabek-Surabaya"},
			{"AST-FLT-TRK01", "DISTANCE", 23800.0, "2026-07-31", "Odometer pengiriman rute Jabodetabek-Surabaya"},
			{"AST-FLT-TRK01", "DISTANCE", 25900.0, "2026-08-31", "Odometer pengiriman rute Jabodetabek-Surabaya"},

			// Cummins Standby Generator (HOURS)
			{"AST-FAC-GEN01", "HOURS", 145.0, "2026-03-31", "Jam uji operasional mingguan genset darurat"},
			{"AST-FAC-GEN01", "HOURS", 152.0, "2026-04-30", "Jam uji operasional mingguan genset darurat"},
			{"AST-FAC-GEN01", "HOURS", 160.0, "2026-05-31", "Jam uji operasional mingguan genset darurat"},
			{"AST-FAC-GEN01", "HOURS", 168.0, "2026-06-30", "Jam uji operasional mingguan genset darurat"},
			{"AST-FAC-GEN01", "HOURS", 175.0, "2026-07-31", "Jam uji operasional mingguan genset darurat"},
			{"AST-FAC-GEN01", "HOURS", 182.0, "2026-08-31", "Jam uji operasional mingguan genset darurat"},
		}

		for _, m := range meterLogs {
			assetID := assetMap[m.assetCode]
			rDate := ParseDate(m.dateStr)

			if _, err := tx.Exec(ctx, `
				INSERT INTO meter_readings (asset_id, reading_type, value, reading_date, entered_by, notes, created_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				assetID, m.reading, m.value, rDate, maintLeadID, m.note, rDate.Add(17*time.Hour)); err != nil {
				return fmt.Errorf("insert meter_reading for %q (%s): %w", m.assetCode, m.dateStr, err)
			}
		}

		return nil
	})
}
