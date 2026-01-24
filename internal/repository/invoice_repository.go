package repository

import (
	"context"
	"time"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pesio-ai/be-go-common/database"
	"github.com/pesio-ai/be-go-common/errors"
)

// Invoice represents an invoice header
type Invoice struct {
	ID                string
	EntityID          string
	VendorID          string
	InvoiceNumber     string
	InvoiceDate       string
	DueDate           string
	InvoiceType       string
	Status            string
	PaymentTerms      string
	DiscountPercent   *float64
	DiscountDueDate   *string
	Currency          string
	Subtotal          int64
	TaxAmount         int64
	TotalAmount       int64
	AmountPaid        int64
	AmountDue         int64
	PostedToGL        bool
	GLJournalID       *string
	PostedDate        *string
	PostedBy          *string
	ApprovedBy        *string
	ApprovedAt        *time.Time
	ApprovalNotes     *string
	PaymentMethod     *string
	PaymentReference  *string
	PaymentDate       *string
	PONumber          *string
	ReferenceNumber   *string
	Description       *string
	Notes             *string
	AttachmentURLs    []string
	CreatedBy         *string
	CreatedAt         time.Time
	UpdatedBy         *string
	UpdatedAt         time.Time
	Lines             []*InvoiceLine
}

// InvoiceLine represents an invoice line item
type InvoiceLine struct {
	ID          string
	InvoiceID   string
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
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// InvoicePayment represents a payment record
type InvoicePayment struct {
	ID               string
	InvoiceID        string
	PaymentDate      string
	PaymentAmount    int64
	PaymentMethod    *string
	PaymentReference *string
	Notes            *string
	CreatedBy        *string
	CreatedAt        time.Time
}

// InvoiceRepository handles invoice data operations
type InvoiceRepository struct {
	db *database.DB
}

// NewInvoiceRepository creates a new invoice repository
func NewInvoiceRepository(db *database.DB) *InvoiceRepository {
	return &InvoiceRepository{db: db}
}

// Create creates a new invoice with lines
func (r *InvoiceRepository) Create(ctx context.Context, invoice *Invoice) error {
	return r.db.InTransaction(ctx, func(tx pgx.Tx) error {
		// Insert invoice
		query := `
			INSERT INTO invoices (entity_id, vendor_id, invoice_number, invoice_date, due_date,
			                      invoice_type, status, payment_terms, discount_percent, discount_due_date,
			                      currency, po_number, reference_number, description, notes,
			                      attachment_urls, created_by)
			VALUES ($1, $2, $3, $4, $5, $6::invoice_type, $7::invoice_status, $8, $9, $10,
			        $11, $12, $13, $14, $15, $16, $17)
			RETURNING id, created_at, updated_at, subtotal, tax_amount, total_amount, amount_paid, amount_due
		`

		err := tx.QueryRow(ctx, query,
			invoice.EntityID,
			invoice.VendorID,
			invoice.InvoiceNumber,
			invoice.InvoiceDate,
			invoice.DueDate,
			invoice.InvoiceType,
			invoice.Status,
			invoice.PaymentTerms,
			invoice.DiscountPercent,
			invoice.DiscountDueDate,
			invoice.Currency,
			invoice.PONumber,
			invoice.ReferenceNumber,
			invoice.Description,
			invoice.Notes,
			invoice.AttachmentURLs,
			invoice.CreatedBy,
		).Scan(&invoice.ID, &invoice.CreatedAt, &invoice.UpdatedAt,
			&invoice.Subtotal, &invoice.TaxAmount, &invoice.TotalAmount,
			&invoice.AmountPaid, &invoice.AmountDue)

		if err != nil {
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to create invoice")
		}

		// Insert invoice lines
		for _, line := range invoice.Lines {
			lineQuery := `
				INSERT INTO invoice_lines (invoice_id, line_number, account_id, description,
				                          quantity, unit_price, line_amount,
				                          tax_code, tax_rate, tax_amount,
				                          dimension_1, dimension_2, dimension_3, dimension_4,
				                          item_code, item_name)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
				RETURNING id, created_at, updated_at
			`

			err := tx.QueryRow(ctx, lineQuery,
				invoice.ID,
				line.LineNumber,
				line.AccountID,
				line.Description,
				line.Quantity,
				line.UnitPrice,
				line.LineAmount,
				line.TaxCode,
				line.TaxRate,
				line.TaxAmount,
				line.Dimension1,
				line.Dimension2,
				line.Dimension3,
				line.Dimension4,
				line.ItemCode,
				line.ItemName,
			).Scan(&line.ID, &line.CreatedAt, &line.UpdatedAt)

			if err != nil {
				return errors.Wrap(err, errors.ErrCodeInternal, "failed to create invoice line")
			}

			line.InvoiceID = invoice.ID
		}

		// Refresh totals (trigger will update, but we need to read back)
		refreshQuery := `
			SELECT subtotal, tax_amount, total_amount, amount_paid, amount_due
			FROM invoices
			WHERE id = $1
		`
		err = tx.QueryRow(ctx, refreshQuery, invoice.ID).Scan(
			&invoice.Subtotal, &invoice.TaxAmount, &invoice.TotalAmount,
			&invoice.AmountPaid, &invoice.AmountDue)
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to refresh invoice totals")
		}

		return nil
	})
}

