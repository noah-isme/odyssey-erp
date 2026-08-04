import os

base_dir = "/home/noah/project/odyssey-erp/web/templates/pages/cmms"
os.makedirs(base_dir, exist_ok=True)

files = {
    "work_orders.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-primary"><i class="bi bi-tools"></i> Work Orders</h1>
    <a href="/cmms/work-orders/new" class="btn btn-primary shadow-sm"><i class="bi bi-plus-circle"></i> New Work Order</a>
  </div>

  <div class="card shadow-sm mb-4">
    <div class="card-body">
      <form method="get" class="row g-3">
        <div class="col-md-3">
          <select name="status" class="form-select">
            <option value="">All Statuses</option>
            {{ range .Statuses }}
            <option value="{{ . }}" {{ if eq $.Filter.Status . }}selected{{ end }}>{{ . }}</option>
            {{ end }}
          </select>
        </div>
        <div class="col-md-3">
          <select name="category" class="form-select">
            <option value="">All Categories</option>
            <option value="PREVENTIVE" {{ if eq .Filter.Category "PREVENTIVE" }}selected{{ end }}>Preventive</option>
            <option value="CORRECTIVE" {{ if eq .Filter.Category "CORRECTIVE" }}selected{{ end }}>Corrective</option>
            <option value="INSPECTION" {{ if eq .Filter.Category "INSPECTION" }}selected{{ end }}>Inspection</option>
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
            <th>Asset</th>
            <th>Location</th>
            <th>Priority</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .WorkOrders }}
            {{ range .WorkOrders }}
            <tr>
              <td><strong>{{ .Number }}</strong></td>
              <td>{{ .Title }}</td>
              <td>{{ .AssetName }}</td>
              <td>{{ .LocationName }}</td>
              <td><span class="badge bg-secondary">{{ .Priority }}</span></td>
              <td><span class="badge bg-info text-dark">{{ .Status }}</span></td>
              <td>
                <a href="/cmms/work-orders/{{ .ID }}" class="btn btn-sm btn-outline-primary">View</a>
              </td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="7" class="text-center text-muted py-4">No work orders found.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "work_order_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm">
    <div class="card-header bg-primary text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-tools"></i> Create Work Order</h2>
    </div>
    <div class="card-body p-4">
      <form action="/cmms/work-orders" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="mb-3">
          <label class="form-label fw-bold">Title</label>
          <input type="text" name="title" class="form-control" required placeholder="Brief description of the issue">
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Description</label>
          <textarea name="description" class="form-control" rows="4" placeholder="Detailed instructions..."></textarea>
        </div>
        
        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Asset</label>
            <select name="asset_id" class="form-select">
              <option value="">-- None --</option>
              {{ range .Assets }}
              <option value="{{ .ID }}">{{ .Code }} - {{ .Name }}</option>
              {{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Location</label>
            <select name="location_id" class="form-select">
              <option value="">-- None --</option>
              {{ range .Locations }}
              <option value="{{ .ID }}">{{ .Code }} - {{ .Name }}</option>
              {{ end }}
            </select>
          </div>
        </div>

        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Priority</label>
            <select name="priority" class="form-select">
              {{ range .Priorities }}
              <option value="{{ . }}">{{ . }}</option>
              {{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Category</label>
            <select name="category" class="form-select">
              {{ range .Categories }}
              <option value="{{ . }}">{{ . }}</option>
              {{ end }}
            </select>
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/cmms/work-orders" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-primary">Create Work Order</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "work_order_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-primary mb-1">Work Order: {{ .WorkOrder.Number }}</h1>
      <p class="text-muted">{{ .WorkOrder.Title }}</p>
    </div>
    <a href="/cmms/work-orders" class="btn btn-outline-secondary">Back to List</a>
  </div>

  <div class="row">
    <div class="col-lg-8">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Details</div>
        <div class="card-body">
          <p><strong>Description:</strong><br> {{ .WorkOrder.Description }}</p>
          <div class="row mt-4">
            <div class="col-sm-6">
              <p><strong>Asset:</strong> {{ .WorkOrder.AssetName }}</p>
              <p><strong>Location:</strong> {{ .WorkOrder.LocationName }}</p>
            </div>
            <div class="col-sm-6">
              <p><strong>Priority:</strong> <span class="badge bg-secondary">{{ .WorkOrder.Priority }}</span></p>
              <p><strong>Category:</strong> {{ .WorkOrder.Category }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
    
    <div class="col-lg-4">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Status Update</div>
        <div class="card-body">
          <p class="mb-3">Current Status: <span class="badge bg-info text-dark fs-6">{{ .WorkOrder.Status }}</span></p>
          <form action="/cmms/work-orders/{{ .WorkOrder.ID }}/status" method="POST">
            <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
            <div class="input-group">
              <select name="status" class="form-select">
                {{ range .Statuses }}
                <option value="{{ . }}">{{ . }}</option>
                {{ end }}
              </select>
              <button class="btn btn-primary" type="submit">Update</button>
            </div>
          </form>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
""",
    "assets.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-success"><i class="bi bi-box"></i> Assets</h1>
    <a href="/cmms/assets/new" class="btn btn-success shadow-sm"><i class="bi bi-plus"></i> Add Asset</a>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Code</th>
            <th>Name</th>
            <th>Type</th>
            <th>Manufacturer</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .Assets }}
            {{ range .Assets }}
            <tr>
              <td><strong>{{ .Code }}</strong></td>
              <td>{{ .Name }}</td>
              <td>{{ .AssetType }}</td>
              <td>{{ .Manufacturer }}</td>
              <td><span class="badge bg-secondary">{{ .Status }}</span></td>
              <td>
                <a href="/cmms/assets/{{ .ID }}" class="btn btn-sm btn-outline-success">View</a>
              </td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="6" class="text-center text-muted py-4">No assets found.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "asset_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm">
    <div class="card-header bg-success text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-box"></i> Register New Asset</h2>
    </div>
    <div class="card-body p-4">
      <form action="/cmms/assets" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="row mb-3">
          <div class="col-md-4">
            <label class="form-label fw-bold">Code</label>
            <input type="text" name="code" class="form-control" required placeholder="EQ-001">
          </div>
          <div class="col-md-8">
            <label class="form-label fw-bold">Name</label>
            <input type="text" name="name" class="form-control" required placeholder="Asset Name">
          </div>
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Description</label>
          <textarea name="description" class="form-control" rows="3"></textarea>
        </div>
        
        <div class="row mb-3">
          <div class="col-md-4">
            <label class="form-label fw-bold">Type</label>
            <select name="asset_type" class="form-select">
              {{ range .AssetTypes }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-4">
            <label class="form-label fw-bold">Manufacturer</label>
            <input type="text" name="manufacturer" class="form-control">
          </div>
          <div class="col-md-4">
            <label class="form-label fw-bold">Model</label>
            <input type="text" name="model" class="form-control">
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/cmms/assets" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-success">Register Asset</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "asset_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-success mb-1">Asset: {{ .Asset.Code }}</h1>
      <p class="text-muted">{{ .Asset.Name }}</p>
    </div>
    <a href="/cmms/assets" class="btn btn-outline-secondary">Back to Assets</a>
  </div>

  <div class="card shadow-sm mb-4">
    <div class="card-header bg-light fw-bold">Asset Details</div>
    <div class="card-body">
      <div class="row">
        <div class="col-sm-6">
          <p><strong>Description:</strong><br> {{ .Asset.Description }}</p>
          <p><strong>Type:</strong> {{ .Asset.AssetType }}</p>
        </div>
        <div class="col-sm-6">
          <p><strong>Manufacturer:</strong> {{ .Asset.Manufacturer }}</p>
          <p><strong>Model:</strong> {{ .Asset.Model }}</p>
          <p><strong>Serial Number:</strong> {{ .Asset.SerialNumber }}</p>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
""",
    "pm_schedules.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-info"><i class="bi bi-calendar-check"></i> PM Schedules</h1>
    <a href="/cmms/pm-schedules/new" class="btn btn-info text-white shadow-sm"><i class="bi bi-plus"></i> New Schedule</a>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Name</th>
            <th>Frequency</th>
            <th>Next Due Date</th>
            <th>Status</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .PMSchedules }}
            {{ range .PMSchedules }}
            <tr>
              <td><strong>{{ .Name }}</strong></td>
              <td>{{ .FrequencyValue }} {{ .FrequencyType }}</td>
              <td>{{ if .NextDueDate }}{{ .NextDueDate.Format "2006-01-02" }}{{ else }}-{{ end }}</td>
              <td>{{ if .Active }}<span class="badge bg-success">Active</span>{{ else }}<span class="badge bg-secondary">Inactive</span>{{ end }}</td>
              <td>
                <a href="/cmms/pm-schedules/{{ .ID }}" class="btn btn-sm btn-outline-info">View</a>
              </td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="5" class="text-center text-muted py-4">No PM Schedules found.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "pm_schedule_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm">
    <div class="card-header bg-info text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-calendar-check"></i> Create PM Schedule</h2>
    </div>
    <div class="card-body p-4">
      <form action="/cmms/pm-schedules" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="mb-3">
          <label class="form-label fw-bold">Asset</label>
          <select name="asset_id" class="form-select" required>
            <option value="">-- Select Asset --</option>
            {{ range .Assets }}
            <option value="{{ .ID }}">{{ .Code }} - {{ .Name }}</option>
            {{ end }}
          </select>
        </div>

        <div class="mb-3">
          <label class="form-label fw-bold">Name</label>
          <input type="text" name="name" class="form-control" required placeholder="e.g. Monthly Oil Change">
        </div>
        
        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Frequency Type</label>
            <select name="frequency_type" class="form-select">
              {{ range .FrequencyTypes }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Frequency Value</label>
            <input type="number" name="frequency_value" class="form-control" value="1" min="1" required>
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/cmms/pm-schedules" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-info text-white">Save Schedule</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "pm_schedule_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-info mb-1">PM Schedule: {{ .Schedule.Name }}</h1>
      <p class="text-muted">{{ .Schedule.Description }}</p>
    </div>
    <a href="/cmms/pm-schedules" class="btn btn-outline-secondary">Back to Schedules</a>
  </div>

  <div class="card shadow-sm mb-4">
    <div class="card-header bg-light fw-bold">Schedule Details</div>
    <div class="card-body">
      <div class="row">
        <div class="col-sm-6">
          <p><strong>Frequency:</strong> {{ .Schedule.FrequencyValue }} {{ .Schedule.FrequencyType }}</p>
          <p><strong>Next Due Date:</strong> {{ if .Schedule.NextDueDate }}{{ .Schedule.NextDueDate.Format "2006-01-02" }}{{ else }}-{{ end }}</p>
        </div>
        <div class="col-sm-6">
          <p><strong>Status:</strong> {{ if .Schedule.Active }}Active{{ else }}Inactive{{ end }}</p>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
""",
    "spare_parts.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-warning"><i class="bi bi-gear"></i> Spare Parts</h1>
    <a href="/cmms/spare-parts/new" class="btn btn-warning shadow-sm"><i class="bi bi-plus"></i> Add Part</a>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Code</th>
            <th>Name</th>
            <th>Category</th>
            <th>Unit Cost</th>
            <th>Critical</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .SpareParts }}
            {{ range .SpareParts }}
            <tr>
              <td><strong>{{ .Code }}</strong></td>
              <td>{{ .Name }}</td>
              <td>{{ .Category }}</td>
              <td>${{ .UnitCost }}</td>
              <td>{{ if .CriticalSpare }}<span class="badge bg-danger">Yes</span>{{ else }}No{{ end }}</td>
              <td>
                <button class="btn btn-sm btn-outline-warning">View</button>
              </td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="6" class="text-center text-muted py-4">No spare parts found.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "spare_part_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm">
    <div class="card-header bg-warning py-3">
      <h2 class="h5 mb-0"><i class="bi bi-gear"></i> Register Spare Part</h2>
    </div>
    <div class="card-body p-4">
      <form action="/cmms/spare-parts" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="row mb-3">
          <div class="col-md-4">
            <label class="form-label fw-bold">Code</label>
            <input type="text" name="code" class="form-control" required placeholder="PART-001">
          </div>
          <div class="col-md-8">
            <label class="form-label fw-bold">Name</label>
            <input type="text" name="name" class="form-control" required>
          </div>
        </div>

        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Category</label>
            <select name="category" class="form-select">
              {{ range .Categories }}<option value="{{ . }}">{{ . }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Unit Cost</label>
            <input type="number" step="0.01" name="unit_cost" class="form-control" value="0.00">
          </div>
        </div>

        <div class="mb-3 form-check">
          <input type="checkbox" class="form-check-input" id="critical" name="critical_spare" value="true">
          <label class="form-check-label fw-bold" for="critical">Critical Spare Part (Required for operations)</label>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/cmms/spare-parts" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-warning">Save Part</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
"""
}

for file_name, content in files.items():
    with open(os.path.join(base_dir, file_name), "w") as f:
        f.write(content)

print("Generated full UI templates for CMMS")
