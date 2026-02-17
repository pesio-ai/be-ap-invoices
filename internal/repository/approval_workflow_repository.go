package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pesio-ai/be-lib-common/database"
	"github.com/pesio-ai/be-lib-common/errors"
)

// ApprovalWorkflowRepository manages workflow instances and their steps.
// Workflow + step creation is always done together in a single transaction.
type ApprovalWorkflowRepository struct {
	db *database.DB
}

// NewApprovalWorkflowRepository creates a new ApprovalWorkflowRepository.
func NewApprovalWorkflowRepository(db *database.DB) *ApprovalWorkflowRepository {
	return &ApprovalWorkflowRepository{db: db}
}

// Create inserts a workflow and its initial steps in one transaction.
func (r *ApprovalWorkflowRepository) Create(ctx context.Context, wf *ApprovalWorkflow, steps []*ApprovalWorkflowStep) error {
	return r.db.InTransaction(ctx, func(tx pgx.Tx) error {
		// Insert workflow
		wfQuery := `
			INSERT INTO invoice_approval_workflows
			    (invoice_id, entity_id, rule_id, status,
			     total_steps, current_step, submitted_by, submission_notes)
			VALUES ($1, $2, $3, $4::approval_workflow_status,
			        $5, $6, $7, $8)
			RETURNING id, submitted_at, created_at, updated_at
		`

		err := tx.QueryRow(ctx, wfQuery,
			wf.InvoiceID,
			wf.EntityID,
			wf.RuleID,
			wf.Status,
			wf.TotalSteps,
			wf.CurrentStep,
			wf.SubmittedBy,
			wf.SubmissionNotes,
		).Scan(&wf.ID, &wf.SubmittedAt, &wf.CreatedAt, &wf.UpdatedAt)
		if err != nil {
			return errors.Wrap(err, errors.ErrCodeInternal, "failed to create approval workflow")
		}

		// Insert steps
		stepQuery := `
			INSERT INTO invoice_approval_steps
			    (workflow_id, invoice_id, entity_id,
			     step_number, required_role, is_required,
			     assigned_to, assigned_at, due_at, status)
			VALUES ($1, $2, $3,
			        $4, $5, $6,
			        $7, $8, $9, $10::approval_step_status)
			RETURNING id, created_at, updated_at
		`

		for _, step := range steps {
			step.WorkflowID = wf.ID
			step.InvoiceID = wf.InvoiceID
			step.EntityID = wf.EntityID

			err := tx.QueryRow(ctx, stepQuery,
				step.WorkflowID,
				step.InvoiceID,
				step.EntityID,
				step.StepNumber,
				step.RequiredRole,
				step.IsRequired,
				step.AssignedTo,
				step.AssignedAt,
				step.DueAt,
				step.Status,
			).Scan(&step.ID, &step.CreatedAt, &step.UpdatedAt)
			if err != nil {
				return errors.Wrap(err, errors.ErrCodeInternal, "failed to create approval step")
			}
		}

		return nil
	})
}

// GetByID retrieves a workflow by its primary key.
func (r *ApprovalWorkflowRepository) GetByID(ctx context.Context, id string) (*ApprovalWorkflow, error) {
	query := `
		SELECT id, invoice_id, entity_id, rule_id, status,
		       total_steps, current_step,
		       submitted_by, submitted_at,
		       completed_at, submission_notes,
		       created_at, updated_at
		FROM invoice_approval_workflows
		WHERE id = $1
	`

	wf, err := r.scanWorkflow(r.db.QueryRow(ctx, query, id))
	if err == pgx.ErrNoRows {
		return nil, errors.NotFound("approval_workflow", id)
	}
	return wf, err
}

// GetActiveByInvoiceID returns the most recent (active) workflow for an invoice.
// Returns nil when no workflow exists yet.
func (r *ApprovalWorkflowRepository) GetActiveByInvoiceID(ctx context.Context, invoiceID string) (*ApprovalWorkflow, error) {
	query := `
		SELECT id, invoice_id, entity_id, rule_id, status,
		       total_steps, current_step,
		       submitted_by, submitted_at,
		       completed_at, submission_notes,
		       created_at, updated_at
		FROM invoice_approval_workflows
		WHERE invoice_id = $1
		  AND status IN ('pending', 'in_progress')
		ORDER BY submitted_at DESC
		LIMIT 1
	`

	wf, err := r.scanWorkflow(r.db.QueryRow(ctx, query, invoiceID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return wf, err
}

// UpdateStatus sets the workflow status and optionally stamps completed_at.
func (r *ApprovalWorkflowRepository) UpdateStatus(ctx context.Context, id, status string, completedAt *time.Time) error {
	query := `
		UPDATE invoice_approval_workflows
		SET status       = $2::approval_workflow_status,
		    completed_at = $3,
		    updated_at   = NOW()
		WHERE id = $1
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, status, completedAt).Scan(&returnedID)
	if err == pgx.ErrNoRows {
		return errors.NotFound("approval_workflow", id)
	}
	return err
}

// AdvanceStep increments current_step and sets status to in_progress.
func (r *ApprovalWorkflowRepository) AdvanceStep(ctx context.Context, id string, nextStep int) error {
	query := `
		UPDATE invoice_approval_workflows
		SET current_step = $2,
		    status       = 'in_progress'::approval_workflow_status,
		    updated_at   = NOW()
		WHERE id = $1
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, nextStep).Scan(&returnedID)
	if err == pgx.ErrNoRows {
		return errors.NotFound("approval_workflow", id)
	}
	return err
}

// ── scan helper ───────────────────────────────────────────────────────────────

type workflowScanner interface {
	Scan(dest ...any) error
}

func (r *ApprovalWorkflowRepository) scanWorkflow(row workflowScanner) (*ApprovalWorkflow, error) {
	wf := &ApprovalWorkflow{}
	err := row.Scan(
		&wf.ID,
		&wf.InvoiceID,
		&wf.EntityID,
		&wf.RuleID,
		&wf.Status,
		&wf.TotalSteps,
		&wf.CurrentStep,
		&wf.SubmittedBy,
		&wf.SubmittedAt,
		&wf.CompletedAt,
		&wf.SubmissionNotes,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return wf, nil
}
