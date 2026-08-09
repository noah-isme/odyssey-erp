package ap

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/odyssey-erp/odyssey-erp/internal/shared"
)

func (h *Handler) listDebitNotes(w http.ResponseWriter, r *http.Request) {
	notes, err := h.service.ListAPDebitNotes(r.Context(), ListAPDebitNotesRequest{Limit: 100})
	if err != nil {
		h.render(w, r, "pages/ap/ap_debit_note_list.html", map[string]any{"Errors": formErrors{"general": shared.UserSafeMessage(err)}}, http.StatusInternalServerError)
		return
	}
	h.render(w, r, "pages/ap/ap_debit_note_list.html", map[string]any{"DebitNotes": notes}, http.StatusOK)
}

func (h *Handler) showDebitNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid debit note ID", http.StatusBadRequest)
		return
	}
	note, err := h.service.GetAPDebitNoteWithDetails(r.Context(), id)
	if err != nil {
		shared.WriteErrorStatus(w, http.StatusNotFound, err)
		return
	}
	h.render(w, r, "pages/ap/ap_debit_note_detail.html", map[string]any{"DebitNote": note}, http.StatusOK)
}

func (h *Handler) createDebitNoteFromReturn(w http.ResponseWriter, r *http.Request) {
	returnID, err := strconv.ParseInt(chi.URLParam(r, "returnID"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid goods return ID", http.StatusBadRequest)
		return
	}
	note, err := h.service.CreateAPDebitNoteFromReturn(r.Context(), CreateAPDebitNoteFromReturnInput{GoodsReturnGRNID: returnID, Reason: r.FormValue("reason"), CreatedBy: getUserID(shared.SessionFromContext(r.Context()))})
	if err != nil {
		h.redirectWithFlash(w, r, "/procurement/grns/returns/"+strconv.FormatInt(returnID, 10), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, debitNoteURL(note.ID), "success", "Debit note created")
}

func (h *Handler) postDebitNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid debit note ID", http.StatusBadRequest)
		return
	}
	if err := h.service.PostAPDebitNote(r.Context(), PostAPDebitNoteInput{DebitNoteID: id, PostedBy: getUserID(shared.SessionFromContext(r.Context()))}); err != nil {
		h.redirectWithFlash(w, r, debitNoteURL(id), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, debitNoteURL(id), "success", "Debit note posted")
}

func (h *Handler) voidDebitNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid debit note ID", http.StatusBadRequest)
		return
	}
	if err := h.service.VoidAPDebitNote(r.Context(), VoidAPDebitNoteInput{DebitNoteID: id, VoidedBy: getUserID(shared.SessionFromContext(r.Context())), VoidReason: r.FormValue("reason")}); err != nil {
		h.redirectWithFlash(w, r, debitNoteURL(id), "error", shared.UserSafeMessage(err))
		return
	}
	h.redirectWithFlash(w, r, debitNoteURL(id), "success", "Debit note voided")
}

func debitNoteURL(id int64) string { return "/finance/ap/debit-notes/" + strconv.FormatInt(id, 10) }
