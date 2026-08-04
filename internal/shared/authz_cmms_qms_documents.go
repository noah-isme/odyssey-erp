package shared

// CMMS, QMS, and Document Management permissions introduced in the Phase 1–7
// module plan (docs/guides/missing-modules-cmms-qms-documents-plan.md).

const (
	// Documents module permissions
	PermDocumentsView            = "documents.view"
	PermDocumentsUpload          = "documents.upload"
	PermDocumentsVersion         = "documents.version"
	PermDocumentsReview          = "documents.review"
	PermDocumentsApprove         = "documents.approve"
	PermDocumentsSign            = "documents.sign"
	PermDocumentsShare           = "documents.share"
	PermDocumentsRetentionManage = "documents.retention.manage"
	PermDocumentsHoldManage      = "documents.hold.manage"
	PermDocumentsDispose         = "documents.dispose"
	PermDocumentsAdmin           = "documents.admin"

	// CMMS module permissions
	PermCMMSAssetView       = "cmms.asset.view"
	PermCMMSAssetManage     = "cmms.asset.manage"
	PermCMMSRequestCreate   = "cmms.request.create"
	PermCMMSRequestTriage   = "cmms.request.triage"
	PermCMMSPlanView        = "cmms.plan.view"
	PermCMMSPlanManage      = "cmms.plan.manage"
	PermCMMSWorkOrderView    = "cmms.work_order.view"
	PermCMMSWorkOrderRelease = "cmms.work_order.release"
	PermCMMSWorkOrderExecute = "cmms.work_order.execute"
	PermCMMSWorkOrderClose   = "cmms.work_order.close"
	PermCMMSCostView        = "cmms.cost.view"
	PermCMMSCostApprove     = "cmms.cost.approve"
	PermCMMSAdmin           = "cmms.admin"

	// QMS module permissions
	PermQMSSpecificationView    = "qms.specification.view"
	PermQMSSpecificationManage  = "qms.specification.manage"
	PermQMSInspectionView       = "qms.inspection.view"
	PermQMSInspectionExecute    = "qms.inspection.execute"
	PermQMSHoldView             = "qms.hold.view"
	PermQMSHoldManage           = "qms.hold.manage"
	PermQMSNCRView              = "qms.ncr.view"
	PermQMSNCRCreate            = "qms.ncr.create"
	PermQMSNCRManage            = "qms.ncr.manage"
	PermQMSCAPAView             = "qms.capa.view"
	PermQMSCAPACreate           = "qms.capa.create"
	PermQMSCAPAManage           = "qms.capa.manage"
	PermQMSCAPAVerify           = "qms.capa.verify"
	PermQMSAuditView            = "qms.audit.view"
	PermQMSAuditManage          = "qms.audit.manage"
	PermQMSComplaintView        = "qms.complaint.view"
	PermQMSComplaintManage      = "qms.complaint.manage"
	PermQMSSupplierQualityView  = "qms.supplier_quality.view"
	PermQMSSupplierQualityManage = "qms.supplier_quality.manage"
	PermQMSAdmin                = "qms.admin"
)

// CMMSQMSDocumentsPermissions lists all permissions for the three new modules.
var CMMSQMSDocumentsPermissions = []PermissionSpec{
	{PermDocumentsView, "View managed documents"},
	{PermDocumentsUpload, "Upload new documents"},
	{PermDocumentsVersion, "Create new document versions"},
	{PermDocumentsReview, "Review and comment on documents"},
	{PermDocumentsApprove, "Approve document versions"},
	{PermDocumentsSign, "Electronically sign documents"},
	{PermDocumentsShare, "Create and manage document shares"},
	{PermDocumentsRetentionManage, "Manage document retention policies"},
	{PermDocumentsHoldManage, "Create and release legal holds"},
	{PermDocumentsDispose, "Execute approved document dispositions"},
	{PermDocumentsAdmin, "Administer document management settings"},
	{PermCMMSAssetView, "View CMMS assets and locations"},
	{PermCMMSAssetManage, "Create and edit CMMS assets"},
	{PermCMMSRequestCreate, "Create maintenance requests"},
	{PermCMMSRequestTriage, "Triage and schedule maintenance requests"},
	{PermCMMSPlanView, "View preventive maintenance plans"},
	{PermCMMSPlanManage, "Create and edit preventive maintenance plans"},
	{PermCMMSWorkOrderView, "View work orders"},
	{PermCMMSWorkOrderRelease, "Release work orders for execution"},
	{PermCMMSWorkOrderExecute, "Record labor, parts, and complete tasks"},
	{PermCMMSWorkOrderClose, "Close completed work orders"},
	{PermCMMSCostView, "View maintenance costs"},
	{PermCMMSCostApprove, "Approve cost exceptions"},
	{PermCMMSAdmin, "Administer CMMS module settings"},
	{PermQMSSpecificationView, "View quality specifications and inspection plans"},
	{PermQMSSpecificationManage, "Create and manage quality specifications"},
	{PermQMSInspectionView, "View inspections and results"},
	{PermQMSInspectionExecute, "Record inspection results"},
	{PermQMSHoldView, "View quality holds"},
	{PermQMSHoldManage, "Create and release quality holds"},
	{PermQMSNCRView, "View non-conformance reports"},
	{PermQMSNCRCreate, "Create non-conformance reports"},
	{PermQMSNCRManage, "Manage NCR disposition and closure"},
	{PermQMSCAPAView, "View corrective actions"},
	{PermQMSCAPACreate, "Create corrective actions"},
	{PermQMSCAPAManage, "Manage CAPA lifecycle"},
	{PermQMSCAPAVerify, "Verify CAPA effectiveness"},
	{PermQMSAuditView, "View quality audits"},
	{PermQMSAuditManage, "Plan and manage quality audits"},
	{PermQMSComplaintView, "View customer complaints"},
	{PermQMSComplaintManage, "Manage complaint investigations"},
	{PermQMSSupplierQualityView, "View supplier quality records"},
	{PermQMSSupplierQualityManage, "Manage supplier quality ratings"},
	{PermQMSAdmin, "Administer QMS module settings"},
}
