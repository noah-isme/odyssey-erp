import os

base_dir = "/home/noah/project/odyssey-erp/web/templates/pages/documents"
os.makedirs(base_dir, exist_ok=True)

files = {
    # Document Library
    "library.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-primary"><i class="bi bi-file-earmark-text"></i> Document Library</h1>
    <div>
      <a href="/documents/categories" class="btn btn-outline-secondary me-2">Categories</a>
      <a href="/documents/classifications" class="btn btn-outline-secondary me-2">Classifications</a>
      <a href="/documents/library/new" class="btn btn-primary shadow-sm"><i class="bi bi-plus-lg"></i> New Document</a>
    </div>
  </div>

  <div class="card shadow-sm mb-4">
    <div class="card-body">
      <form method="get" class="row g-3">
        <div class="col-md-3">
          <select name="status" class="form-select">
            <option value="">All Statuses</option>
            {{ range .Statuses }}<option value="{{ . }}">{{ . }}</option>{{ end }}
          </select>
        </div>
        <div class="col-md-3">
          <select name="category_id" class="form-select">
            <option value="">All Categories</option>
            {{ range .Categories }}<option value="{{ .ID }}">{{ .Name }}</option>{{ end }}
          </select>
        </div>
        <div class="col-md-3">
          <select name="classification_id" class="form-select">
            <option value="">All Classifications</option>
            {{ range .Classifications }}<option value="{{ .ID }}">{{ .Name }}</option>{{ end }}
          </select>
        </div>
        <div class="col-md-2">
          <button type="submit" class="btn btn-outline-secondary w-100">Search</button>
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
            <th>Category</th>
            <th>Classification</th>
            <th>Status</th>
            <th>Last Updated</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {{ if .Documents }}
            {{ range .Documents }}
            <tr>
              <td><strong>{{ .Number }}</strong></td>
              <td>{{ .Title }}</td>
              <td>{{ .CategoryName }}</td>
              <td><span class="badge bg-secondary">{{ .ClassificationName }}</span></td>
              <td><span class="badge bg-info text-dark">{{ .Status }}</span></td>
              <td>{{ .UpdatedAt.Format "2006-01-02 15:04" }}</td>
              <td>
                <a href="/documents/library/{{ .ID }}" class="btn btn-sm btn-outline-primary">Open</a>
              </td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="7" class="text-center text-muted py-4">No documents found in the library.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "document_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm">
    <div class="card-header bg-primary text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-file-earmark-text"></i> Register New Document</h2>
    </div>
    <div class="card-body p-4">
      <form action="/documents/library" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="mb-3">
          <label class="form-label fw-bold">Title</label>
          <input type="text" name="title" class="form-control" required placeholder="Document Title">
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Description / Abstract</label>
          <textarea name="description" class="form-control" rows="3"></textarea>
        </div>
        
        <div class="row mb-3">
          <div class="col-md-6">
            <label class="form-label fw-bold">Category</label>
            <select name="category_id" class="form-select" required>
              <option value="">-- Select Category --</option>
              {{ range .Categories }}<option value="{{ .ID }}">{{ .Name }}</option>{{ end }}
            </select>
          </div>
          <div class="col-md-6">
            <label class="form-label fw-bold">Classification Level</label>
            <select name="classification_id" class="form-select" required>
              <option value="">-- Select Classification --</option>
              {{ range .Classifications }}<option value="{{ .ID }}">{{ .Name }}</option>{{ end }}
            </select>
          </div>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/documents/library" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-primary">Register Document</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",
    "document_detail.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-primary mb-1">{{ .Document.Number }}</h1>
      <p class="text-muted h5">{{ .Document.Title }}</p>
    </div>
    <div>
      <a href="/documents/library" class="btn btn-outline-secondary me-2">Back to Library</a>
      <a href="/documents/library/{{ .Document.ID }}/versions" class="btn btn-primary shadow-sm"><i class="bi bi-clock-history"></i> Manage Versions</a>
    </div>
  </div>

  <div class="row">
    <div class="col-lg-8">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Document Metadata</div>
        <div class="card-body">
          <p>{{ .Document.Description }}</p>
          <hr>
          <div class="row">
            <div class="col-sm-6">
              <p><strong>Category:</strong> {{ .Document.CategoryName }}</p>
              <p><strong>Classification:</strong> <span class="badge bg-secondary">{{ .Document.ClassificationName }}</span></p>
            </div>
            <div class="col-sm-6">
              <p><strong>Owner:</strong> {{ .Document.OwnerName }}</p>
              <p><strong>Created:</strong> {{ .Document.CreatedAt.Format "Jan 02, 2006" }}</p>
            </div>
          </div>
        </div>
      </div>
      
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Version History (Preview)</div>
        <div class="table-responsive">
          <table class="table table-hover mb-0">
            <thead>
              <tr>
                <th>V#</th>
                <th>Status</th>
                <th>Change Summary</th>
                <th>Created By</th>
                <th>Date</th>
              </tr>
            </thead>
            <tbody>
              {{ if .Versions }}
                {{ range .Versions }}
                <tr>
                  <td>v{{ .VersionNumber }}</td>
                  <td><span class="badge bg-secondary">{{ .Status }}</span></td>
                  <td class="text-truncate" style="max-width: 200px;">{{ .ChangeSummary }}</td>
                  <td>{{ .CreatedByName }}</td>
                  <td>{{ .CreatedAt.Format "Jan 02, 2006" }}</td>
                </tr>
                {{ end }}
              {{ else }}
                <tr><td colspan="5" class="text-center text-muted">No versions uploaded yet.</td></tr>
              {{ end }}
            </tbody>
          </table>
        </div>
      </div>
    </div>
    
    <div class="col-lg-4">
      <div class="card shadow-sm mb-4 border-info">
        <div class="card-header bg-info text-white fw-bold">Lifecycle Status</div>
        <div class="card-body">
          <p class="mb-3">Current Status: <span class="badge bg-dark fs-6">{{ .Document.Status }}</span></p>
          
          <form action="/documents/library/{{ .Document.ID }}/status" method="POST">
            <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
            <div class="input-group">
              <select name="status" class="form-select">
                {{ range .Statuses }}<option value="{{ . }}">{{ . }}</option>{{ end }}
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
    "versions.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <div>
      <h1 class="h3 text-primary mb-1">Version Control: {{ .Document.Number }}</h1>
      <p class="text-muted">{{ .Document.Title }}</p>
    </div>
    <a href="/documents/library/{{ .Document.ID }}" class="btn btn-outline-secondary">Back to Document</a>
  </div>

  <div class="row">
    <div class="col-lg-8">
      <div class="card shadow-sm mb-4">
        <div class="card-header bg-light fw-bold">Version History</div>
        <div class="table-responsive">
          <table class="table table-hover table-striped mb-0 align-middle">
            <thead class="table-light">
              <tr>
                <th>V#</th>
                <th>Status</th>
                <th>Change Summary</th>
                <th>Date</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {{ if .Versions }}
                {{ range .Versions }}
                <tr>
                  <td><strong>v{{ .VersionNumber }}</strong></td>
                  <td><span class="badge bg-secondary">{{ .Status }}</span></td>
                  <td>{{ .ChangeSummary }}</td>
                  <td>{{ .CreatedAt.Format "Jan 02, 2006 15:04" }}</td>
                  <td>
                    <button class="btn btn-sm btn-outline-primary" disabled><i class="bi bi-download"></i> Download</button>
                  </td>
                </tr>
                {{ end }}
              {{ else }}
                <tr><td colspan="5" class="text-center text-muted py-4">No versions uploaded yet.</td></tr>
              {{ end }}
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div class="col-lg-4">
      <div class="card shadow-sm border-primary">
        <div class="card-header bg-primary text-white fw-bold">Upload New Version</div>
        <div class="card-body">
          <form action="/documents/library/{{ .Document.ID }}/versions" method="POST" enctype="multipart/form-data">
            <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
            
            <div class="mb-3">
              <label class="form-label fw-bold small">File</label>
              <input type="file" name="file" class="form-control form-control-sm" required>
            </div>
            
            <div class="mb-3">
              <label class="form-label fw-bold small">Change Summary</label>
              <textarea name="description" class="form-control form-control-sm" rows="3" required placeholder="What changed in this version?"></textarea>
            </div>
            
            <button class="btn btn-primary w-100" type="submit"><i class="bi bi-upload"></i> Upload Version</button>
          </form>
        </div>
      </div>
    </div>
  </div>
</div>
{{ end }}
""",

    # Categories
    "categories.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-primary"><i class="bi bi-folder2-open"></i> Document Categories</h1>
    <a href="/documents/categories/new" class="btn btn-primary shadow-sm"><i class="bi bi-folder-plus"></i> New Category</a>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Code</th>
            <th>Name</th>
            <th>Description</th>
            <th>Active</th>
          </tr>
        </thead>
        <tbody>
          {{ if .Categories }}
            {{ range .Categories }}
            <tr>
              <td><strong>{{ .Code }}</strong></td>
              <td>{{ .Name }}</td>
              <td>{{ .Description }}</td>
              <td>{{ if .Active }}<span class="badge bg-success">Yes</span>{{ else }}<span class="badge bg-secondary">No</span>{{ end }}</td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="4" class="text-center text-muted py-4">No categories defined.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
""",
    "category_new.html": """{{ define "content" }}
<div class="container mt-4 max-w-4xl mx-auto">
  <div class="card shadow-sm">
    <div class="card-header bg-primary text-white py-3">
      <h2 class="h5 mb-0"><i class="bi bi-folder-plus"></i> Create Category</h2>
    </div>
    <div class="card-body p-4">
      <form action="/documents/categories" method="POST">
        <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
        
        <div class="row mb-3">
          <div class="col-md-4">
            <label class="form-label fw-bold">Code</label>
            <input type="text" name="code" class="form-control" required placeholder="CAT-01">
          </div>
          <div class="col-md-8">
            <label class="form-label fw-bold">Name</label>
            <input type="text" name="name" class="form-control" required>
          </div>
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Description</label>
          <textarea name="description" class="form-control" rows="2"></textarea>
        </div>
        
        <div class="mb-3">
          <label class="form-label fw-bold">Parent Category (Optional)</label>
          <select name="parent_id" class="form-select">
            <option value="">-- None (Top Level) --</option>
            {{ range .Categories }}<option value="{{ .ID }}">{{ .Name }}</option>{{ end }}
          </select>
        </div>

        <div class="d-flex justify-content-end gap-2 mt-4">
          <a href="/documents/categories" class="btn btn-light border">Cancel</a>
          <button type="submit" class="btn btn-primary">Save Category</button>
        </div>
      </form>
    </div>
  </div>
</div>
{{ end }}
""",

    # Classifications
    "classifications.html": """{{ define "content" }}
<div class="container-fluid mt-4">
  <div class="d-flex justify-content-between align-items-center mb-4">
    <h1 class="h3 text-primary"><i class="bi bi-shield-lock"></i> Classifications</h1>
  </div>

  <div class="card shadow-sm">
    <div class="table-responsive">
      <table class="table table-hover table-striped mb-0 align-middle">
        <thead class="table-light">
          <tr>
            <th>Code</th>
            <th>Name</th>
            <th>Description</th>
            <th>Requires Approval</th>
            <th>Requires Signature</th>
          </tr>
        </thead>
        <tbody>
          {{ if .Classifications }}
            {{ range .Classifications }}
            <tr>
              <td><strong>{{ .Code }}</strong></td>
              <td>{{ .Name }}</td>
              <td>{{ .Description }}</td>
              <td>{{ if .RequiresApproval }}<span class="badge bg-danger">Yes</span>{{ else }}No{{ end }}</td>
              <td>{{ if .RequiresSignature }}<span class="badge bg-danger">Yes</span>{{ else }}No{{ end }}</td>
            </tr>
            {{ end }}
          {{ else }}
            <tr><td colspan="5" class="text-center text-muted py-4">No classifications defined.</td></tr>
          {{ end }}
        </tbody>
      </table>
    </div>
  </div>
</div>
{{ end }}
"""
}

for file_name, content in files.items():
    with open(os.path.join(base_dir, file_name), "w") as f:
        f.write(content)

print("Generated full UI templates for Documents module")