// GetByID retrieves an invoice by ID with all lines
func (r *InvoiceRepository) GetByID(ctx context.Context, id, entityID string) (*Invoice, error) {
	invoice := &Invoice{}

	query := `
		SELECT id, entity_id, vendor_id, invoice_number, invoice_date, due_date,
		       invoice_type, status, payment_terms, discount_percent, discount_due_date,
		       currency, subtotal, tax_amount, total_amount, amount_paid, amount_due,
		       posted_to_gl, gl_journal_id, posted_date, posted_by,
		       approved_by, approved_at, approval_notes,
		       payment_method, payment_reference, payment_date,
		       po_number, reference_number, description, notes, attachment_urls,
		       created_by, created_at, updated_by, updated_at
		FROM invoices
		WHERE id = $1 AND entity_id = $2
	`

	err := r.db.QueryRow(ctx, query, id, entityID).Scan(
		&invoice.ID,
		&invoice.EntityID,
		&invoice.VendorID,
		&invoice.InvoiceNumber,
		&invoice.InvoiceDate,
		&invoice.DueDate,
		&invoice.InvoiceType,
		&invoice.Status,
		&invoice.PaymentTerms,
		&invoice.DiscountPercent,
		&invoice.DiscountDueDate,
		&invoice.Currency,
		&invoice.Subtotal,
		&invoice.TaxAmount,
		&invoice.TotalAmount,
		&invoice.AmountPaid,
		&invoice.AmountDue,
		&invoice.PostedToGL,
		&invoice.GLJournalID,
		&invoice.PostedDate,
		&invoice.PostedBy,
		&invoice.ApprovedBy,
		&invoice.ApprovedAt,
		&invoice.ApprovalNotes,
		&invoice.PaymentMethod,
		&invoice.PaymentReference,
		&invoice.PaymentDate,
		&invoice.PONumber,
		&invoice.ReferenceNumber,
		&invoice.Description,
		&invoice.Notes,
		&invoice.AttachmentURLs,
		&invoice.CreatedBy,
		&invoice.CreatedAt,
		&invoice.UpdatedBy,
		&invoice.UpdatedAt,
	)

	if err == pgx.ErrNoRows {
		return nil, errors.NotFound("invoice", id)
	}
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to get invoice")
	}

	// Get lines
	lines, err := r.GetLines(ctx, invoice.ID)
	if err != nil {
		return nil, err
	}
	invoice.Lines = lines

	return invoice, nil
}

