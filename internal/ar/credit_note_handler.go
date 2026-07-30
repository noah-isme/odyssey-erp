package ar

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func (h *Handler) listCreditNotes(w http.ResponseWriter, r *http.Request) {
	invoiceID, _ := strconv.ParseInt(r.URL.Query().Get("invoice_id"), 10, 64)
	notes, err := h.service.ListARCreditNotes(r.Context(), ListARCreditNotesRequest{InvoiceID: invoiceID, Limit: 100})
	if err != nil {
		h.logger.Error("list AR credit notes", "error", err)
		h.render(w, r, "pages/ar/ar_credit_note_list.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/ar/ar_credit_note_list.html", map[string]any{"CreditNotes": notes}, http.StatusOK)
}

func (h *Handler) showCreditNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid credit note ID", http.StatusBadRequest)
		return
	}
	note, err := h.service.GetARCreditNoteWithDetails(r.Context(), id)
	if err != nil {
		http.Error(w, shared.UserSafeMessage(err), http.StatusNotFound)
		return
	}
	h.render(w, r, "pages/ar/ar_credit_note_detail.html", map[string]any{"CreditNote": note}, http.StatusOK)
}

func (h *Handler) createCreditNoteFromReturn(w http.ResponseWriter, r *http.Request) {
	returnID, err := parseID(chi.URLParam(r, "returnID"))
	if err != nil {
		http.Error(w, "Invalid return ID", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	note, err := h.service.CreateARCreditNoteFromReturn(r.Context(), CreateARCreditNoteFromReturnInput{
		ReturnDeliveryOrderID: returnID,
		Reason:                r.FormValue("reason"),
		CreatedBy:             getUserID(shared.SessionFromContext(r.Context())),
	})
	if err != nil {
		h.redirectWithFlash(w, r, "/delivery/orders/returns/"+strconv.FormatInt(returnID, 10), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, "/finance/ar/credit-notes/"+strconv.FormatInt(note.ID, 10), "success", "Credit note created")
}

func (h *Handler) postCreditNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid credit note ID", http.StatusBadRequest)
		return
	}
	if err := h.service.PostARCreditNote(r.Context(), PostARCreditNoteInput{CreditNoteID: id, PostedBy: getUserID(shared.SessionFromContext(r.Context()))}); err != nil {
		h.redirectWithFlash(w, r, creditNoteURL(id), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, creditNoteURL(id), "success", "Credit note posted")
}

func (h *Handler) voidCreditNote(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid credit note ID", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	if err := h.service.VoidARCreditNote(r.Context(), VoidARCreditNoteInput{CreditNoteID: id, VoidedBy: getUserID(shared.SessionFromContext(r.Context())), VoidReason: r.FormValue("reason")}); err != nil {
		h.redirectWithFlash(w, r, creditNoteURL(id), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, creditNoteURL(id), "success", "Credit note voided")
}

func (h *Handler) creditNotePDFDownload(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid credit note ID", http.StatusBadRequest)
		return
	}
	if h.creditNotePDF == nil {
		http.Error(w, "PDF renderer unavailable", http.StatusServiceUnavailable)
		return
	}
	note, err := h.service.GetARCreditNoteWithDetails(r.Context(), id)
	if err != nil {
		http.Error(w, shared.UserSafeMessage(err), http.StatusNotFound)
		return
	}
	pdf, err := h.creditNotePDF.Render(r.Context(), *note)
	if err != nil {
		http.Error(w, "Failed to render PDF", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, note.Number))
	_, _ = w.Write(pdf)
}

func parseID(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

func creditNoteURL(id int64) string {
	return "/finance/ar/credit-notes/" + strconv.FormatInt(id, 10)
}
