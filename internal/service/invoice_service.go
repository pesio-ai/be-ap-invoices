package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pesio-ai/be-lib-common/errors"
	"github.com/pesio-ai/be-lib-common/logger"
	"github.com/pesio-ai/be-ap-invoices/internal/client"
	"github.com/pesio-ai/be-ap-invoices/internal/repository"
)

// InvoiceService handles invoice business logic
type InvoiceService struct {
	invoiceRepo    *repository.InvoiceRepository
	vendorsClient  client.VendorsClientInterface
	accountsClient client.AccountsClientInterface
	journalsClient client.JournalsClientInterface
	log            *logger.Logger
}

// NewInvoiceService creates a new invoice service
func NewInvoiceService(
	invoiceRepo *repository.InvoiceRepository,
	vendorsClient client.VendorsClientInterface,
	accountsClient client.AccountsClientInterface,
	journalsClient client.JournalsClientInterface,
	log *logger.Logger,
) *InvoiceService {
	return &InvoiceService{
		invoiceRepo:    invoiceRepo,
		vendorsClient:  vendorsClient,
		accountsClient: accountsClient,
		journalsClient: journalsClient,
		log:            log,
	}
}

// CreateInvoiceRequest represents a create invoice request
type CreateInvoiceRequest struct {
	EntityID        string                 `json:"entity_id"`
	VendorID        string                 `json:"vendor_id"`
	InvoiceNumber   string                 `json:"invoice_number"`
	InvoiceDate     string                 `json:"invoice_date"`
	DueDate         string                 `json:"due_date"`
	InvoiceType     string                 `json:"invoice_type"`
	PaymentTerms    string                 `json:"payment_terms"`
	DiscountPercent *float64               `json:"discount_percent,omitempty"`
	DiscountDueDate *string                `json:"discount_due_date,omitempty"`
	Currency        string                 `json:"currency"`
	PONumber        *string                `json:"po_number,omitempty"`
	ReferenceNumber *string                `json:"reference_number,omitempty"`
	Description     *string                `json:"description,omitempty"`
	Notes           *string                `json:"notes,omitempty"`
	AttachmentURLs  []string               `json:"attachment_urls,omitempty"`
	Lines           []*InvoiceLineRequest  `json:"lines"`
	CreatedBy       string                 `json:"created_by,omitempty"`
}

// InvoiceLineRequest represents an invoice line request
type InvoiceLineRequest struct {
	LineNumber  int      `json:"line_number"`
	AccountID   string   `json:"account_id"`
	Description string   `json:"description"`
	Quantity    float64  `json:"quantity"`
	UnitPrice   int64    `json:"unit_price"`
	LineAmount  int64    `json:"line_amount"`
	TaxCode     *string  `json:"tax_code,omitempty"`
	TaxRate     *float64 `json:"tax_rate,omitempty"`
	TaxAmount   int64    `json:"tax_amount"`
	Dimension1  *string  `json:"dimension1,omitempty"`
	Dimension2  *string  `json:"dimension2,omitempty"`
	Dimension3  *string  `json:"dimension3,omitempty"`
	Dimension4  *string  `json:"dimension4,omitempty"`
	ItemCode    *string  `json:"item_code,omitempty"`
	ItemName    *string  `json:"item_name,omitempty"`
}

// ApproveInvoiceRequest represents an approve invoice request
type ApproveInvoiceRequest struct {
	ID         string  `json:"id"`
	EntityID   string  `json:"entity_id"`
	ApprovedBy string  `json:"approved_by"`
	Notes      *string `json:"notes,omitempty"`
}

// PostInvoiceRequest represents a post invoice request
type PostInvoiceRequest struct {
	ID       string `json:"id"`
	EntityID string `json:"entity_id"`
	PostedBy string `json:"posted_by"`
}

// RecordPaymentRequest represents a record payment request
type RecordPaymentRequest struct {
	InvoiceID        string  `json:"invoice_id"`
	EntityID         string  `json:"entity_id"`
	PaymentDate      string  `json:"payment_date"`
	PaymentAmount    int64   `json:"payment_amount"`
	PaymentMethod    *string `json:"payment_method,omitempty"`
	PaymentReference *string `json:"payment_reference,omitempty"`
	Notes            *string `json:"notes,omitempty"`
	CreatedBy        string  `json:"created_by,omitempty"`
}

