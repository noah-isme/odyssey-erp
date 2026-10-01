package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase13Logistics seeds carriers, rate cards, fleets, vehicles, commercial drivers,
// shipments, shipment lines, trips, and trip stops for PT Nusantara Teknik Perkasa.
func seedPhase13Logistics(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 13: Logistics & Fleet Management", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			return fmt.Errorf("admin user budi.santoso@nusantarateknik.co.id not found")
		}
		warehouseLeadID := sctx.UserIDs["dewi.lestari@nusantarateknik.co.id"]
		if warehouseLeadID == 0 {
			warehouseLeadID = adminID
		}

		whJktFG := sctx.WarehouseIDs["WH-JKT-FG"]
		if whJktFG == 0 {
			return fmt.Errorf("warehouse WH-JKT-FG not found")
		}
		whCkrFG := sctx.WarehouseIDs["WH-CKR-FG"]
		if whCkrFG == 0 {
			whCkrFG = whJktFG
		}
		whSbyDist := sctx.WarehouseIDs["WH-SBY-DIST"]

		// -------------------------------------------------------------------------
		// 1. Carriers (2 Logistics Partners: JNE Express & Dakota Cargo)
		// -------------------------------------------------------------------------
		type carrierDef struct {
			code         string
			name         string
			status       string
			contactName  string
			contactEmail string
			contactPhone string
			insProvider  string
			insPolicy    string
			insExpiry    time.Time
		}

		carriers := []carrierDef{
			{
				code:         "CARRIER-JNE",
				name:         "PT Jalur Nugraha Ekakurir (JNE Express)",
				status:       "ACTIVE",
				contactName:  "Bambang Hidayat",
				contactEmail: "bambang.hidayat@jne.co.id",
				contactPhone: "+62215665262",
				insProvider:  "PT Asuransi Jasa Indonesia (Jasindo)",
				insPolicy:    "POL-JAS-2026-8821",
				insExpiry:    time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				code:         "CARRIER-DAKOTA",
				name:         "PT Dakota Buana Semesta (Dakota Cargo)",
				status:       "ACTIVE",
				contactName:  "Surya Gunawan",
				contactEmail: "surya.gunawan@dakotacargo.co.id",
				contactPhone: "+622182410101",
				insProvider:  "PT Asuransi Tokio Marine Indonesia",
				insPolicy:    "POL-TMI-2026-4419",
				insExpiry:    time.Date(2027, 6, 30, 0, 0, 0, 0, time.UTC),
			},
		}

		carrierIDs := make(map[string]int64)
		for _, c := range carriers {
			var cid int64
			err := tx.QueryRow(ctx, `
				INSERT INTO carriers (
					company_id, carrier_name, carrier_code, status,
					contact_name, contact_email, contact_phone,
					insurance_provider, insurance_policy_number, insurance_expires_at,
					created_at, updated_at, created_by, updated_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW(), $11, $11)
				ON CONFLICT (company_id, carrier_code) DO UPDATE SET
					carrier_name = EXCLUDED.carrier_name,
					status = EXCLUDED.status,
					contact_name = EXCLUDED.contact_name,
					contact_email = EXCLUDED.contact_email,
					contact_phone = EXCLUDED.contact_phone,
					insurance_provider = EXCLUDED.insurance_provider,
					insurance_policy_number = EXCLUDED.insurance_policy_number,
					insurance_expires_at = EXCLUDED.insurance_expires_at,
					updated_at = NOW(),
					updated_by = EXCLUDED.updated_by
				RETURNING id`,
				sctx.CompanyNTPID, c.name, c.code, c.status,
				c.contactName, c.contactEmail, c.contactPhone,
				c.insProvider, c.insPolicy, c.insExpiry,
				adminID,
			).Scan(&cid)
			if err != nil {
				return fmt.Errorf("upsert carrier %q: %w", c.code, err)
			}
			carrierIDs[c.code] = cid
		}

		// -------------------------------------------------------------------------
		// 2. Carrier Rate Cards
		// -------------------------------------------------------------------------
		type rateCardDef struct {
			carrierCode string
			fromCity    string
			toCity      string
			weightFrom  float64
			weightTo    float64
			volFrom     float64
			volTo       float64
			rate        float64
			unit        string
			currency    string
			effFrom     string
			minCharge   float64
			fuelPct     float64
		}

		rateCards := []rateCardDef{
			// JNE Express rates (Express parcel & lightweight cargo)
			{"CARRIER-JNE", "Jakarta", "Surabaya", 0, 10, 0, 0.1, 35000, "KG", "IDR", "2026-01-01", 35000, 3.5},
			{"CARRIER-JNE", "Jakarta", "Surabaya", 10.0001, 100, 0.1, 1.0, 28000, "KG", "IDR", "2026-01-01", 350000, 3.5},
			{"CARRIER-JNE", "Jakarta", "Bandung", 0, 50, 0, 0.5, 18000, "KG", "IDR", "2026-01-01", 18000, 2.5},
			{"CARRIER-JNE", "Jakarta", "Semarang", 0, 100, 0, 1.0, 24000, "KG", "IDR", "2026-01-01", 24000, 3.0},
			{"CARRIER-JNE", "Jakarta", "Medan", 0, 100, 0, 1.0, 48000, "KG", "IDR", "2026-01-01", 48000, 4.5},
			{"CARRIER-JNE", "Jakarta", "Balikpapan", 0, 100, 0, 1.0, 55000, "KG", "IDR", "2026-01-01", 55000, 5.0},
			// Dakota Cargo rates (Heavy industrial pallets & bulk hardware)
			{"CARRIER-DAKOTA", "Jakarta", "Surabaya", 50, 1000, 0.5, 10.0, 3500, "KG", "IDR", "2026-01-01", 175000, 4.0},
			{"CARRIER-DAKOTA", "Jakarta", "Bandung", 50, 1000, 0.5, 10.0, 2200, "KG", "IDR", "2026-01-01", 110000, 3.0},
			{"CARRIER-DAKOTA", "Jakarta", "Semarang", 50, 1000, 0.5, 10.0, 2800, "KG", "IDR", "2026-01-01", 140000, 3.5},
			{"CARRIER-DAKOTA", "Cikarang", "Jakarta", 20, 1000, 0.2, 10.0, 1500, "KG", "IDR", "2026-01-01", 75000, 2.0},
		}

		for _, rc := range rateCards {
			cid := carrierIDs[rc.carrierCode]
			effDate := ParseDate(rc.effFrom)
			_, err := tx.Exec(ctx, `
				INSERT INTO carrier_rate_cards (
					company_id, carrier_id, route_from_city, route_to_city,
					weight_from, weight_to, volume_from, volume_to,
					rate_per_unit, rate_unit, currency, effective_from,
					minimum_charge, fuel_surcharge_pct, created_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
				ON CONFLICT (company_id, carrier_id, route_from_city, route_to_city, weight_from, weight_to, effective_from)
				DO NOTHING`,
				sctx.CompanyNTPID, cid, rc.fromCity, rc.toCity,
				rc.weightFrom, rc.weightTo, rc.volFrom, rc.volTo,
				rc.rate, rc.unit, rc.currency, effDate,
				rc.minCharge, rc.fuelPct,
			)
			if err != nil {
				return fmt.Errorf("insert rate card %s -> %s: %w", rc.fromCity, rc.toCity, err)
			}
		}

		// -------------------------------------------------------------------------
		// 3. Fleets (2 Fleet Divisions)
		// -------------------------------------------------------------------------
		type fleetDef struct {
			code        string
			name        string
			fleetType   string
			status      string
			warehouseID int64
			homeCity    string
			notes       string
		}

		fleets := []fleetDef{
			{
				code:        "FLT-JKT-01",
				name:        "Armada Distribusi Jabodetabek",
				fleetType:   "OWN",
				status:      "ACTIVE",
				warehouseID: whJktFG,
				homeCity:    "Jakarta",
				notes:       "Armada kendaraan operasional distribusi Jakarta-Banten-Jawa Barat",
			},
			{
				code:        "FLT-CKR-02",
				name:        "Armada Logistik Jawa-Bali",
				fleetType:   "OWN",
				status:      "ACTIVE",
				warehouseID: whCkrFG,
				homeCity:    "Cikarang",
				notes:       "Armada angkutan antar-kota koridor Jawa Tengah dan Jawa Timur",
			},
		}

		fleetIDs := make(map[string]int64)
		for _, f := range fleets {
			var fid int64
			err := tx.QueryRow(ctx, `
				INSERT INTO fleets (
					company_id, fleet_name, fleet_code, fleet_type, status,
					warehouse_id, home_city, notes, created_at, updated_at, created_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW(), $9)
				ON CONFLICT (company_id, fleet_code) DO UPDATE SET
					fleet_name = EXCLUDED.fleet_name,
					fleet_type = EXCLUDED.fleet_type,
					status = EXCLUDED.status,
					warehouse_id = EXCLUDED.warehouse_id,
					home_city = EXCLUDED.home_city,
					notes = EXCLUDED.notes,
					updated_at = NOW()
				RETURNING id`,
				sctx.CompanyNTPID, f.name, f.code, f.fleetType, f.status,
				f.warehouseID, f.homeCity, f.notes, adminID,
			).Scan(&fid)
			if err != nil {
				return fmt.Errorf("upsert fleet %q: %w", f.code, err)
			}
			fleetIDs[f.code] = fid
		}

		// -------------------------------------------------------------------------
		// 4. Vehicles (3 Fleet Vehicles)
		// -------------------------------------------------------------------------
		type vehicleDef struct {
			regNumber    string
			fleetCode    string
			vehType      string
			status       string
			maxWeightKg  float64
			maxVolCbm    float64
			licensePlate string
			vin          string
			make         string
			model        string
			year         int
			lastMaint    time.Time
			nextMaint    time.Time
			insExpiry    time.Time
			gpsID        string
			notes        string
		}

		vehicles := []vehicleDef{
			{
				regNumber:    "VEH-TRK-001",
				fleetCode:    "FLT-JKT-01",
				vehType:      "TRUCK",
				status:       "AVAILABLE",
				maxWeightKg:  5000.00,
				maxVolCbm:    18.50,
				licensePlate: "B 9123 NTP",
				vin:          "MHKM1B1234K001001",
				make:         "Mitsubishi Fuso",
				model:        "Canter FE 74 HD 136PS",
				year:         2023,
				lastMaint:    time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC),
				nextMaint:    time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
				insExpiry:    time.Date(2027, 2, 28, 0, 0, 0, 0, time.UTC),
				gpsID:        "GPS-NTP-TRK01",
				notes:        "Truk box pendingin aluminium 6-roda untuk distribusi Jabodetabek",
			},
			{
				regNumber:    "VEH-TRK-002",
				fleetCode:    "FLT-JKT-01",
				vehType:      "TRUCK",
				status:       "IN_USE",
				maxWeightKg:  5000.00,
				maxVolCbm:    18.50,
				licensePlate: "B 9456 NTP",
				vin:          "MHKM1B1234K001002",
				make:         "Hino",
				model:        "Dutro 130 HD X-Power",
				year:         2024,
				lastMaint:    time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC),
				nextMaint:    time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				insExpiry:    time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC),
				gpsID:        "GPS-NTP-TRK02",
				notes:        "Truk box tertutup heavy duty untuk distribusi Jawa Barat & Banten",
			},
			{
				regNumber:    "VEH-VAN-001",
				fleetCode:    "FLT-JKT-01",
				vehType:      "VAN",
				status:       "AVAILABLE",
				maxWeightKg:  1200.00,
				maxVolCbm:    5.20,
				licensePlate: "B 9789 NTP",
				vin:          "MHKV2C5678K002001",
				make:         "Daihatsu",
				model:        "Gran Max Blind Van 1.5 AC/PS",
				year:         2024,
				lastMaint:    time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC),
				nextMaint:    time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
				insExpiry:    time.Date(2027, 4, 30, 0, 0, 0, 0, time.UTC),
				gpsID:        "GPS-NTP-VAN01",
				notes:        "Blind van lincah untuk pengiriman cepat komponen IoT perkotaan",
			},
		}

		vehicleIDs := make(map[string]int64)
		for _, v := range vehicles {
			fid := fleetIDs[v.fleetCode]
			var vid int64
			err := tx.QueryRow(ctx, `
				INSERT INTO vehicles (
					company_id, fleet_id, vehicle_registration, vehicle_type, status,
					max_weight_kg, max_volume_cbm, license_plate, vin, make, model,
					year_manufactured, last_maintenance_at, next_maintenance_due,
					insurance_expires_at, gps_device_id, notes,
					created_at, updated_at, created_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW(), NOW(), $18)
				ON CONFLICT (company_id, vehicle_registration) DO UPDATE SET
					fleet_id = EXCLUDED.fleet_id,
					vehicle_type = EXCLUDED.vehicle_type,
					status = EXCLUDED.status,
					max_weight_kg = EXCLUDED.max_weight_kg,
					max_volume_cbm = EXCLUDED.max_volume_cbm,
					license_plate = EXCLUDED.license_plate,
					vin = EXCLUDED.vin,
					make = EXCLUDED.make,
					model = EXCLUDED.model,
					year_manufactured = EXCLUDED.year_manufactured,
					last_maintenance_at = EXCLUDED.last_maintenance_at,
					next_maintenance_due = EXCLUDED.next_maintenance_due,
					insurance_expires_at = EXCLUDED.insurance_expires_at,
					gps_device_id = EXCLUDED.gps_device_id,
					notes = EXCLUDED.notes,
					updated_at = NOW()
				RETURNING id`,
				sctx.CompanyNTPID, fid, v.regNumber, v.vehType, v.status,
				v.maxWeightKg, v.maxVolCbm, v.licensePlate, v.vin, v.make, v.model,
				v.year, v.lastMaint, v.nextMaint, v.insExpiry, v.gpsID, v.notes, adminID,
			).Scan(&vid)
			if err != nil {
				return fmt.Errorf("upsert vehicle %q: %w", v.regNumber, err)
			}
			vehicleIDs[v.regNumber] = vid
		}

		// -------------------------------------------------------------------------
		// 5. Commercial Drivers (2 Drivers)
		// -------------------------------------------------------------------------
		type driverDef struct {
			code         string
			name         string
			status       string
			email        string
			phone        string
			licNum       string
			licClass     string
			licExpiry    time.Time
			emgName      string
			emgPhone     string
			notes        string
		}

		drivers := []driverDef{
			{
				code:      "DRV-001",
				name:      "Eko Prasetyo",
				status:    "ACTIVE",
				email:     "eko.prasetyo@nusantarateknik.co.id",
				phone:     "+6281211223344",
				licNum:    "SIM-B2-9102384729",
				licClass:  "B",
				licExpiry: time.Date(2028, 5, 15, 0, 0, 0, 0, time.UTC),
				emgName:   "Tri Wahyuni (Istri)",
				emgPhone:  "+6281211223345",
				notes:     "Senior commercial heavy truck driver with defensive driving certificate",
			},
			{
				code:      "DRV-002",
				name:      "Hendro Siswanto",
				status:    "ACTIVE",
				email:     "hendro.siswanto@nusantarateknik.co.id",
				phone:     "+6281222334455",
				licNum:    "SIM-B1-8201938471",
				licClass:  "B",
				licExpiry: time.Date(2027, 11, 20, 0, 0, 0, 0, time.UTC),
				emgName:   "Endang Lestari (Istri)",
				emgPhone:  "+6281222334456",
				notes:     "Commercial delivery driver certified for delicate electronics distribution",
			},
		}

		driverIDs := make(map[string]int64)
		for _, d := range drivers {
			var did int64
			err := tx.QueryRow(ctx, `
				INSERT INTO drivers (
					company_id, driver_name, driver_code, status, email, phone,
					license_number, license_class, license_expires_at,
					emergency_contact_name, emergency_contact_phone, notes,
					created_at, updated_at, created_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW(), $13)
				ON CONFLICT (company_id, driver_code) DO UPDATE SET
					driver_name = EXCLUDED.driver_name,
					status = EXCLUDED.status,
					email = EXCLUDED.email,
					phone = EXCLUDED.phone,
					license_number = EXCLUDED.license_number,
					license_class = EXCLUDED.license_class,
					license_expires_at = EXCLUDED.license_expires_at,
					emergency_contact_name = EXCLUDED.emergency_contact_name,
					emergency_contact_phone = EXCLUDED.emergency_contact_phone,
					notes = EXCLUDED.notes,
					updated_at = NOW()
				RETURNING id`,
				sctx.CompanyNTPID, d.name, d.code, d.status, d.email, d.phone,
				d.licNum, d.licClass, d.licExpiry,
				d.emgName, d.emgPhone, d.notes, adminID,
			).Scan(&did)
			if err != nil {
				return fmt.Errorf("upsert driver %q: %w", d.code, err)
			}
			driverIDs[d.code] = did
		}

		// -------------------------------------------------------------------------
		// 6. Shipments (4 Shipments satisfying check constraint shipment_transport_assignment)
		// -------------------------------------------------------------------------
		// Check constraint rules:
		// (vehicle_id IS NOT NULL AND driver_id IS NOT NULL AND carrier_id IS NULL) OR
		// (vehicle_id IS NULL AND driver_id IS NULL AND carrier_id IS NOT NULL) OR
		// (vehicle_id IS NULL AND driver_id IS NULL AND carrier_id IS NULL)

		type shpLineDef struct {
			productSKU string
			qty        float64
			weightKg   float64
			volCbm     float64
			lotNum     string
			serials    []string
		}

		type shipmentDef struct {
			number          string
			status          string
			shipmentType    string
			originWH        int64
			destWH          *int64
			destAddress     string
			destCity        string
			destCountry     string
			destContactName string
			destContactPhone string
			vehicleCode     string
			driverCode      string
			carrierCode     string
			carrierService  *string
			plannedDispatch time.Time
			plannedDelivery time.Time
			actualDispatch  *time.Time
			actualDelivery  *time.Time
			totalWeightKg   float64
			totalVolCbm     float64
			freightCharge   float64
			freightCurrency string
			notes           string
			lines           []shpLineDef
		}

		tDispatch1 := time.Date(2026, 3, 25, 9, 15, 0, 0, time.UTC)
		tDelivery1 := time.Date(2026, 3, 25, 13, 45, 0, 0, time.UTC)

		tDispatch2 := time.Date(2026, 4, 10, 10, 30, 0, 0, time.UTC)
		tDelivery2 := time.Date(2026, 4, 12, 15, 20, 0, 0, time.UTC)

		tDispatch3 := time.Date(2026, 8, 15, 8, 45, 0, 0, time.UTC)

		srvExpress := "EXPRESS"

		shipments := []shipmentDef{
			{
				// Assignment type 1: Own vehicle + driver
				number:          "SHP-202603-0001",
				status:          "DELIVERED",
				shipmentType:    "DELIVERY",
				originWH:        whJktFG,
				destWH:          nil,
				destAddress:     "Jl. Medan Merdeka Barat No. 21, Gambir",
				destCity:        "Jakarta Pusat",
				destCountry:     "Indonesia",
				destContactName: "Ir. Bambang Setyono",
				destContactPhone: "+62213800001",
				vehicleCode:     "VEH-TRK-001",
				driverCode:      "DRV-001",
				carrierCode:     "",
				carrierService:  nil,
				plannedDispatch: time.Date(2026, 3, 25, 9, 0, 0, 0, time.UTC),
				plannedDelivery: time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC),
				actualDispatch:  &tDispatch1,
				actualDelivery:  &tDelivery1,
				totalWeightKg:   125.00,
				totalVolCbm:     0.85,
				freightCharge:   0.00,
				freightCurrency: "IDR",
				notes:           "Direct fleet delivery of 50 IoT Gateway Pro units to PT Telkom Infrastruktur (Ref DO-202603-00001)",
				lines: []shpLineDef{
					{productSKU: "FG-IOT-GW01", qty: 50, weightKg: 100.00, volCbm: 0.70, lotNum: "LOT-202603-GW01", serials: []string{"GW-2026-0001", "GW-2026-0002"}},
					{productSKU: "RM-ANT-LORA", qty: 50, weightKg: 25.00, volCbm: 0.15, lotNum: "LOT-202603-ANT01", serials: []string{}},
				},
			},
			{
				// Assignment type 2: External carrier only
				number:          "SHP-202604-0002",
				status:          "DELIVERED",
				shipmentType:    "DELIVERY",
				originWH:        whJktFG,
				destWH:          nil,
				destAddress:     "Jl. Pemuda No. 88, Gubeng",
				destCity:        "Surabaya",
				destCountry:     "Indonesia",
				destContactName: "Agus Purnomo",
				destContactPhone: "+62315340001",
				vehicleCode:     "",
				driverCode:      "",
				carrierCode:     "CARRIER-JNE",
				carrierService:  &srvExpress,
				plannedDispatch: time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC),
				plannedDelivery: time.Date(2026, 4, 12, 16, 0, 0, 0, time.UTC),
				actualDispatch:  &tDispatch2,
				actualDelivery:  &tDelivery2,
				totalWeightKg:   210.00,
				totalVolCbm:     1.40,
				freightCharge:   1850000.00,
				freightCurrency: "IDR",
				notes:           "JNE Express freight for Power Meters & Environmental Monitors to PT PLN Nusantara Power (Ref DO-202603-00002)",
				lines: []shpLineDef{
					{productSKU: "FG-IOT-PWR01", qty: 100, weightKg: 150.00, volCbm: 1.00, lotNum: "LOT-202604-PWR01", serials: []string{"PWR-2026-0101", "PWR-2026-0102"}},
					{productSKU: "FG-IOT-ENV01", qty: 40, weightKg: 60.00, volCbm: 0.40, lotNum: "LOT-202604-ENV01", serials: []string{"ENV-2026-0041", "ENV-2026-0042"}},
				},
			},
			{
				// Assignment type 1: Own vehicle + driver (In transit)
				number:          "SHP-202608-0003",
				status:          "IN_TRANSIT",
				shipmentType:    "DELIVERY",
				originWH:        whJktFG,
				destWH:          nil,
				destAddress:     "Kawasan Industri GIIC Blok AA No. 5, Kota Deltamas",
				destCity:        "Cikarang",
				destCountry:     "Indonesia",
				destContactName: "Hadi Santoso",
				destContactPhone: "+62218990001",
				vehicleCode:     "VEH-TRK-002",
				driverCode:      "DRV-002",
				carrierCode:     "",
				carrierService:  nil,
				plannedDispatch: time.Date(2026, 8, 15, 8, 30, 0, 0, time.UTC),
				plannedDelivery: time.Date(2026, 8, 15, 15, 0, 0, 0, time.UTC),
				actualDispatch:  &tDispatch3,
				actualDelivery:  nil,
				totalWeightKg:   95.00,
				totalVolCbm:     0.65,
				freightCharge:   0.00,
				freightCurrency: "IDR",
				notes:           "Dedicated truck delivery for smart power meter devices to PT Astra Otoparts (Ref DO-202605-00005)",
				lines: []shpLineDef{
					{productSKU: "FG-IOT-PWR01", qty: 40, weightKg: 60.00, volCbm: 0.40, lotNum: "LOT-202608-PWR02", serials: []string{"PWR-2026-0201", "PWR-2026-0202"}},
					{productSKU: "FG-IOT-FLT01", qty: 25, weightKg: 35.00, volCbm: 0.25, lotNum: "LOT-202608-FLT01", serials: []string{"FLT-2026-0301", "FLT-2026-0302"}},
				},
			},
			{
				// Assignment type 3: Neither vehicle nor carrier (Draft planning)
				number:          "SHP-202608-0004",
				status:          "DRAFT",
				shipmentType:    "DELIVERY",
				originWH:        whJktFG,
				destWH:          nil,
				destAddress:     "Jl. Soekarno-Hatta No. 543",
				destCity:        "Bandung",
				destCountry:     "Indonesia",
				destContactName: "Dra. Maya Indriati",
				destContactPhone: "+62226010001",
				vehicleCode:     "",
				driverCode:      "",
				carrierCode:     "",
				carrierService:  nil,
				plannedDispatch: time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC),
				plannedDelivery: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
				actualDispatch:  nil,
				actualDelivery:  nil,
				totalWeightKg:   50.00,
				totalVolCbm:     0.40,
				freightCharge:   450000.00,
				freightCurrency: "IDR",
				notes:           "Draft planned shipment for precision agriculture sensor nodes to PT Petrokimia Gresik regional depot",
				lines: []shpLineDef{
					{productSKU: "FG-IOT-AGR01", qty: 20, weightKg: 50.00, volCbm: 0.40, lotNum: "LOT-202608-AGR01", serials: []string{}},
				},
			},
		}

		shipmentIDs := make(map[string]int64)
		for _, s := range shipments {
			var vehID *int64
			if s.vehicleCode != "" {
				v := vehicleIDs[s.vehicleCode]
				vehID = &v
			}
			var drvID *int64
			if s.driverCode != "" {
				d := driverIDs[s.driverCode]
				drvID = &d
			}
			var carID *int64
			if s.carrierCode != "" {
				c := carrierIDs[s.carrierCode]
				carID = &c
			}

			var shpID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO shipments (
					company_id, shipment_number, status, shipment_type,
					origin_warehouse_id, destination_warehouse_id,
					destination_address, destination_city, destination_country,
					destination_contact_name, destination_contact_phone,
					vehicle_id, driver_id, carrier_id, carrier_service_type,
					planned_dispatch_at, planned_delivery_at,
					actual_dispatch_at, actual_delivery_at,
					total_weight_kg, total_volume_cbm, freight_charge, freight_currency,
					notes, created_at, updated_at, created_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, NOW(), NOW(), $25)
				ON CONFLICT (company_id, shipment_number) DO UPDATE SET
					status = EXCLUDED.status,
					shipment_type = EXCLUDED.shipment_type,
					origin_warehouse_id = EXCLUDED.origin_warehouse_id,
					destination_warehouse_id = EXCLUDED.destination_warehouse_id,
					destination_address = EXCLUDED.destination_address,
					destination_city = EXCLUDED.destination_city,
					destination_country = EXCLUDED.destination_country,
					destination_contact_name = EXCLUDED.destination_contact_name,
					destination_contact_phone = EXCLUDED.destination_contact_phone,
					vehicle_id = EXCLUDED.vehicle_id,
					driver_id = EXCLUDED.driver_id,
					carrier_id = EXCLUDED.carrier_id,
					carrier_service_type = EXCLUDED.carrier_service_type,
					planned_dispatch_at = EXCLUDED.planned_dispatch_at,
					planned_delivery_at = EXCLUDED.planned_delivery_at,
					actual_dispatch_at = EXCLUDED.actual_dispatch_at,
					actual_delivery_at = EXCLUDED.actual_delivery_at,
					total_weight_kg = EXCLUDED.total_weight_kg,
					total_volume_cbm = EXCLUDED.total_volume_cbm,
					freight_charge = EXCLUDED.freight_charge,
					freight_currency = EXCLUDED.freight_currency,
					notes = EXCLUDED.notes,
					updated_at = NOW()
				RETURNING id`,
				sctx.CompanyNTPID, s.number, s.status, s.shipmentType,
				s.originWH, s.destWH,
				s.destAddress, s.destCity, s.destCountry,
				s.destContactName, s.destContactPhone,
				vehID, drvID, carID, s.carrierService,
				s.plannedDispatch, s.plannedDelivery,
				s.actualDispatch, s.actualDelivery,
				s.totalWeightKg, s.totalVolCbm, s.freightCharge, s.freightCurrency,
				s.notes, warehouseLeadID,
			).Scan(&shpID)
			if err != nil {
				return fmt.Errorf("upsert shipment %q: %w", s.number, err)
			}
			shipmentIDs[s.number] = shpID

			// Insert shipment lines
			for _, line := range s.lines {
				prodID := sctx.ProductIDs[line.productSKU]
				if prodID == 0 {
					return fmt.Errorf("product %q for shipment line not found", line.productSKU)
				}

				// Clean previous lines for idempotency
				_, _ = tx.Exec(ctx, `DELETE FROM shipment_lines WHERE shipment_id = $1 AND product_id = $2`, shpID, prodID)

				_, err := tx.Exec(ctx, `
					INSERT INTO shipment_lines (
						company_id, shipment_id, product_id, quantity,
						weight_kg, volume_cbm, lot_number, serial_numbers, created_at
					)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())`,
					sctx.CompanyNTPID, shpID, prodID, line.qty,
					line.weightKg, line.volCbm, line.lotNum, line.serials,
				)
				if err != nil {
					return fmt.Errorf("insert shipment line %s for shipment %s: %w", line.productSKU, s.number, err)
				}
			}
		}

		// -------------------------------------------------------------------------
		// 7. Trips & Trip Stops (2 Transport Execution Trips)
		// -------------------------------------------------------------------------
		type stopDef struct {
			seq         int
			stopType    string
			warehouseID *int64
			address     string
			city        string
			lat         float64
			lon         float64
			contactName string
			contactPh   string
			plannedArr  time.Time
			actualArr   *time.Time
			actualDep   *time.Time
			shipmentNum string
			notes       string
		}

		type tripDef struct {
			number       string
			status       string
			vehicleCode  string
			driverCode   string
			fleetCode    string
			originWH     int64
			plannedStart time.Time
			plannedEnd   time.Time
			actualStart  *time.Time
			actualEnd    *time.Time
			distanceKm   float64
			fuelLiters   float64
			notes        string
			stops        []stopDef
		}

		tStart1 := time.Date(2026, 3, 25, 8, 45, 0, 0, time.UTC)
		tEnd1 := time.Date(2026, 3, 25, 14, 30, 0, 0, time.UTC)

		tStart2 := time.Date(2026, 8, 15, 8, 15, 0, 0, time.UTC)

		tStopArr1_1 := time.Date(2026, 3, 25, 8, 45, 0, 0, time.UTC)
		tStopDep1_1 := time.Date(2026, 3, 25, 9, 15, 0, 0, time.UTC)
		tStopArr1_2 := time.Date(2026, 3, 25, 13, 15, 0, 0, time.UTC)
		tStopDep1_2 := time.Date(2026, 3, 25, 14, 0, 0, 0, time.UTC)

		tStopArr2_1 := time.Date(2026, 8, 15, 8, 15, 0, 0, time.UTC)
		tStopDep2_1 := time.Date(2026, 8, 15, 8, 45, 0, 0, time.UTC)

		trips := []tripDef{
			{
				number:       "TRP-202603-0001",
				status:       "COMPLETED",
				vehicleCode:  "VEH-TRK-001",
				driverCode:   "DRV-001",
				fleetCode:    "FLT-JKT-01",
				originWH:     whJktFG,
				plannedStart: time.Date(2026, 3, 25, 8, 30, 0, 0, time.UTC),
				plannedEnd:   time.Date(2026, 3, 25, 15, 0, 0, 0, time.UTC),
				actualStart:  &tStart1,
				actualEnd:    &tEnd1,
				distanceKm:   45.5,
				fuelLiters:   12.5,
				notes:        "Jakarta central distribution delivery run to Telkom Landmark Tower & Gambir",
				stops: []stopDef{
					{
						seq:         1,
						stopType:    "PICKUP",
						warehouseID: &whJktFG,
						address:     "Kawasan Industri Pulogadung Jl. Rawa Gelam IV No. 8",
						city:        "Jakarta Timur",
						lat:         -6.192834,
						lon:         106.912345,
						contactName: "Dewi Lestari",
						contactPh:   "+628155667788",
						plannedArr:  time.Date(2026, 3, 25, 8, 30, 0, 0, time.UTC),
						actualArr:   &tStopArr1_1,
						actualDep:   &tStopDep1_1,
						shipmentNum: "SHP-202603-0001",
						notes:       "Loaded 50 units IoT Gateway Pro & accessories into Box Truck B 9123 NTP",
					},
					{
						seq:         2,
						stopType:    "DELIVERY",
						warehouseID: nil,
						address:     "Jl. Medan Merdeka Barat No. 21, Gambir",
						city:        "Jakarta Pusat",
						lat:         -6.175392,
						lon:         106.827153,
						contactName: "Ir. Bambang Setyono",
						contactPh:   "+62213800001",
						plannedArr:  time.Date(2026, 3, 25, 13, 0, 0, 0, time.UTC),
						actualArr:   &tStopArr1_2,
						actualDep:   &tStopDep1_2,
						shipmentNum: "SHP-202603-0001",
						notes:       "Delivered goods signed and stamped by PT Telkom Infrastruktur receiving team",
					},
				},
			},
			{
				number:       "TRP-202608-0002",
				status:       "IN_PROGRESS",
				vehicleCode:  "VEH-TRK-002",
				driverCode:   "DRV-002",
				fleetCode:    "FLT-JKT-01",
				originWH:     whJktFG,
				plannedStart: time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC),
				plannedEnd:   time.Date(2026, 8, 15, 16, 0, 0, 0, time.UTC),
				actualStart:  &tStart2,
				actualEnd:    nil,
				distanceKm:   62.0,
				fuelLiters:   15.0,
				notes:        "Jabodetabek corridor industrial delivery run to Cikarang GIIC industrial estate",
				stops: []stopDef{
					{
						seq:         1,
						stopType:    "PICKUP",
						warehouseID: &whJktFG,
						address:     "Kawasan Industri Pulogadung Jl. Rawa Gelam IV No. 8",
						city:        "Jakarta Timur",
						lat:         -6.192834,
						lon:         106.912345,
						contactName: "Dewi Lestari",
						contactPh:   "+628155667788",
						plannedArr:  time.Date(2026, 8, 15, 8, 0, 0, 0, time.UTC),
						actualArr:   &tStopArr2_1,
						actualDep:   &tStopDep2_1,
						shipmentNum: "SHP-202608-0003",
						notes:       "Loaded 40 units Power Meter & 25 Fleet GPS OBD trackers into Hino Dutro B 9456 NTP",
					},
					{
						seq:         2,
						stopType:    "DELIVERY",
						warehouseID: nil,
						address:     "Kawasan Industri GIIC Blok AA No. 5, Kota Deltamas",
						city:        "Cikarang",
						lat:         -6.368912,
						lon:         107.165432,
						contactName: "Hadi Santoso",
						contactPh:   "+62218990001",
						plannedArr:  time.Date(2026, 8, 15, 14, 30, 0, 0, time.UTC),
						actualArr:   nil,
						actualDep:   nil,
						shipmentNum: "SHP-202608-0003",
						notes:       "En-route to PT Astra Otoparts receiving dock via Jakarta-Cikampek toll road",
					},
				},
			},
		}

		for _, t := range trips {
			vid := vehicleIDs[t.vehicleCode]
			did := driverIDs[t.driverCode]
			fid := fleetIDs[t.fleetCode]

			var tripID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO trips (
					company_id, trip_number, status, vehicle_id, driver_id, fleet_id,
					origin_warehouse_id, planned_start_at, planned_end_at,
					actual_start_at, actual_end_at, total_distance_km, fuel_used_liters,
					notes, created_at, updated_at, created_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW(), NOW(), $15)
				ON CONFLICT (company_id, trip_number) DO UPDATE SET
					status = EXCLUDED.status,
					vehicle_id = EXCLUDED.vehicle_id,
					driver_id = EXCLUDED.driver_id,
					fleet_id = EXCLUDED.fleet_id,
					origin_warehouse_id = EXCLUDED.origin_warehouse_id,
					planned_start_at = EXCLUDED.planned_start_at,
					planned_end_at = EXCLUDED.planned_end_at,
					actual_start_at = EXCLUDED.actual_start_at,
					actual_end_at = EXCLUDED.actual_end_at,
					total_distance_km = EXCLUDED.total_distance_km,
					fuel_used_liters = EXCLUDED.fuel_used_liters,
					notes = EXCLUDED.notes,
					updated_at = NOW()
				RETURNING id`,
				sctx.CompanyNTPID, t.number, t.status, vid, did, fid,
				t.originWH, t.plannedStart, t.plannedEnd,
				t.actualStart, t.actualEnd, t.distanceKm, t.fuelLiters,
				t.notes, warehouseLeadID,
			).Scan(&tripID)
			if err != nil {
				return fmt.Errorf("upsert trip %q: %w", t.number, err)
			}

			// Insert trip stops
			for _, stop := range t.stops {
				var shpID *int64
				if stop.shipmentNum != "" {
					sid := shipmentIDs[stop.shipmentNum]
					if sid > 0 {
						shpID = &sid
					}
				}

				_, err := tx.Exec(ctx, `
					INSERT INTO trip_stops (
						company_id, trip_id, shipment_id, stop_sequence, stop_type,
						warehouse_id, location_address, location_city,
						location_lat, location_lon, contact_name, contact_phone,
						planned_arrival_at, actual_arrival_at, actual_departure_at,
						notes, created_at
					)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, NOW())
					ON CONFLICT (company_id, trip_id, stop_sequence) DO UPDATE SET
						shipment_id = EXCLUDED.shipment_id,
						stop_type = EXCLUDED.stop_type,
						warehouse_id = EXCLUDED.warehouse_id,
						location_address = EXCLUDED.location_address,
						location_city = EXCLUDED.location_city,
						location_lat = EXCLUDED.location_lat,
						location_lon = EXCLUDED.location_lon,
						contact_name = EXCLUDED.contact_name,
						contact_phone = EXCLUDED.contact_phone,
						planned_arrival_at = EXCLUDED.planned_arrival_at,
						actual_arrival_at = EXCLUDED.actual_arrival_at,
						actual_departure_at = EXCLUDED.actual_departure_at,
						notes = EXCLUDED.notes`,
					sctx.CompanyNTPID, tripID, shpID, stop.seq, stop.stopType,
					stop.warehouseID, stop.address, stop.city,
					stop.lat, stop.lon, stop.contactName, stop.contactPh,
					stop.plannedArr, stop.actualArr, stop.actualDep,
					stop.notes,
				)
				if err != nil {
					return fmt.Errorf("upsert trip stop seq %d for trip %s: %w", stop.seq, t.number, err)
				}
			}
		}

		_ = whSbyDist
		return nil
	})
}
