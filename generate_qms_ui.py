import os

base_dir = "/home/noah/project/odyssey-erp/web/templates/pages/qms"
os.makedirs(base_dir, exist_ok=True)

files = {
    # NCR Templates
    "ncrs.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-danger"><i class="bi bi-exclamation-triangle"></i> Non-Conformance Reports (NCR)</h1>
    <a href="/qms/ncrs/new" class="btn btn-danger shadow-sm"><i class="bi bi-plus-circle"></i> Log NCR</a>
  </div>

  <div class="card shadow-sm mb-4">
    <div class="card-body">
      <form method="get" class="row g-3">
        <div class="col-md-3">
          <select name="status" class="form-select">
            <option value="">All Statuses</option>
            {{ range .Statuses }}
            <option value="{{ . }}">{{ . }}</option>
            {{ end }}
          </select>
        </div>
        <div class="col-md-3">
          <select name="severity" class="form-select">
            <option value="">All Severities</option>
            {{ range .Severities }}
            <option value="{{ . }}">{{ . }}</option>
            {{ end }}
          </select>
        </div>
        <div class="col-md-2">
          <button type="submit" class="btn btn-outline-secondary w-100">Filter</button>
        </div>
      </form>
    </div>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Number</th>
            <th>Title</th>
            <th>Source</th>
            <th>Category</th>
            <th>Severity</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .NCRs }}
            {{ range .NCRs }}
            <tr>
              <td><strong>{{ .Number }}</strong></td>
              <td>{{ .Title }}</td>
              <td>{{ .SourceType }}</td>
              <td>{{ .Category }}</td>
              <td>
                {{ if eq .Severity "CRITICAL" }}<span class="badge bg-danger">Critical</span>
                {{ else if eq .Severity "MAJOR" }}<span class="badge bg-warning text-dark">Major</span>
                {{ else }}<span class="badge bg-secondary">Minor</span>{{ end }}
              </td>
              <td><span class="badge bg-info text-dark">{{ .Status }}</span></td>
              <td><a href="/qms/ncrs/{{ .ID }}" class="btn btn-sm btn-outline-danger">Review</a></td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="7" class="text-center text-muted py-4">No NCRs found.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "ncr_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm border-danger">
    <div class="card-header bg-danger text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-exclamation-triangle"></i> Log Non-Conformance</h2>
    </div>
    <div class="card-body p-4">
      <form action="/qms/ncrs" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="mb-3">
          <label class="form-label fw-bold">Title</label>
          <input type="text" name="title" class="form-control" required placeholder="Brief description of the defect/issue">
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Detailed Description</label>
          <textarea name="description" class="form-control" rows="4"></textarea>
        </div>
        
        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Category</label>
            <select name="category" class="form-select">
              {{ range .Categories }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Severity</label>
            <select name="severity" class="form-select">
              {{ range .Severities }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
        </div>

        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Source Type</label>
            <select name="source_type" class="form-select">
              {{ range .SourceTypes }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Detected Location</label>
            <input type="text" name="detected_location" class="form-control">
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/qms/ncrs" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-danger">Submit NCR</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "ncr_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-danger mb-1">NCR: {{ .NCR.Number }}</h1>
      <p class="text-muted">{{ .NCR.Title }}</p>
    </div>
    <a href="/qms/ncrs" class="btn btn-outline-secondary">Back to NCRs</a>
  </div>

  <div class="row">
    <div class="col-lg-8">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Defect Details</div>
        <div class="card-body">
          <p><strong>Description:</strong><br> {{ .NCR.Description }}</p>
          <div class="row mt-4">
            <div class="col-sm-6">
              <p><strong>Category:</strong> {{ .NCR.Category }}</p>
              <p><strong>Severity:</strong> <span class="badge bg-secondary">{{ .NCR.Severity }}</span></p>
            </div>
            <div class="col-sm-6">
              <p><strong>Source:</strong> {{ .NCR.SourceType }}</p>
              <p><strong>Location:</strong> {{ .NCR.DetectedLocation }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
    
    <div class="col-lg-4">
      <div class="card shadow-sm mb-4 border-info">
        <div class="card-header bg-info text-white fw-bold">Disposition & Status</div>
        <div class="card-body">
          <p class="mb-3">Current Status: <span class="badge bg-dark fs-6">{{ .NCR.Status }}</span></p>
          
          <form action="/qms/ncrs/{{ .NCR.ID }}/status" method="POST" class="mb-4">
            <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
            <label class="form-label fw-bold">Update Status</label>
            <div class="input-group">
              <select name="status" class="form-select">
                {{ range .Statuses }}<option value="{{ . }}">{{ . }}</option>{{ end }}
              </select>
              <button class="btn btn-primary" type="submit">Update</button>
            </div>
          </form>

          <hr>

          <form action="/qms/ncrs/{{ .NCR.ID }}/disposition" method="POST">
            <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
            <label class="form-label fw-bold">Record Disposition</label>
            <select name="disposition_type" class="form-select mb-2" required>
              <option value="">-- Decision --</option>
              {{ range .DispositionTypes }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
            <textarea name="description" class="form-control mb-2" placeholder="Justification..." required></textarea>
            <button class="btn btn-warning w-100" type="submit">Approve Disposition</button>
          </form>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
""",

    # CAPA Templates
    "capas.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-warning"><i class="bi bi-journal-medical"></i> Corrective & Preventive Actions (CAPA)</h1>
    <a href="/qms/capas/new" class="btn btn-warning shadow-sm"><i class="bi bi-plus-circle"></i> Initiate CAPA</a>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Number</th>
            <th>Title</th>
            <th>Source</th>
            <th>Priority</th>
            <th>Target Date</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .CAPAs }}
            {{ range .CAPAs }}
            <tr>
              <td><strong>{{ .Number }}</strong></td>
              <td>{{ .Title }}</td>
              <td>{{ .SourceType }}</td>
              <td><span class="badge bg-secondary">{{ .Priority }}</span></td>
              <td>{{ if .TargetDate }}{{ .TargetDate.Format "2006-01-02" }}{{ else }}-{{ end }}</td>
              <td><span class="badge bg-info text-dark">{{ .Status }}</span></td>
              <td><a href="/qms/capas/{{ .ID }}" class="btn btn-sm btn-outline-warning">Track</a></td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="7" class="text-center text-muted py-4">No active CAPAs found.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "capa_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm border-warning">
    <div class="card-header bg-warning text-dark py-3">
      <h2 class="h5 mb-0"><i class="bi bi-journal-medical"></i> Initiate CAPA</h2>
    </div>
    <div class="card-body p-4">
      <form action="/qms/capas" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="mb-3">
          <label class="form-label fw-bold">Title</label>
          <input type="text" name="title" class="form-control" required placeholder="CAPA objective">
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Problem Statement</label>
          <textarea name="description" class="form-control" rows="4"></textarea>
        </div>
        
        <div class="row mb-3">
          <div class="col-md-4">
            <label class="form-label fw-bold">Priority</label>
            <select name="priority" class="form-select">
              {{ range .Priorities }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-4">
            <label class="form-label fw-bold">Source Type</label>
            <select name="source_type" class="form-select">
              {{ range .SourceTypes }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-4">
            <label class="form-label fw-bold">Root Cause Method</label>
            <select name="root_cause_method" class="form-select">
              {{ range .RootCauseMethods }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/qms/capas" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-warning">Initiate</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "capa_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-warning mb-1">CAPA: {{ .CAPA.Number }}</h1>
      <p class="text-muted">{{ .CAPA.Title }}</p>
    </div>
    <a href="/qms/capas" class="btn btn-outline-secondary">Back to CAPAs</a>
  </div>

  <div class="row">
    <div class="col-lg-8">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Problem Statement</div>
        <div class="card-body">
          <p>{{ .CAPA.Description }}</p>
          <hr>
          <div class="row">
            <div class="col-sm-6">
              <p><strong>Root Cause Method:</strong> {{ .CAPA.RootCauseMethod }}</p>
              <p><strong>Priority:</strong> <span class="badge bg-secondary">{{ .CAPA.Priority }}</span></p>
            </div>
            <div class="col-sm-6">
              <p><strong>Source:</strong> {{ .CAPA.SourceType }}</p>
            </div>
          </div>
        </div>
      </div>
      
      <div class="card shadow-sm mb-4 border-success">
        <div class="card-header bg-success text-white fw-bold">Investigation & Action Plan</div>
        <div class="card-body">
           <p><strong>Root Cause:</strong><br> {{ if .CAPA.RootCause }}{{ .CAPA.RootCause }}{{ else }}<span class="text-muted">Pending Investigation</span>{{ end }}</p>
           <p><strong>Corrective Action:</strong><br> {{ if .CAPA.CorrectiveAction }}{{ .CAPA.CorrectiveAction }}{{ else }}<span class="text-muted">Pending</span>{{ end }}</p>
        </div>
      </div>
    </div>
    
    <div class="col-lg-4">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Lifecycle Status</div>
        <div class="card-body">
          <p class="mb-3">Current Status: <span class="badge bg-info text-dark fs-6">{{ .CAPA.Status }}</span></p>
          <form action="/qms/capas/{{ .CAPA.ID }}/status" method="POST">
            <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
            <div class="input-group">
              <select name="status" class="form-select">
                {{ range .Statuses }}<option value="{{ . }}">{{ . }}</option>{{ end }}
              </select>
              <button class="btn btn-warning" type="submit">Update</button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
""",

    # Audits Templates
    "audits.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-success"><i class="bi bi-shield-check"></i> Quality Audits</h1>
    <a href="/qms/audits/new" class="btn btn-success shadow-sm"><i class="bi bi-plus-circle"></i> Plan Audit</a>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Number</th>
            <th>Title</th>
            <th>Type</th>
            <th>Standard</th>
            <th>Planned Date</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .Audits }}
            {{ range .Audits }}
            <tr>
              <td><strong>{{ .Number }}</strong></td>
              <td>{{ .Title }}</td>
              <td>{{ .AuditType }}</td>
              <td>{{ .Standard }}</td>
              <td>{{ if .PlannedStart }}{{ .PlannedStart.Format "2006-01-02" }}{{ else }}-{{ end }}</td>
              <td><span class="badge bg-info text-dark">{{ .Status }}</span></td>
              <td><a href="/qms/audits/{{ .ID }}" class="btn btn-sm btn-outline-success">Manage</a></td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="7" class="text-center text-muted py-4">No audits planned.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "audit_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm border-success">
    <div class="card-header bg-success text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-shield-check"></i> Plan Quality Audit</h2>
    </div>
    <div class="card-body p-4">
      <form action="/qms/audits" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="mb-3">
          <label class="form-label fw-bold">Audit Title</label>
          <input type="text" name="title" class="form-control" required>
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Scope / Description</label>
          <textarea name="description" class="form-control" rows="3"></textarea>
        </div>
        
        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Type</label>
            <select name="audit_type" class="form-select">
              {{ range .AuditTypes }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Standard Reference</label>
            <select name="standard" class="form-select">
              {{ range .Standards }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
        </div>

        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Planned Start Date</label>
            <input type="date" name="planned_start" class="form-control">
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Planned End Date</label>
            <input type="date" name="planned_end" class="form-control">
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/qms/audits" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-success">Plan Audit</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "audit_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-success mb-1">Audit: {{ .Audit.Number }}</h1>
      <p class="text-muted">{{ .Audit.Title }}</p>
    </div>
    <a href="/qms/audits" class="btn btn-outline-secondary">Back to Audits</a>
  </div>

  <div class="row">
    <div class="col-lg-8">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Audit Scope</div>
        <div class="card-body">
          <p>{{ .Audit.Description }}</p>
          <hr>
          <div class="row">
            <div class="col-sm-4">
              <p><strong>Standard:</strong> {{ .Audit.Standard }}</p>
            </div>
            <div class="col-sm-4">
              <p><strong>Type:</strong> {{ .Audit.AuditType }}</p>
            </div>
            <div class="col-sm-4">
              <p><strong>Status:</strong> <span class="badge bg-secondary">{{ .Audit.Status }}</span></p>
            </div>
          </div>
        </div>
      </div>
      
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-warning fw-bold">Audit Findings</div>
        <div class="card-body">
          {{ if .Findings }}
            <ul class="list-group list-group-flush">
              {{ range .Findings }}
              <li class="list-group-item">
                <div class="d-flex justify-content-between">
                  <strong>{{ .FindingNumber }} - {{ .Category }}</strong>
                  <span class="badge bg-danger">{{ .RiskLevel }} Risk</span>
                </div>
                <p class="mb-1">{{ .Description }}</p>
                <small class="text-muted">Clause: {{ .Clause }}</small>
              </li>
              {{ end }}
            </ul>
          {{ else }}
            <p class="text-muted mb-0">No findings logged yet.</p>
          {{ end }}
        </div>
      </div>
    </div>

    <div class="col-lg-4">
      <div class="card shadow-sm border-danger">
        <div class="card-header bg-danger text-white fw-bold">Log Finding</div>
        <div class="card-body">
          <form action="/qms/audits/{{ .Audit.ID }}/findings" method="POST">
            <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
            
            <div class="mb-2">
              <label class="form-label fw-bold small">Category</label>
              <select name="category" class="form-select form-select-sm" required>
                {{ range .FindingCategories }}<option value="{{ . }}">{{ . }}</option>{{ end }}
              </select>
            </div>
            <div class="mb-2">
              <label class="form-label fw-bold small">Risk Level</label>
              <select name="risk_level" class="form-select form-select-sm" required>
                {{ range .RiskLevels }}<option value="{{ . }}">{{ . }}</option>{{ end }}
              </select>
            </div>
            <div class="mb-2">
              <label class="form-label fw-bold small">Standard Clause</label>
              <input type="text" name="clause" class="form-control form-control-sm" placeholder="e.g. ISO9001 8.2">
            </div>
            <div class="mb-3">
              <label class="form-label fw-bold small">Description</label>
              <textarea name="description" class="form-control form-control-sm" rows="3" required></textarea>
            </div>
            <button class="btn btn-sm btn-danger w-100" type="submit">Save Finding</button>
          </form>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
""",

    # Supplier Quality Templates
    "supplier_quality.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-secondary"><i class="bi bi-truck"></i> Supplier Quality Ratings</h1>
    <a href="/qms/supplier-quality/new" class="btn btn-secondary shadow-sm"><i class="bi bi-plus"></i> Add Rating</a>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Supplier</th>
            <th>Status</th>
            <th>Quality Score</th>
            <th>Risk Level</th>
            <th>Last Audit</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .Records }}
            {{ range .Records }}
            <tr>
              <td><strong>{{ .SupplierName }}</strong></td>
              <td><span class="badge bg-info text-dark">{{ .Status }}</span></td>
              <td>
                <div class="progress" style="height: 20px;">
                  <div class="progress-bar {{ if ge .QualityRating 90.0 }}bg-success{{ else if ge .QualityRating 70.0 }}bg-warning{{ else }}bg-danger{{ end }}" role="progressbar" style="width: {{ .QualityRating }}%">{{ .QualityRating }}%</div>
                </div>
              </td>
              <td><span class="badge bg-secondary">{{ .RiskLevel }}</span></td>
              <td>{{ if .LastAuditDate }}{{ .LastAuditDate.Format "2006-01-02" }}{{ else }}-{{ end }}</td>
              <td><a href="/qms/supplier-quality/{{ .ID }}" class="btn btn-sm btn-outline-secondary">View</a></td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="6" class="text-center text-muted py-4">No supplier records found.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "supplier_quality_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm">
    <div class="card-header bg-secondary text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-truck"></i> Create Supplier Quality Record</h2>
    </div>
    <div class="card-body p-4">
      <form action="/qms/supplier-quality" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Supplier ID</label>
            <input type="number" name="supplier_id" class="form-control" required placeholder="Database ID of Supplier">
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Supplier Name (Snapshot)</label>
            <input type="text" name="supplier_name" class="form-control" required>
          </div>
        </div>
        
        <div class="row mb-3">
          <div class="col-md-4">
            <label class="form-label fw-bold">Status</label>
            <select name="status" class="form-select">
              {{ range .Statuses }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-4">
            <label class="form-label fw-bold">Risk Level</label>
            <select name="risk_level" class="form-select">
              {{ range .RiskLevels }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-4">
            <label class="form-label fw-bold">Quality Rating (0-100)</label>
            <input type="number" step="0.1" name="quality_rating" class="form-control" value="100.0" min="0" max="100">
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/qms/supplier-quality" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-secondary">Save Record</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "supplier_quality_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-secondary mb-1">Supplier: {{ .Record.SupplierName }}</h1>
      <p class="text-muted">Quality Rating Record</p>
    </div>
    <a href="/qms/supplier-quality" class="btn btn-outline-secondary">Back</a>
  </div>

  <div class="card shadow-sm mb-4">
    <div class="card-header bg-light fw-bold">Rating Breakdown</div>
    <div class="card-body">
      <div class="row">
        <div class="col-sm-4 text-center">
          <h1 class="display-4 {{ if ge .Record.QualityRating 90.0 }}text-success{{ else if ge .Record.QualityRating 70.0 }}text-warning{{ else }}text-danger{{ end }}">{{ .Record.QualityRating }}%</h1>
          <p class="text-muted">Overall Score</p>
        </div>
        <div class="col-sm-8 border-start px-4">
          <p><strong>Status:</strong> <span class="badge bg-info text-dark">{{ .Record.Status }}</span></p>
          <p><strong>Risk Level:</strong> {{ .Record.RiskLevel }}</p>
          <p><strong>Last Audit:</strong> {{ if .Record.LastAuditDate }}{{ .Record.LastAuditDate.Format "2006-01-02" }}{{ else }}None{{ end }}</p>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
"""
}

for file_name, content in files.items():
    with open(os.path.join(base_dir, file_name), "w") as f:
        f.write(content)

print("Generated full UI templates for QMS")