// CreateInvoice creates a new invoice
func (s *InvoiceService) CreateInvoice(ctx context.Context, req *CreateInvoiceRequest) (*repository.Invoice, error) {
	// Validate vendor exists and is active
	valid, message, err := s.vendorsClient.ValidateVendor(ctx, req.VendorID, req.EntityID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate vendor: %w", err)
	}
	if !valid {
		return nil, errors.InvalidInput("vendor_id", message)
	}

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

	// Convert empty string to NULL for CreatedBy
	var createdBy *string
	if req.CreatedBy != "" {
		createdBy = &req.CreatedBy
	}

	// Parse optional discount due date
	var discountDueDate *time.Time
	if req.DiscountDueDate != nil && *req.DiscountDueDate != "" {
		parsedDiscountDueDate, err := time.Parse("2006-01-02", *req.DiscountDueDate)
		if err != nil {
			return nil, errors.InvalidInput("discount_due_date", "invalid date format, expected YYYY-MM-DD")
		}
		discountDueDate = &parsedDiscountDueDate
	}

	// Build invoice
	invoice := &repository.Invoice{
		EntityID:        req.EntityID,
		VendorID:        req.VendorID,
		InvoiceNumber:   req.InvoiceNumber,
		InvoiceDate:     invoiceDate,
		DueDate:         dueDate,
		InvoiceType:     invoiceType,
		Status:          "draft",
		PaymentTerms:    req.PaymentTerms,
		DiscountPercent: req.DiscountPercent,
		DiscountDueDate: discountDueDate,
		Currency:        strings.ToUpper(req.Currency),
		PONumber:        req.PONumber,
		ReferenceNumber: req.ReferenceNumber,
		Description:     req.Description,
		Notes:           req.Notes,
		AttachmentURLs:  req.AttachmentURLs,
		CreatedBy:       createdBy,
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

		// Validate account exists and allows posting (only validate each account once)
		if !accountsSeen[lineReq.AccountID] {
			valid, message, err := s.accountsClient.ValidateAccount(ctx, lineReq.AccountID, req.EntityID)
			if err != nil {
				return nil, fmt.Errorf("failed to validate account %s: %w", lineReq.AccountID, err)
			}
			if !valid {
				return nil, errors.InvalidInput("account_id", fmt.Sprintf("account %s: %s", lineReq.AccountID, message))
			}
			accountsSeen[lineReq.AccountID] = true
		}

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

	// Convert empty string to NULL for ApprovedBy
	var approvedBy *string
	if req.ApprovedBy != "" {
		approvedBy = &req.ApprovedBy
	}

	// Approve invoice
	if err := s.invoiceRepo.Approve(ctx, req.ID, req.EntityID, approvedBy, req.Notes); err != nil {
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

	// Build journal entry lines from invoice
	journalLines := make([]*client.JournalLineRequest, 0, len(invoice.Lines)+1)

	// Debit lines from invoice line items
	for i, line := range invoice.Lines {
		desc := fmt.Sprintf("Invoice %s - %s", invoice.InvoiceNumber, line.Description)
		journalLines = append(journalLines, &client.JournalLineRequest{
			LineNumber:  i + 1,
			AccountID:   line.AccountID,
			LineType:    "debit",
			Amount:      line.LineAmount + line.TaxAmount,
			Description: &desc,
			Dimension1:  line.Dimension1,
			Dimension2:  line.Dimension2,
			Dimension3:  line.Dimension3,
			Dimension4:  line.Dimension4,
			Reference:   &invoice.InvoiceNumber,
		})
	}

	// Credit line to Accounts Payable
	// TODO: Make AP account configurable via entity settings
	apAccountCode := "2000" // Default Accounts Payable account
	apAccount, err := s.accountsClient.GetAccountByCode(ctx, apAccountCode, req.EntityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AP account: %w", err)
	}

	apDesc := fmt.Sprintf("Invoice %s from vendor %s", invoice.InvoiceNumber, invoice.VendorID)
	journalLines = append(journalLines, &client.JournalLineRequest{
		LineNumber:  len(invoice.Lines) + 1,
		AccountID:   apAccount.ID,
		LineType:    "credit",
		Amount:      invoice.TotalAmount,
		Description: &apDesc,
		Reference:   &invoice.InvoiceNumber,
	})

	// Create journal entry in GL-2
	journalReq := &client.CreateJournalRequest{
		EntityID:      req.EntityID,
		JournalNumber: fmt.Sprintf("AP-INV-%s", invoice.InvoiceNumber),
		JournalDate:   invoice.InvoiceDate.Format("2006-01-02"),
		JournalType:   "ap_invoice",
		Description:   invoice.Description,
		Reference:     &invoice.InvoiceNumber,
		Currency:      invoice.Currency,
		Lines:         journalLines,
	}

	glJournalID, err := s.journalsClient.CreateJournal(ctx, journalReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create journal entry: %w", err)
	}

	// Post the journal entry immediately
	if err := s.journalsClient.PostJournal(ctx, glJournalID, req.EntityID); err != nil {
		return nil, fmt.Errorf("failed to post journal entry: %w", err)
	}

	// Update vendor balance in AP-1
	if err := s.vendorsClient.UpdateBalance(ctx, invoice.VendorID, req.EntityID, invoice.TotalAmount); err != nil {
		return nil, fmt.Errorf("failed to update vendor balance: %w", err)
	}

	// Convert empty string to NULL for PostedBy
	var postedBy *string
	if req.PostedBy != "" {
		postedBy = &req.PostedBy
	}

	// Mark as posted
	if err := s.invoiceRepo.MarkAsPosted(ctx, req.ID, req.EntityID, glJournalID, postedBy); err != nil {
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

	// Validate and parse payment date
	paymentDate, err := time.Parse("2006-01-02", req.PaymentDate)
	if err != nil {
		return nil, errors.InvalidInput("payment_date", "invalid date format, expected YYYY-MM-DD")
	}

	// Convert empty string to NULL for CreatedBy
	var createdBy *string
	if req.CreatedBy != "" {
		createdBy = &req.CreatedBy
	}

	// Record payment
	payment := &repository.InvoicePayment{
		InvoiceID:        req.InvoiceID,
		PaymentDate:      paymentDate,
		PaymentAmount:    req.PaymentAmount,
		PaymentMethod:    req.PaymentMethod,
		PaymentReference: req.PaymentReference,
		Notes:            req.Notes,
		CreatedBy:        createdBy,
	}

	if err := s.invoiceRepo.RecordPayment(ctx, payment); err != nil {
		return nil, err
	}

	// Create payment journal entry in GL-2
	// TODO: Make cash/bank account configurable via entity settings
	cashAccountCode := "1010" // Default Cash account
	apAccountCode := "2000"    // Default Accounts Payable account

	// Look up account IDs by code
	cashAccount, err := s.accountsClient.GetAccountByCode(ctx, cashAccountCode, req.EntityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cash account: %w", err)
	}

	apAccount, err := s.accountsClient.GetAccountByCode(ctx, apAccountCode, req.EntityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get AP account: %w", err)
	}

	paymentDesc := fmt.Sprintf("Payment for invoice %s", invoice.InvoiceNumber)
	paymentRef := fmt.Sprintf("Payment-%s", payment.ID)

	journalLines := []*client.JournalLineRequest{
		{
			LineNumber:  1,
			AccountID:   cashAccount.ID,
			LineType:    "debit",
			Amount:      req.PaymentAmount,
			Description: &paymentDesc,
			Reference:   &paymentRef,
		},
		{
			LineNumber:  2,
			AccountID:   apAccount.ID,
			LineType:    "credit",
			Amount:      req.PaymentAmount,
			Description: &paymentDesc,
			Reference:   &paymentRef,
		},
	}

	journalReq := &client.CreateJournalRequest{
		EntityID:      req.EntityID,
		JournalNumber: fmt.Sprintf("AP-PMT-%s", payment.ID),
		JournalDate:   req.PaymentDate,
		JournalType:   "ap_payment",
		Description:   req.Notes,
		Reference:     &paymentRef,
		Currency:      invoice.Currency,
		Lines:         journalLines,
	}

	glJournalID, err := s.journalsClient.CreateJournal(ctx, journalReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create payment journal entry: %w", err)
	}

	// Post the payment journal entry immediately
	if err := s.journalsClient.PostJournal(ctx, glJournalID, req.EntityID); err != nil {
		return nil, fmt.Errorf("failed to post payment journal entry: %w", err)
	}

	// Update vendor balance in AP-1 (decrease balance)
	if err := s.vendorsClient.UpdateBalance(ctx, invoice.VendorID, req.EntityID, -req.PaymentAmount); err != nil {
		return nil, fmt.Errorf("failed to update vendor balance: %w", err)
	}

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

	// Convert empty string to NULL for submitted_by
	var submittedByPtr *string
	if submittedBy != "" {
		submittedByPtr = &submittedBy
	}

	// Update status
	if err := s.invoiceRepo.UpdateStatus(ctx, id, entityID, "pending_approval", submittedByPtr); err != nil {
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
