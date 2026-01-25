package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/pesio-ai/be-lib-common/logger"
	"github.com/pesio-ai/be-ap-invoices/internal/service"
)

// HTTPHandler handles HTTP requests
type HTTPHandler struct {
	service *service.InvoiceService
	log     *logger.Logger
}

// NewHTTPHandler creates a new HTTP handler
func NewHTTPHandler(service *service.InvoiceService, log *logger.Logger) *HTTPHandler {
	return &HTTPHandler{
		service: service,
		log:     log,
	}
}

// CreateInvoice handles create invoice HTTP requests
func (h *HTTPHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req service.CreateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Get user ID from JWT token
	// req.CreatedBy = "system" // Leave empty for NULL

	invoice, err := h.service.CreateInvoice(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(invoice)
}

// GetInvoice handles get invoice HTTP requests
func (h *HTTPHandler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	invoiceID := r.URL.Query().Get("id")
	entityID := r.URL.Query().Get("entity_id")

	if invoiceID == "" || entityID == "" {
		http.Error(w, "Invoice ID and Entity ID are required", http.StatusBadRequest)
		return
	}

	invoice, err := h.service.GetInvoice(r.Context(), invoiceID, entityID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invoice)
}

// ListInvoices handles list invoices HTTP requests
func (h *HTTPHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entityID := r.URL.Query().Get("entity_id")
	if entityID == "" {
		http.Error(w, "Entity ID is required", http.StatusBadRequest)
		return
	}

	vendorID := r.URL.Query().Get("vendor_id")
	status := r.URL.Query().Get("status")
	fromDate := r.URL.Query().Get("from_date")
	toDate := r.URL.Query().Get("to_date")

	var vendorIDPtr *string
	if vendorID != "" {
		vendorIDPtr = &vendorID
	}

	var statusPtr *string
	if status != "" {
		statusPtr = &status
	}

	var fromDatePtr *string
	if fromDate != "" {
		fromDatePtr = &fromDate
	}

	var toDatePtr *string
	if toDate != "" {
		toDatePtr = &toDate
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	invoices, total, err := h.service.ListInvoices(r.Context(), entityID, vendorIDPtr, statusPtr, fromDatePtr, toDatePtr, page, pageSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"invoices": invoices,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// SubmitForApproval handles submit for approval HTTP requests
func (h *HTTPHandler) SubmitForApproval(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID       string `json:"id"`
		EntityID string `json:"entity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Get user ID from JWT token once PLT-1 (Identity/Authentication) is implemented
	submittedBy := ""

	if err := h.service.SubmitForApproval(r.Context(), req.ID, req.EntityID, submittedBy); err != nil{
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "submitted"})
}

// ApproveInvoice handles approve invoice HTTP requests
func (h *HTTPHandler) ApproveInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req service.ApproveInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Get user ID from JWT token once PLT-1 (Identity/Authentication) is implemented
	req.ApprovedBy = ""

	invoice, err := h.service.ApproveInvoice(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invoice)
}

// PostInvoice handles post invoice HTTP requests
func (h *HTTPHandler) PostInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req service.PostInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Get user ID from JWT token once PLT-1 (Identity/Authentication) is implemented
	req.PostedBy = ""

	invoice, err := h.service.PostInvoice(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invoice)
}

// RecordPayment handles record payment HTTP requests
func (h *HTTPHandler) RecordPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req service.RecordPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// TODO: Get user ID from JWT token
	// req.CreatedBy = "system" // Leave empty for NULL

	invoice, err := h.service.RecordPayment(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invoice)
}

// DeleteInvoice handles delete invoice HTTP requests
func (h *HTTPHandler) DeleteInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	invoiceID := r.URL.Query().Get("id")
	entityID := r.URL.Query().Get("entity_id")

	if invoiceID == "" || entityID == "" {
		http.Error(w, "Invoice ID and Entity ID are required", http.StatusBadRequest)
		return
	}

	if err := h.service.DeleteInvoice(r.Context(), invoiceID, entityID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
