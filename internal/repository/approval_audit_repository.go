package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/pesio-ai/be-lib-common/database"
	"github.com/pesio-ai/be-lib-common/errors"
)

// ApprovalAuditRepository appends and reads immutable approval audit log entries.
type ApprovalAuditRepository struct {
	db *database.DB
}

// NewApprovalAuditRepository creates a new ApprovalAuditRepository.
func NewApprovalAuditRepository(db *database.DB) *ApprovalAuditRepository {
	return &ApprovalAuditRepository{db: db}
}

// Append inserts one audit entry. The table has a delete-prevention trigger so
// this is the only mutation operation exposed.
func (r *ApprovalAuditRepository) Append(ctx context.Context, entry *ApprovalAuditEntry) error {
	var metadataJSON []byte
	if entry.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(entry.Metadata)
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to marshal audit metadata")
		}
	}

	query := `
		INSERT INTO invoice_approval_audit_log
		    (invoice_id, workflow_id, step_id, entity_id,
		     action, performed_by,
		     invoice_status_before, invoice_status_after,
		     metadata)
		VALUES ($1, $2, $3, $4,
		        $5, $6,
		        $7, $8,
		        $9)
		RETURNING id, performed_at
	`

	return r.db.QueryRow(ctx, query,
		entry.InvoiceID,
		entry.WorkflowID,
		entry.StepID,
		entry.EntityID,
		entry.Action,
		entry.PerformedBy,
		entry.InvoiceStatusBefore,
		entry.InvoiceStatusAfter,
		metadataJSON,
	).Scan(&entry.ID, &entry.PerformedAt)
}

// GetByInvoiceID returns the full audit trail for an invoice ordered oldest-first.
func (r *ApprovalAuditRepository) GetByInvoiceID(ctx context.Context, invoiceID, entityID string) ([]*ApprovalAuditEntry, error) {
	query := `
		SELECT id, invoice_id, workflow_id, step_id, entity_id,
		       action, performed_by, performed_at,
		       invoice_status_before, invoice_status_after,
		       metadata
		FROM invoice_approval_audit_log
		WHERE invoice_id = $1 AND entity_id = $2
		ORDER BY performed_at ASC
	`

	rows, err := r.db.Query(ctx, query, invoiceID, entityID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to get audit log")
	}
	defer rows.Close()

	return r.scanRows(rows)
}

// GetByWorkflowID returns all audit entries for a specific workflow.
func (r *ApprovalAuditRepository) GetByWorkflowID(ctx context.Context, workflowID string) ([]*ApprovalAuditEntry, error) {
	query := `
		SELECT id, invoice_id, workflow_id, step_id, entity_id,
		       action, performed_by, performed_at,
		       invoice_status_before, invoice_status_after,
		       metadata
		FROM invoice_approval_audit_log
		WHERE workflow_id = $1
		ORDER BY performed_at ASC
	`

	rows, err := r.db.Query(ctx, query, workflowID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to get workflow audit log")
	}
	defer rows.Close()

	return r.scanRows(rows)
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func (r *ApprovalAuditRepository) scanRows(rows pgx.Rows) ([]*ApprovalAuditEntry, error) {
	var entries []*ApprovalAuditEntry
	for rows.Next() {
		entry, err := r.scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

type auditScanner interface {
	Scan(dest ...any) error
}

func (r *ApprovalAuditRepository) scanEntry(sc auditScanner) (*ApprovalAuditEntry, error) {
	entry := &ApprovalAuditEntry{}
	var metadataJSON []byte

	err := sc.Scan(
		&entry.ID,
		&entry.InvoiceID,
		&entry.WorkflowID,
		&entry.StepID,
		&entry.EntityID,
		&entry.Action,
		&entry.PerformedBy,
		&entry.PerformedAt,
		&entry.InvoiceStatusBefore,
		&entry.InvoiceStatusAfter,
		&metadataJSON,
	)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to scan audit entry")
	}

	if metadataJSON != nil {
		if err := json.Unmarshal(metadataJSON, &entry.Metadata); err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to unmarshal audit metadata")
		}
	}

	return entry, nil
}
