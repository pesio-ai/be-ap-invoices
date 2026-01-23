package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pesio-ai/be-go-common/errors"
	"github.com/pesio-ai/be-go-common/logger"
	"github.com/pesio-ai/be-invoices-service/internal/repository"
)

// InvoiceService handles invoice business logic
type InvoiceService struct {
	invoiceRepo *repository.InvoiceRepository
	log         *logger.Logger
}

// NewInvoiceService creates a new invoice service
func NewInvoiceService(
	invoiceRepo *repository.InvoiceRepository,
	log *logger.Logger,
) *InvoiceService {
	return &InvoiceService{
		invoiceRepo: invoiceRepo,
		log:         log,
	}
}

// CreateInvoiceRequest represents a create invoice request
type CreateInvoiceRequest struct {
	EntityID        string
	VendorID        string
	InvoiceNumber   string
	InvoiceDate     string
	DueDate         string
	InvoiceType     string
	PaymentTerms    string
	DiscountPercent *float64
	DiscountDueDate *string
	Currency        string
	PONumber        *string
	ReferenceNumber *string
	Description     *string
	Notes           *string
	AttachmentURLs  []string
	Lines           []*InvoiceLineRequest
	CreatedBy       string
}

// InvoiceLineRequest represents an invoice line request
type InvoiceLineRequest struct {
	LineNumber  int
	AccountID   string
	Description string
	Quantity    float64
	UnitPrice   int64
	LineAmount  int64
	TaxCode     *string
	TaxRate     *float64
	TaxAmount   int64
	Dimension1  *string
	Dimension2  *string
	Dimension3  *string
	Dimension4  *string
	ItemCode    *string
	ItemName    *string
}

// ApproveInvoiceRequest represents an approve invoice request
type ApproveInvoiceRequest struct {
	ID         string
	EntityID   string
	ApprovedBy string
	Notes      *string
}

// PostInvoiceRequest represents a post invoice request
type PostInvoiceRequest struct {
	ID       string
	EntityID string
	PostedBy string
}

// RecordPaymentRequest represents a record payment request
type RecordPaymentRequest struct {
	InvoiceID        string
	EntityID         string
	PaymentDate      string
	PaymentAmount    int64
	PaymentMethod    *string
	PaymentReference *string
	Notes            *string
	CreatedBy        string
}

// CreateInvoice creates a new invoice
func (s *InvoiceService) CreateInvoice(ctx context.Context, req *CreateInvoiceRequest) (*repository.Invoice, error) {
	// TODO: Validate vendor exists and is active (call AP-1 vendors service)
	// For now, assume vendor is valid

	// Validate invoice type
	validTypes := map[string]bool{
		"standard":   true,
		"credit_memo": true,
		"debit_memo":  true,
		"prepayment":  true,
		"recurring":   true,
	}
	invoiceType := strings.ToLower(req.InvoiceType)
	if !validTypes[invoiceType] {
		return nil, errors.InvalidInput("invoice_type", "invalid invoice type")
	}

	// Validate dates
	invoiceDate, err := time.Parse("2006-01-02", req.InvoiceDate)
	if err != nil {
		return nil, errors.InvalidInput("invoice_date", "invalid date format, expected YYYY-MM-DD")
	}

	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		return nil, errors.InvalidInput("due_date", "invalid date format, expected YYYY-MM-DD")
	}

	if dueDate.Before(invoiceDate) {
		return nil, errors.InvalidInput("due_date", "due date cannot be before invoice date")
	}

	// Validate currency
	if len(req.Currency) != 3 {
		return nil, errors.InvalidInput("currency", "currency must be 3-letter ISO code")
	}

	// Validate lines exist
	if len(req.Lines) < 1 {
		return nil, errors.InvalidInput("lines", "invoice must have at least 1 line")
	}

	// Validate discount
	if req.DiscountPercent != nil && (*req.DiscountPercent < 0 || *req.DiscountPercent > 100) {
		return nil, errors.InvalidInput("discount_percent", "discount must be between 0 and 100")
	}

	// Build invoice
	invoice := &repository.Invoice{
		EntityID:        req.EntityID,
		VendorID:        req.VendorID,
		InvoiceNumber:   req.InvoiceNumber,
		InvoiceDate:     req.InvoiceDate,
		DueDate:         req.DueDate,
		InvoiceType:     invoiceType,
		Status:          "draft",
		PaymentTerms:    req.PaymentTerms,
		DiscountPercent: req.DiscountPercent,
		DiscountDueDate: req.DiscountDueDate,
		Currency:        strings.ToUpper(req.Currency),
		PONumber:        req.PONumber,
		ReferenceNumber: req.ReferenceNumber,
		Description:     req.Description,
		Notes:           req.Notes,
		AttachmentURLs:  req.AttachmentURLs,
		CreatedBy:       &req.CreatedBy,
		Lines:           make([]*repository.InvoiceLine, 0),
	}

	// Validate and build lines
	accountsSeen := make(map[string]bool)

	for _, lineReq := range req.Lines {
		// Validate quantity
		if lineReq.Quantity <= 0 {
			return nil, errors.InvalidInput("quantity", "quantity must be positive")
		}

		// Validate amounts
		if lineReq.UnitPrice < 0 {
			return nil, errors.InvalidInput("unit_price", "unit price cannot be negative")
		}

		if lineReq.LineAmount < 0 {
			return nil, errors.InvalidInput("line_amount", "line amount cannot be negative")
		}

		if lineReq.TaxAmount < 0 {
			return nil, errors.InvalidInput("tax_amount", "tax amount cannot be negative")
		}

		// Validate tax rate
		if lineReq.TaxRate != nil && (*lineReq.TaxRate < 0 || *lineReq.TaxRate > 100) {
			return nil, errors.InvalidInput("tax_rate", "tax rate must be between 0 and 100")
		}

		// TODO: Validate account exists and allows posting (call GL-1 accounts service)
		accountsSeen[lineReq.AccountID] = true

		line := &repository.InvoiceLine{
			LineNumber:  lineReq.LineNumber,
			AccountID:   lineReq.AccountID,
			Description: lineReq.Description,
			Quantity:    lineReq.Quantity,
			UnitPrice:   lineReq.UnitPrice,
			LineAmount:  lineReq.LineAmount,
			TaxCode:     lineReq.TaxCode,
			TaxRate:     lineReq.TaxRate,
			TaxAmount:   lineReq.TaxAmount,
			Dimension1:  lineReq.Dimension1,
			Dimension2:  lineReq.Dimension2,
			Dimension3:  lineReq.Dimension3,
			Dimension4:  lineReq.Dimension4,
			ItemCode:    lineReq.ItemCode,
			ItemName:    lineReq.ItemName,
		}

		invoice.Lines = append(invoice.Lines, line)
	}

	// Create invoice
	if err := s.invoiceRepo.Create(ctx, invoice); err != nil {
		return nil, err
	}

	s.log.Info().
		Str("invoice_id", invoice.ID).
		Str("invoice_number", invoice.InvoiceNumber).
		Str("vendor_id", req.VendorID).
		Str("entity_id", req.EntityID).
		Int64("total_amount", invoice.TotalAmount).
		Int("line_count", len(invoice.Lines)).
		Msg("Invoice created")

	return invoice, nil
}