// GetLines retrieves all lines for an invoice
func (r *InvoiceRepository) GetLines(ctx context.Context, invoiceID string) ([]*InvoiceLine, error) {
	query := `
		SELECT id, invoice_id, line_number, account_id, description,
		       quantity, unit_price, line_amount,
		       tax_code, tax_rate, tax_amount,
		       dimension_1, dimension_2, dimension_3, dimension_4,
		       item_code, item_name,
		       created_at, updated_at
		FROM invoice_lines
		WHERE invoice_id = $1
		ORDER BY line_number
	`

	rows, err := r.db.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to get invoice lines")
	}
	defer rows.Close()

	lines := make([]*InvoiceLine, 0)
	for rows.Next() {
		line := &InvoiceLine{}
		err := rows.Scan(
			&line.ID,
			&line.InvoiceID,
			&line.LineNumber,
			&line.AccountID,
			&line.Description,
			&line.Quantity,
			&line.UnitPrice,
			&line.LineAmount,
			&line.TaxCode,
			&line.TaxRate,
			&line.TaxAmount,
			&line.Dimension1,
			&line.Dimension2,
			&line.Dimension3,
			&line.Dimension4,
			&line.ItemCode,
			&line.ItemName,
			&line.CreatedAt,
			&line.UpdatedAt,
		)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to scan invoice line")
		}

		lines = append(lines, line)
	}

	return lines, nil
}

// List retrieves invoices with filtering and pagination
func (r *InvoiceRepository) List(ctx context.Context, entityID string, vendorID, status *string, fromDate, toDate *string, limit, offset int) ([]*Invoice, int64, error) {
	query := `
		SELECT id, entity_id, vendor_id, invoice_number, invoice_date, due_date,
		       invoice_type, status, payment_terms, discount_percent, discount_due_date,
		       currency, subtotal, tax_amount, total_amount, amount_paid, amount_due,
		       posted_to_gl, gl_journal_id, posted_date, posted_by,
		       approved_by, approved_at, approval_notes,
		       payment_method, payment_reference, payment_date,
		       po_number, reference_number, description, notes, attachment_urls,
		       created_by, created_at, updated_by, updated_at
		FROM invoices
		WHERE entity_id = $1
	`

	countQuery := `SELECT COUNT(*) FROM invoices WHERE entity_id = $1`

	args := []interface{}{entityID}
	argCount := 2

	if vendorID != nil {
		query += fmt.Sprintf(" AND vendor_id = $%d", argCount)
		countQuery += fmt.Sprintf(" AND vendor_id = $%d", argCount)
		args = append(args, *vendorID)
		argCount++
	}

	if status != nil {
		query += fmt.Sprintf(" AND status = $%d::invoice_status", argCount)
		countQuery += fmt.Sprintf(" AND status = $%d::invoice_status", argCount)
		args = append(args, *status)
		argCount++
	}

	if fromDate != nil {
		query += fmt.Sprintf(" AND invoice_date >= $%d", argCount)
		countQuery += fmt.Sprintf(" AND invoice_date >= $%d", argCount)
		args = append(args, *fromDate)
		argCount++
	}

	if toDate != nil {
		query += fmt.Sprintf(" AND invoice_date <= $%d", argCount)
		countQuery += fmt.Sprintf(" AND invoice_date <= $%d", argCount)
		args = append(args, *toDate)
		argCount++
	}

	query += " ORDER BY invoice_date DESC, invoice_number DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCount, argCount+1)

	queryArgs := append(args, limit, offset)

	// Get total count
	var total int64
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeInternal, "failed to count invoices")
	}

	// Get invoices
	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, errors.Wrap(err, errors.ErrCodeInternal, "failed to list invoices")
	}
	defer rows.Close()

	invoices := make([]*Invoice, 0)
	for rows.Next() {
		invoice := &Invoice{}
		err := rows.Scan(
			&invoice.ID,
			&invoice.EntityID,
			&invoice.VendorID,
			&invoice.InvoiceNumber,
			&invoice.InvoiceDate,
			&invoice.DueDate,
			&invoice.InvoiceType,
			&invoice.Status,
			&invoice.PaymentTerms,
			&invoice.DiscountPercent,
			&invoice.DiscountDueDate,
			&invoice.Currency,
			&invoice.Subtotal,
			&invoice.TaxAmount,
			&invoice.TotalAmount,
			&invoice.AmountPaid,
			&invoice.AmountDue,
			&invoice.PostedToGL,
			&invoice.GLJournalID,
			&invoice.PostedDate,
			&invoice.PostedBy,
			&invoice.ApprovedBy,
			&invoice.ApprovedAt,
			&invoice.ApprovalNotes,
			&invoice.PaymentMethod,
			&invoice.PaymentReference,
			&invoice.PaymentDate,
			&invoice.PONumber,
			&invoice.ReferenceNumber,
			&invoice.Description,
			&invoice.Notes,
			&invoice.AttachmentURLs,
			&invoice.CreatedBy,
			&invoice.CreatedAt,
			&invoice.UpdatedBy,
			&invoice.UpdatedAt,
		)
		if err != nil {
			return nil, 0, errors.Wrap(err, errors.ErrCodeInternal, "failed to scan invoice")
		}

		invoices = append(invoices, invoice)
	}

	return invoices, total, nil
}

