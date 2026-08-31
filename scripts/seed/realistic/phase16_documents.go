package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// seedPhase16Documents seeds storage blobs with clean malware scan statuses and SHA-256 checksums,
// document classifications (PUBLIC, INTERNAL, CONFIDENTIAL, RESTRICTED), document categories,
// 5 controlled documents with immutable versions, and properly resolves the circular FK current_version_id.
func seedPhase16Documents(ctx context.Context, sctx *SeedContext) error {
	return ExecInTx(ctx, sctx.Pool, "Phase 16: Document Management & Object Storage", func(tx pgx.Tx) error {
		adminID := sctx.UserIDs["budi.santoso@nusantarateknik.co.id"]
		if adminID == 0 {
			return fmt.Errorf("admin user budi.santoso@nusantarateknik.co.id not found")
		}
		qaLeadID := sctx.UserIDs["ratna.sari@nusantarateknik.co.id"]
		if qaLeadID == 0 {
			qaLeadID = adminID
		}
		prodLeadID := sctx.UserIDs["joko.prasetyo@nusantarateknik.co.id"]
		if prodLeadID == 0 {
			prodLeadID = adminID
		}
		procLeadID := sctx.UserIDs["agus.setiawan@nusantarateknik.co.id"]
		if procLeadID == 0 {
			procLeadID = adminID
		}

		// -------------------------------------------------------------------------
		// 1. Storage Blobs (5 Object Storage Blobs with SHA-256 and CLEAN scan status)
		// -------------------------------------------------------------------------
		type blobDef struct {
			key         string
			sizeBytes   int64
			checksum    string
			contentType string
			details     string
		}

		blobs := []blobDef{
			{
				key:         "docs/eng/NTP-GW01-SPEC-v2.1.pdf",
				sizeBytes:   2458900,
				checksum:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				contentType: "application/pdf",
				details:     `{"engine": "ClamAV", "definition": "2026.03.01", "verdict": "CLEAN"}`,
			},
			{
				key:         "docs/legal/MSA-2026-NTP-IFH-Signed.pdf",
				sizeBytes:   4120800,
				checksum:    "8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4",
				contentType: "application/pdf",
				details:     `{"engine": "ClamAV", "definition": "2026.03.01", "verdict": "CLEAN"}`,
			},
			{
				key:         "docs/qms/QM-ISO9001-2015-Rev4.2.pdf",
				sizeBytes:   6890400,
				checksum:    "a591a6d40bf420404a011733cfb7b190d62c65bf0bcda32b57b277d9ad9f146e",
				contentType: "application/pdf",
				details:     `{"engine": "ClamAV", "definition": "2026.03.01", "verdict": "CLEAN"}`,
			},
			{
				key:         "docs/sop/SOP-MFG-SMT-Reflow-v3.0.pdf",
				sizeBytes:   3250100,
				checksum:    "5e884898da28047151d0e56f8dc6292773603d0d6aabbdd62a11ef721d1542d8",
				contentType: "application/pdf",
				details:     `{"engine": "ClamAV", "definition": "2026.03.01", "verdict": "CLEAN"}`,
			},
			{
				key:         "docs/cert/ISO27001-2022-ISMS-Certificate.pdf",
				sizeBytes:   1850300,
				checksum:    "4b227777d4dd1fc61c6f884f48641d02b4d121d3fd328cb08b5531fcacdabf8a",
				contentType: "application/pdf",
				details:     `{"engine": "ClamAV", "definition": "2026.03.01", "verdict": "CLEAN"}`,
			},
		}

		blobIDs := make(map[string]int64)
		for _, b := range blobs {
			var bid int64
			err := tx.QueryRow(ctx, `
				INSERT INTO storage_blobs (
					company_id, storage_key, storage_driver, bucket, size_bytes,
					checksum_sha256, declared_content_type, detected_content_type,
					malware_scan_status, malware_scan_details, reference_count,
					created_at, created_by
				)
				VALUES ($1, $2, 'local', NULL, $3, $4, $5, $5, 'CLEAN', $6::jsonb, 1, NOW(), $7)
				ON CONFLICT (company_id, storage_key) DO UPDATE SET
					size_bytes = EXCLUDED.size_bytes,
					checksum_sha256 = EXCLUDED.checksum_sha256,
					malware_scan_status = 'CLEAN',
					malware_scan_details = EXCLUDED.malware_scan_details
				RETURNING id`,
				sctx.CompanyNTPID, b.key, b.sizeBytes, b.checksum, b.contentType, b.details, adminID,
			).Scan(&bid)
			if err != nil {
				return fmt.Errorf("upsert storage_blob %q: %w", b.key, err)
			}
			blobIDs[b.key] = bid
		}

		// -------------------------------------------------------------------------
		// 2. Document Classifications (PUBLIC, INTERNAL, CONFIDENTIAL, RESTRICTED)
		// -------------------------------------------------------------------------
		type classDef struct {
			code     string
			name     string
			desc     string
			reqAppr  bool
			reqSign  bool
			exts     []string
			maxSize  int64
			sortOrd  int
		}

		classifications := []classDef{
			{"PUBLIC", "Public Information", "Freely accessible documents suitable for external release", false, false, []string{"pdf", "png", "jpg"}, 10485760, 1},
			{"INTERNAL", "Internal Company Use", "Standard business documents accessible to all employees", false, false, []string{"pdf", "docx", "xlsx", "pptx"}, 52428800, 2},
			{"CONFIDENTIAL", "Confidential Business Information", "Restricted to authorized departmental personnel and management", true, false, []string{"pdf", "docx", "xlsx"}, 52428800, 3},
			{"RESTRICTED", "Strictly Restricted / Executive", "Highly sensitive executive, legal, compliance, and IP documents", true, true, []string{"pdf"}, 20971520, 4},
		}

		classIDs := make(map[string]int64)
		for _, c := range classifications {
			var cid int64
			err := tx.QueryRow(ctx, `
				INSERT INTO document_classifications (
					company_id, code, name, description, requires_approval,
					requires_signature, allowed_extensions, max_size_bytes,
					sort_order, active, created_at, created_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE, NOW(), $10)
				ON CONFLICT (company_id, code) DO UPDATE SET
					name = EXCLUDED.name,
					description = EXCLUDED.description,
					requires_approval = EXCLUDED.requires_approval,
					requires_signature = EXCLUDED.requires_signature,
					allowed_extensions = EXCLUDED.allowed_extensions,
					max_size_bytes = EXCLUDED.max_size_bytes,
					sort_order = EXCLUDED.sort_order,
					active = TRUE
				RETURNING id`,
				sctx.CompanyNTPID, c.code, c.name, c.desc, c.reqAppr,
				c.reqSign, c.exts, c.maxSize, c.sortOrd, adminID,
			).Scan(&cid)
			if err != nil {
				return fmt.Errorf("upsert document_classification %q: %w", c.code, err)
			}
			classIDs[c.code] = cid
		}

		// -------------------------------------------------------------------------
		// 3. Document Categories (5 Categories)
		// -------------------------------------------------------------------------
		type catDef struct {
			code        string
			name        string
			desc        string
			defaultCode string
		}

		categories := []catDef{
			{"ENG-SPEC", "Engineering & Technical Specifications", "Hardware schematics, firmware architectures, IoT device specifications", "INTERNAL"},
			{"LEGAL-AGR", "Legal & Commercial Agreements", "Master service agreements, vendor contracts, nondisclosure agreements", "CONFIDENTIAL"},
			{"QMS-MANUAL", "Quality Management System Manuals", "ISO 9001 quality manual, procedures, inspection standards, CAPA guidelines", "INTERNAL"},
			{"SOP-MFG", "Manufacturing & Assembly SOPs", "Standard operating procedures for SMT lines, testing, packaging, calibration", "INTERNAL"},
			{"CERT-ISO", "Regulatory & ISO Certifications", "Accredited ISO certifications, SDPPI type approvals, safety compliance", "RESTRICTED"},
		}

		catIDs := make(map[string]int64)
		for _, cat := range categories {
			defClassID := classIDs[cat.defaultCode]

			var catID int64
			err := tx.QueryRow(ctx, `
				SELECT id FROM document_categories
				WHERE company_id = $1 AND parent_id IS NULL AND code = $2`,
				sctx.CompanyNTPID, cat.code,
			).Scan(&catID)

			if err != nil {
				err = tx.QueryRow(ctx, `
					INSERT INTO document_categories (
						company_id, parent_id, code, name, description,
						default_classification_id, active, created_at, created_by
					)
					VALUES ($1, NULL, $2, $3, $4, $5, TRUE, NOW(), $6)
					RETURNING id`,
					sctx.CompanyNTPID, cat.code, cat.name, cat.desc, defClassID, adminID,
				).Scan(&catID)
			} else {
				_, err = tx.Exec(ctx, `
					UPDATE document_categories SET
						name = $1, description = $2,
						default_classification_id = $3, active = TRUE
					WHERE id = $4`,
					cat.name, cat.desc, defClassID, catID,
				)
			}
			if err != nil {
				return fmt.Errorf("upsert document_category %q: %w", cat.code, err)
			}
			catIDs[cat.code] = catID
		}

		// -------------------------------------------------------------------------
		// 4. Controlled Documents & Immutable Versions (5 Documents)
		// -------------------------------------------------------------------------
		type docDef struct {
			number      string
			title       string
			desc        string
			catCode     string
			classCode   string
			ownerID     int64
			status      string
			effFrom     time.Time
			blobKey     string
			verNum      int
			verLabel    string
			verStatus   string
			changeSum   string
			approverID  int64
		}

		tEffMarch := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		tEffApril := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		tEffMay := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		tEffJune := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

		controlledDocs := []docDef{
			{
				number:     "DOC-2026-001",
				title:      "Product Specification - Nusantara IoT Gateway Pro 4G/LoRaWAN (NTP-GW-01)",
				desc:       "Complete engineering hardware specification, pinout diagram, LoRaWAN RF parameters, 4G LTE bands, and environmental enclosure ratings",
				catCode:    "ENG-SPEC",
				classCode:  "INTERNAL",
				ownerID:    prodLeadID,
				status:     "ACTIVE",
				effFrom:    tEffMarch,
				blobKey:    "docs/eng/NTP-GW01-SPEC-v2.1.pdf",
				verNum:     1,
				verLabel:   "2.1",
				verStatus:  "EFFECTIVE",
				changeSum:  "Updated RF power transmission parameters and SDPPI compliance appendix",
				approverID: prodLeadID,
			},
			{
				number:     "DOC-2026-002",
				title:      "Master Supplier Agreement - PT Indo Fastener & Hardware (MSA-2026-NTP-IFH)",
				desc:       "Bilateral commercial terms, quality SLA, pricing schedule, warranty provisions, and annual delivery volume commitments",
				catCode:    "LEGAL-AGR",
				classCode:  "CONFIDENTIAL",
				ownerID:    procLeadID,
				status:     "ACTIVE",
				effFrom:    tEffApril,
				blobKey:    "docs/legal/MSA-2026-NTP-IFH-Signed.pdf",
				verNum:     1,
				verLabel:   "1.0",
				verStatus:  "EFFECTIVE",
				changeSum:  "Initial executed contract terms approved by Procurement Manager and Legal Counsel",
				approverID: procLeadID,
			},
			{
				number:     "DOC-2026-003",
				title:      "ISO 9001:2015 Quality Management System Manual Rev 4.2",
				desc:       "Core quality manual defining quality policy, organizational processes, risk assessment matrix, and audit compliance controls",
				catCode:    "QMS-MANUAL",
				classCode:  "INTERNAL",
				ownerID:    qaLeadID,
				status:     "ACTIVE",
				effFrom:    tEffMarch,
				blobKey:    "docs/qms/QM-ISO9001-2015-Rev4.2.pdf",
				verNum:     1,
				verLabel:   "4.2",
				verStatus:  "EFFECTIVE",
				changeSum:  "Annual QMS review incorporating automated ERP audit trail verification procedures",
				approverID: qaLeadID,
			},
			{
				number:     "DOC-2026-004",
				title:      "SMT Assembly & Reflow Soldering Standard Operating Procedure (SOP-PRD-012)",
				desc:       "Standard operating procedure for surface mount technology pick-and-place setup, solder paste inspection, and 10-zone reflow profile",
				catCode:    "SOP-MFG",
				classCode:  "INTERNAL",
				ownerID:    prodLeadID,
				status:     "ACTIVE",
				effFrom:    tEffMay,
				blobKey:    "docs/sop/SOP-MFG-SMT-Reflow-v3.0.pdf",
				verNum:     1,
				verLabel:   "3.0",
				verStatus:  "EFFECTIVE",
				changeSum:  "Refined lead-free SAC305 thermal profile settings for multi-layer PCB boards",
				approverID: prodLeadID,
			},
			{
				number:     "DOC-2026-005",
				title:      "ISO 27001:2022 Information Security Management System Certificate",
				desc:       "Accredited ISO/IEC 27001:2022 ISMS certificate issued by Lloyd's Register Quality Assurance covering ERP cloud infrastructure and IoT firmware development",
				catCode:    "CERT-ISO",
				classCode:  "RESTRICTED",
				ownerID:    adminID,
				status:     "ACTIVE",
				effFrom:    tEffJune,
				blobKey:    "docs/cert/ISO27001-2022-ISMS-Certificate.pdf",
				verNum:     1,
				verLabel:   "1.0",
				verStatus:  "EFFECTIVE",
				changeSum:  "Triennial certification issuance following clean surveillance audit",
				approverID: adminID,
			},
		}

		for _, d := range controlledDocs {
			catID := catIDs[d.catCode]
			clsID := classIDs[d.classCode]
			blobID := blobIDs[d.blobKey]
			if blobID == 0 {
				return fmt.Errorf("storage blob %q for document %q not found", d.blobKey, d.number)
			}

			// Step A: Insert or update document header with current_version_id = NULL initially to avoid circular constraint
			var docID int64
			err := tx.QueryRow(ctx, `
				INSERT INTO documents (
					company_id, category_id, classification_id, document_number,
					title, description, owner_id, status, effective_from,
					current_version_id, created_at, created_by, updated_at, updated_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, NOW(), $10, NOW(), $10)
				ON CONFLICT (company_id, document_number) DO UPDATE SET
					category_id = EXCLUDED.category_id,
					classification_id = EXCLUDED.classification_id,
					title = EXCLUDED.title,
					description = EXCLUDED.description,
					owner_id = EXCLUDED.owner_id,
					status = EXCLUDED.status,
					effective_from = EXCLUDED.effective_from,
					updated_at = NOW(),
					updated_by = EXCLUDED.updated_by
				RETURNING id`,
				sctx.CompanyNTPID, catID, clsID, d.number,
				d.title, d.desc, d.ownerID, d.status, d.effFrom, adminID,
			).Scan(&docID)
			if err != nil {
				return fmt.Errorf("upsert document %q: %w", d.number, err)
			}

			// Step B: Insert or update document_version
			var verID int64
			err = tx.QueryRow(ctx, `
				INSERT INTO document_versions (
					company_id, document_id, version_number, version_label, blob_id,
					status, classification_id, change_summary, approved_by,
					approved_at, effective_at, created_at, created_by
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, NOW(), $11)
				ON CONFLICT (company_id, document_id, version_number) DO UPDATE SET
					version_label = EXCLUDED.version_label,
					blob_id = EXCLUDED.blob_id,
					status = EXCLUDED.status,
					classification_id = EXCLUDED.classification_id,
					change_summary = EXCLUDED.change_summary,
					approved_by = EXCLUDED.approved_by,
					approved_at = EXCLUDED.approved_at,
					effective_at = EXCLUDED.effective_at
				RETURNING id`,
				sctx.CompanyNTPID, docID, d.verNum, d.verLabel, blobID,
				d.verStatus, clsID, d.changeSum, d.approverID,
				d.effFrom, adminID,
			).Scan(&verID)
			if err != nil {
				return fmt.Errorf("upsert document_version %s v%d: %w", d.number, d.verNum, err)
			}

			// Step C: Update document with current_version_id pointing to the version
			_, err = tx.Exec(ctx, `
				UPDATE documents SET current_version_id = $1, updated_at = NOW() WHERE id = $2`,
				verID, docID,
			)
			if err != nil {
				return fmt.Errorf("update current_version_id for document %q: %w", d.number, err)
			}
		}

		return nil
	})
}