// GetInvoice retrieves an invoice by ID
func (s *InvoiceService) GetInvoice(ctx context.Context, id, entityID string) (*repository.Invoice, error) {
	return s.invoiceRepo.GetByID(ctx, id, entityID)
}

// ListInvoices lists invoices with filtering and pagination
func (s *InvoiceService) ListInvoices(ctx context.Context, entityID string, vendorID, status *string, fromDate, toDate *string, page, pageSize int) ([]*repository.Invoice, int64, error) {
	offset := (page - 1) * pageSize
	return s.invoiceRepo.List(ctx, entityID, vendorID, status, fromDate, toDate, pageSize, offset)
}

// ApproveInvoice approves an invoice
func (s *InvoiceService) ApproveInvoice(ctx context.Context, req *ApproveInvoiceRequest) (*repository.Invoice, error) {
	// Get invoice
	invoice, err := s.invoiceRepo.GetByID(ctx, req.ID, req.EntityID)
	if err != nil {
		return nil, err
	}

	// Validate status
	if invoice.Status != "pending_approval" {
		return nil, errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("cannot approve invoice with status '%s'", invoice.Status))
	}

	// TODO: Validate vendor is still active
	// TODO: Validate all accounts are still active and allow posting

	// Approve invoice
	if err := s.invoiceRepo.Approve(ctx, req.ID, req.EntityID, req.ApprovedBy, req.Notes); err != nil {
		return nil, err
	}

	s.log.Info().
		Str("invoice_id", req.ID).
		Str("invoice_number", invoice.InvoiceNumber).
		Str("approved_by", req.ApprovedBy).
		Msg("Invoice approved")

	// Retrieve updated invoice
	return s.invoiceRepo.GetByID(ctx, req.ID, req.EntityID)
}