// UpdateStatus updates the status of an invoice
func (r *InvoiceRepository) UpdateStatus(ctx context.Context, id, entityID, status string, updatedBy *string) error {
	query := `
		UPDATE invoices
		SET status = $3::invoice_status,
		    updated_by = $4,
		    updated_at = NOW()
		WHERE id = $1 AND entity_id = $2
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, entityID, status, updatedBy).Scan(&returnedID)

	if err == pgx.ErrNoRows {
		return errors.NotFound("invoice", id)
	}
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to update invoice status")
	}

	return nil
}

// Approve approves an invoice
func (r *InvoiceRepository) Approve(ctx context.Context, id, entityID string, approvedBy *string, notes *string) error {
	query := `
		UPDATE invoices
		SET status = 'approved'::invoice_status,
		    approved_by = $3,
		    approved_at = NOW(),
		    approval_notes = $4,
		    updated_at = NOW()
		WHERE id = $1 AND entity_id = $2
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, entityID, approvedBy, notes).Scan(&returnedID)

	if err == pgx.ErrNoRows {
		return errors.NotFound("invoice", id)
	}
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to approve invoice")
	}

	return nil
}

// MarkAsPosted marks invoice as posted to GL
func (r *InvoiceRepository) MarkAsPosted(ctx context.Context, id, entityID, glJournalID string, postedBy *string) error {
	query := `
		UPDATE invoices
		SET status = 'posted'::invoice_status,
		    posted_to_gl = TRUE,
		    gl_journal_id = $3,
		    posted_date = CURRENT_DATE,
		    posted_by = $4,
		    updated_at = NOW()
		WHERE id = $1 AND entity_id = $2
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, entityID, glJournalID, postedBy).Scan(&returnedID)

	if err == pgx.ErrNoRows {
		return errors.NotFound("invoice", id)
	}
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to mark invoice as posted")
	}

	return nil
}

// RecordPayment records a payment against an invoice
func (r *InvoiceRepository) RecordPayment(ctx context.Context, payment *InvoicePayment) error {
	query := `
		INSERT INTO invoice_payments (invoice_id, payment_date, payment_amount,
		                               payment_method, payment_reference, notes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`

	err := r.db.QueryRow(ctx, query,
		payment.InvoiceID,
		payment.PaymentDate,
		payment.PaymentAmount,
		payment.PaymentMethod,
		payment.PaymentReference,
		payment.Notes,
		payment.CreatedBy,
	).Scan(&payment.ID, &payment.CreatedAt)

	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to record payment")
	}

	return nil
}

// Delete deletes a draft invoice
func (r *InvoiceRepository) Delete(ctx context.Context, id, entityID string) error {
	query := `
		DELETE FROM invoices
		WHERE id = $1 AND entity_id = $2 AND status = 'draft'
	`

	tag, err := r.db.Exec(ctx, query, id, entityID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to delete invoice")
	}

	if tag.RowsAffected() == 0 {
		return errors.New(errors.ErrCodeConflict, "cannot delete approved or posted invoice")
	}

	return nil
}
