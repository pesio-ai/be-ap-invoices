package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/pesio-ai/be-lib-common/database"
	"github.com/pesio-ai/be-lib-common/errors"
)

// ApprovalStepsRepository handles reads and updates on individual approval steps.
// Step creation is handled by ApprovalWorkflowRepository.Create (transactionally).
type ApprovalStepsRepository struct {
	db *database.DB
}

// NewApprovalStepsRepository creates a new ApprovalStepsRepository.
func NewApprovalStepsRepository(db *database.DB) *ApprovalStepsRepository {
	return &ApprovalStepsRepository{db: db}
}

// GetByWorkflowID returns all steps for a workflow ordered by step_number.
func (r *ApprovalStepsRepository) GetByWorkflowID(ctx context.Context, workflowID string) ([]*ApprovalWorkflowStep, error) {
	query := `
		SELECT id, workflow_id, invoice_id, entity_id,
		       step_number, required_role, is_required,
		       assigned_to, assigned_at,
		       delegated_to, delegated_at, delegated_reason,
		       status, acted_by, acted_at, action_notes, due_at,
		       created_at, updated_at
		FROM invoice_approval_steps
		WHERE workflow_id = $1
		ORDER BY step_number ASC
	`

	rows, err := r.db.Query(ctx, query, workflowID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to get approval steps")
	}
	defer rows.Close()

	return r.scanRows(rows)
}

// GetCurrentStep returns the step at the given step_number within a workflow.
func (r *ApprovalStepsRepository) GetCurrentStep(ctx context.Context, workflowID string, stepNumber int) (*ApprovalWorkflowStep, error) {
	query := `
		SELECT id, workflow_id, invoice_id, entity_id,
		       step_number, required_role, is_required,
		       assigned_to, assigned_at,
		       delegated_to, delegated_at, delegated_reason,
		       status, acted_by, acted_at, action_notes, due_at,
		       created_at, updated_at
		FROM invoice_approval_steps
		WHERE workflow_id = $1 AND step_number = $2
	`

	step, err := r.scanStep(r.db.QueryRow(ctx, query, workflowID, stepNumber))
	if err == pgx.ErrNoRows {
		return nil, errors.NotFound("approval_step", workflowID)
	}
	return step, err
}

// GetPendingForUser returns all pending steps assigned to a specific user
// (including delegated steps) within an entity.
func (r *ApprovalStepsRepository) GetPendingForUser(ctx context.Context, entityID, userID string) ([]*ApprovalWorkflowStep, error) {
	query := `
		SELECT s.id, s.workflow_id, s.invoice_id, s.entity_id,
		       s.step_number, s.required_role, s.is_required,
		       s.assigned_to, s.assigned_at,
		       s.delegated_to, s.delegated_at, s.delegated_reason,
		       s.status, s.acted_by, s.acted_at, s.action_notes, s.due_at,
		       s.created_at, s.updated_at
		FROM invoice_approval_steps s
		JOIN invoice_approval_workflows w ON w.id = s.workflow_id
		WHERE s.entity_id = $1
		  AND s.status = 'pending'
		  AND w.status = 'in_progress'
		  AND (s.assigned_to = $2 OR s.delegated_to = $2)
		ORDER BY s.due_at ASC NULLS LAST, s.created_at ASC
	`

	rows, err := r.db.Query(ctx, query, entityID, userID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to get pending approvals")
	}
	defer rows.Close()

	return r.scanRows(rows)
}

// UpdateStepAction records the outcome of an approval action (approve / reject / skip).
func (r *ApprovalStepsRepository) UpdateStepAction(
	ctx context.Context,
	id, status, actedBy string,
	notes *string,
) error {
	query := `
		UPDATE invoice_approval_steps
		SET status       = $2::approval_step_status,
		    acted_by     = $3,
		    acted_at     = NOW(),
		    action_notes = $4,
		    updated_at   = NOW()
		WHERE id = $1
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, status, actedBy, notes).Scan(&returnedID)
	if err == pgx.ErrNoRows {
		return errors.NotFound("approval_step", id)
	}
	return err
}

// AssignStep assigns a specific user to a step.
func (r *ApprovalStepsRepository) AssignStep(ctx context.Context, id, userID string) error {
	query := `
		UPDATE invoice_approval_steps
		SET assigned_to = $2,
		    assigned_at = NOW(),
		    updated_at  = NOW()
		WHERE id = $1
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, userID).Scan(&returnedID)
	if err == pgx.ErrNoRows {
		return errors.NotFound("approval_step", id)
	}
	return err
}

// DelegateStep delegates a step from the assigned user to another user.
func (r *ApprovalStepsRepository) DelegateStep(ctx context.Context, id, delegatedTo, reason string) error {
	query := `
		UPDATE invoice_approval_steps
		SET status           = 'delegated'::approval_step_status,
		    delegated_to     = $2,
		    delegated_at     = NOW(),
		    delegated_reason = $3,
		    updated_at       = NOW()
		WHERE id = $1
		  AND status = 'pending'
		RETURNING id
	`

	var returnedID string
	err := r.db.QueryRow(ctx, query, id, delegatedTo, reason).Scan(&returnedID)
	if err == pgx.ErrNoRows {
		return errors.New(errors.ErrCodeConflict, "step not found or not in pending status")
	}
	return err
}

// RecallSteps marks all pending steps in a workflow as recalled.
func (r *ApprovalStepsRepository) RecallSteps(ctx context.Context, workflowID string) error {
	query := `
		UPDATE invoice_approval_steps
		SET status     = 'recalled'::approval_step_status,
		    updated_at = NOW()
		WHERE workflow_id = $1
		  AND status = 'pending'
	`

	_, err := r.db.Exec(ctx, query, workflowID)
	return err
}

// ── scan helpers ──────────────────────────────────────────────────────────────

type stepScanner interface {
	Scan(dest ...any) error
}

func (r *ApprovalStepsRepository) scanStep(row stepScanner) (*ApprovalWorkflowStep, error) {
	s := &ApprovalWorkflowStep{}
	err := row.Scan(
		&s.ID,
		&s.WorkflowID,
		&s.InvoiceID,
		&s.EntityID,
		&s.StepNumber,
		&s.RequiredRole,
		&s.IsRequired,
		&s.AssignedTo,
		&s.AssignedAt,
		&s.DelegatedTo,
		&s.DelegatedAt,
		&s.DelegatedReason,
		&s.Status,
		&s.ActedBy,
		&s.ActedAt,
		&s.ActionNotes,
		&s.DueAt,
		&s.CreatedAt,
		&s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *ApprovalStepsRepository) scanRows(rows pgx.Rows) ([]*ApprovalWorkflowStep, error) {
	var steps []*ApprovalWorkflowStep
	for rows.Next() {
		s := &ApprovalWorkflowStep{}
		err := rows.Scan(
			&s.ID,
			&s.WorkflowID,
			&s.InvoiceID,
			&s.EntityID,
			&s.StepNumber,
			&s.RequiredRole,
			&s.IsRequired,
			&s.AssignedTo,
			&s.AssignedAt,
			&s.DelegatedTo,
			&s.DelegatedAt,
			&s.DelegatedReason,
			&s.Status,
			&s.ActedBy,
			&s.ActedAt,
			&s.ActionNotes,
			&s.DueAt,
			&s.CreatedAt,
			&s.UpdatedAt,
		)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to scan approval step")
		}
		steps = append(steps, s)
	}
	return steps, nil
}