// PostInvoice posts an invoice to GL
func (s *InvoiceService) PostInvoice(ctx context.Context, req *PostInvoiceRequest) (*repository.Invoice, error) {
	// Get invoice
	invoice, err := s.invoiceRepo.GetByID(ctx, req.ID, req.EntityID)
	if err != nil {
		return nil, err
	}

	// Validate status
	if invoice.Status != "approved" {
		return nil, errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("cannot post invoice with status '%s', must be approved", invoice.Status))
	}

	// Validate not already posted
	if invoice.PostedToGL {
		return nil, errors.New(errors.ErrCodeConflict, "invoice has already been posted to GL")
	}

	// TODO: Call GL-2 journals service to create journal entry
	// For now, simulate GL posting
	glJournalID := fmt.Sprintf("JE-%s", invoice.InvoiceNumber)

	// The journal entry would look like:
	// Debit: GL accounts from invoice lines (total_amount)
	// Credit: Accounts Payable (total_amount)

	// TODO: Update vendor balance in AP-1 vendors service
	// vendor.current_balance += invoice.TotalAmount

	// Mark as posted
	if err := s.invoiceRepo.MarkAsPosted(ctx, req.ID, req.EntityID, glJournalID, req.PostedBy); err != nil {
		return nil, err
	}

	s.log.Info().
		Str("invoice_id", req.ID).
		Str("invoice_number", invoice.InvoiceNumber).
		Str("gl_journal_id", glJournalID).
		Str("posted_by", req.PostedBy).
		Int64("total_amount", invoice.TotalAmount).
		Msg("Invoice posted to GL")

	// Retrieve updated invoice
	return s.invoiceRepo.GetByID(ctx, req.ID, req.EntityID)
}

// RecordPayment records a payment against an invoice
func (s *InvoiceService) RecordPayment(ctx context.Context, req *RecordPaymentRequest) (*repository.Invoice, error) {
	// Get invoice
	invoice, err := s.invoiceRepo.GetByID(ctx, req.InvoiceID, req.EntityID)
	if err != nil {
		return nil, err
	}

	// Validate invoice is posted
	if invoice.Status != "posted" && invoice.Status != "paid" {
		return nil, errors.New(errors.ErrCodeConflict, "can only record payments for posted invoices")
	}

	// Validate payment amount
	if req.PaymentAmount <= 0 {
		return nil, errors.InvalidInput("payment_amount", "payment amount must be positive")
	}

	// Validate not overpaying
	if req.PaymentAmount > invoice.AmountDue {
		return nil, errors.InvalidInput("payment_amount",
			fmt.Sprintf("payment amount (%d) exceeds amount due (%d)", req.PaymentAmount, invoice.AmountDue))
	}

	// Validate payment date
	if _, err := time.Parse("2006-01-02", req.PaymentDate); err != nil {
		return nil, errors.InvalidInput("payment_date", "invalid date format, expected YYYY-MM-DD")
	}

	// Record payment
	payment := &repository.InvoicePayment{
		InvoiceID:        req.InvoiceID,
		PaymentDate:      req.PaymentDate,
		PaymentAmount:    req.PaymentAmount,
		PaymentMethod:    req.PaymentMethod,
		PaymentReference: req.PaymentReference,
		Notes:            req.Notes,
		CreatedBy:        &req.CreatedBy,
	}

	if err := s.invoiceRepo.RecordPayment(ctx, payment); err != nil {
		return nil, err
	}

	// TODO: Create payment journal entry in GL-2
	// Debit: Cash/Bank account
	// Credit: Accounts Payable

	// TODO: Update vendor balance in AP-1
	// vendor.current_balance -= payment.PaymentAmount

	s.log.Info().
		Str("invoice_id", req.InvoiceID).
		Str("invoice_number", invoice.InvoiceNumber).
		Str("payment_id", payment.ID).
		Int64("payment_amount", req.PaymentAmount).
		Msg("Payment recorded")

	// Retrieve updated invoice
	return s.invoiceRepo.GetByID(ctx, req.InvoiceID, req.EntityID)
}

// SubmitForApproval submits an invoice for approval
func (s *InvoiceService) SubmitForApproval(ctx context.Context, id, entityID, submittedBy string) error {
	// Get invoice
	invoice, err := s.invoiceRepo.GetByID(ctx, id, entityID)
	if err != nil {
		return err
	}

	// Validate status
	if invoice.Status != "draft" {
		return errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("cannot submit invoice with status '%s' for approval", invoice.Status))
	}

	// Validate has lines
	if len(invoice.Lines) < 1 {
		return errors.InvalidInput("lines", "invoice must have at least 1 line")
	}

	// Update status
	if err := s.invoiceRepo.UpdateStatus(ctx, id, entityID, "pending_approval", &submittedBy); err != nil {
		return err
	}

	s.log.Info().
		Str("invoice_id", id).
		Str("invoice_number", invoice.InvoiceNumber).
		Str("submitted_by", submittedBy).
		Msg("Invoice submitted for approval")

	return nil
}

// DeleteInvoice deletes a draft invoice
func (s *InvoiceService) DeleteInvoice(ctx context.Context, id, entityID string) error {
	// Verify invoice exists and is draft
	invoice, err := s.invoiceRepo.GetByID(ctx, id, entityID)
	if err != nil {
		return err
	}

	if invoice.Status != "draft" {
		return errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("cannot delete invoice with status '%s'", invoice.Status))
	}

	if err := s.invoiceRepo.Delete(ctx, id, entityID); err != nil {
		return err
	}

	s.log.Info().
		Str("invoice_id", id).
		Str("invoice_number", invoice.InvoiceNumber).
		Msg("Invoice deleted")

	return nil
}
