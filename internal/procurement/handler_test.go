package procurement

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProcurementWorkbenchListsRequireActiveCompany(t *testing.T) {
	h := &Handler{}
	for _, handle := range []func(*Handler, http.ResponseWriter, *http.Request){
		(*Handler).handleListPOs,
		(*Handler).handleListGRNs,
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		handle(h, recorder, req)
		if recorder.Code != 403 {
			t.Fatalf("workbench status = %d, want 403 when active company is absent", recorder.Code)
		}
	}
}
