package service

import (
	"context"
	"fmt"
	"time"

	"github.com/pesio-ai/be-ap-invoices/internal/repository"
	"github.com/pesio-ai/be-lib-common/errors"
	"github.com/pesio-ai/be-lib-common/logger"
)

// IdentityClientInterface resolves user information from the identity service.
// The real implementation is wired in Task #17.
type IdentityClientInterface interface {
	// GetUsersWithRole returns user IDs that have the given role for an entity.
	GetUsersWithRole(ctx context.Context, entityID, role string) ([]string, error)
	// GetUserRoles returns the roles a specific user holds for an entity.
	GetUserRoles(ctx context.Context, entityID, userID string) ([]string, error)
}

// defaultSingleStepRole is used when no rule matches the invoice.
const defaultSingleStepRole = "FINANCE_MANAGER"

// ApprovalRoutingService orchestrates the multi-level approval workflow.
type ApprovalRoutingService struct {
	rulesRepo      *repository.ApprovalRulesRepository
	workflowRepo   *repository.ApprovalWorkflowRepository
	stepsRepo      *repository.ApprovalStepsRepository
	auditRepo      *repository.ApprovalAuditRepository
	invoiceRepo    *repository.InvoiceRepository
	identityClient IdentityClientInterface
	log            *logger.Logger
}

// NewApprovalRoutingService creates a new ApprovalRoutingService.
func NewApprovalRoutingService(
	rulesRepo *repository.ApprovalRulesRepository,
	workflowRepo *repository.ApprovalWorkflowRepository,
	stepsRepo *repository.ApprovalStepsRepository,
	auditRepo *repository.ApprovalAuditRepository,
	invoiceRepo *repository.InvoiceRepository,
	identityClient IdentityClientInterface,
	log *logger.Logger,
) *ApprovalRoutingService {
	return &ApprovalRoutingService{
		rulesRepo:      rulesRepo,
		workflowRepo:   workflowRepo,
		stepsRepo:      stepsRepo,
		auditRepo:      auditRepo,
		invoiceRepo:    invoiceRepo,
		identityClient: identityClient,
		log:            log,
	}
}

// ── Workflow creation ─────────────────────────────────────────────────────────

// CreateApprovalWorkflow evaluates rules, builds the workflow instance and all
// approval steps. Returns the created workflow and its steps.
func (s *ApprovalRoutingService) CreateApprovalWorkflow(
	ctx context.Context,
	invoice *repository.Invoice,
	submittedBy string,
) (*repository.ApprovalWorkflow, []*repository.ApprovalWorkflowStep, error) {
	// Find department from first invoice line (dimension_2), if present
	var department *string
	if len(invoice.Lines) > 0 && invoice.Lines[0].Dimension2 != nil {
		department = invoice.Lines[0].Dimension2
	}

	// Resolve matching rule
	rule, err := s.rulesRepo.FindMatchingRule(ctx, invoice.EntityID, invoice.TotalAmount, &invoice.VendorID, department)
	if err != nil {
		return nil, nil, err
	}

	// Build step definitions from rule (or fall back to a single default step)
	stepDefs := s.resolveStepDefs(rule)

	// Assign approvers from identity service
	steps, err := s.buildSteps(ctx, invoice.EntityID, stepDefs)
	if err != nil {
		return nil, nil, err
	}

	// Build workflow record
	var ruleID *string
	if rule != nil {
		ruleID = &rule.ID
	}

	wf := &repository.ApprovalWorkflow{
		InvoiceID:   invoice.ID,
		EntityID:    invoice.EntityID,
		RuleID:      ruleID,
		Status:      "in_progress",
		TotalSteps:  len(steps),
		CurrentStep: 1,
		SubmittedBy: submittedBy,
	}

	if err := s.workflowRepo.Create(ctx, wf, steps); err != nil {
		return nil, nil, err
	}

	s.log.Info().
		Str("invoice_id", invoice.ID).
		Str("workflow_id", wf.ID).
		Int("total_steps", wf.TotalSteps).
		Msg("Approval workflow created")

	return wf, steps, nil
}

// resolveStepDefs returns the ordered step definitions from a rule, or a
// single default step when no rule matched.
func (s *ApprovalRoutingService) resolveStepDefs(rule *repository.ApprovalRule) []repository.ApprovalRuleStep {
	if rule != nil && len(rule.ApprovalSteps) > 0 {
		return rule.ApprovalSteps
	}
	return []repository.ApprovalRuleStep{
		{Step: 1, Role: defaultSingleStepRole, Required: true},
	}
}

// buildSteps converts rule step definitions into ApprovalWorkflowStep records,
// pre-assigning the first available user for each role via the identity service.
func (s *ApprovalRoutingService) buildSteps(
	ctx context.Context,
	entityID string,
	defs []repository.ApprovalRuleStep,
) ([]*repository.ApprovalWorkflowStep, error) {
	steps := make([]*repository.ApprovalWorkflowStep, 0, len(defs))

	for _, def := range defs {
		step := &repository.ApprovalWorkflowStep{
			StepNumber:   def.Step,
			RequiredRole: def.Role,
			IsRequired:   def.Required,
			Status:       "pending",
		}

		// Attempt to assign first available approver for this role
		users, err := s.identityClient.GetUsersWithRole(ctx, entityID, def.Role)
		if err != nil {
			s.log.Warn().Err(err).Str("role", def.Role).Msg("Could not fetch users for role; step will be unassigned")
		} else if len(users) > 0 {
			now := time.Now()
			step.AssignedTo = &users[0]
			step.AssignedAt = &now
		}

		steps = append(steps, step)
	}

	return steps, nil
}

// ── Approve ───────────────────────────────────────────────────────────────────

// ApproveStep records approval for a workflow step. Returns true when the
// entire workflow is now complete (all required steps approved).
func (s *ApprovalRoutingService) ApproveStep(
	ctx context.Context,
	invoiceID, workflowID string,
	stepNumber int,
	actedBy string,
	notes *string,
) (workflowComplete bool, err error) {
	wf, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return false, err
	}
	if wf.Status != "in_progress" {
		return false, errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("workflow is not in_progress (status: %s)", wf.Status))
	}

	step, err := s.stepsRepo.GetCurrentStep(ctx, workflowID, stepNumber)
	if err != nil {
		return false, err
	}
	if step.Status != "pending" {
		return false, errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("step %d is not pending (status: %s)", stepNumber, step.Status))
	}
	if err := s.assertCanAct(step, actedBy); err != nil {
		return false, err
	}

	// Persist the approval action
	if err := s.stepsRepo.UpdateStepAction(ctx, step.ID, "approved", actedBy, notes); err != nil {
		return false, err
	}

	// Check whether there is a next step
	if stepNumber < wf.TotalSteps {
		// Advance workflow to next step
		if err := s.workflowRepo.AdvanceStep(ctx, workflowID, stepNumber+1); err != nil {
			return false, err
		}
		workflowComplete = false
	} else {
		// All steps done — complete the workflow and approve the invoice
		now := time.Now()
		if err := s.workflowRepo.UpdateStatus(ctx, workflowID, "approved", &now); err != nil {
			return false, err
		}
		if err := s.invoiceRepo.Approve(ctx, invoiceID, wf.EntityID, &actedBy, notes); err != nil {
			return false, err
		}
		workflowComplete = true
	}

	// Audit log
	invoice, _ := s.invoiceRepo.GetByID(ctx, invoiceID, wf.EntityID)
	statusBefore := "pending_approval"
	statusAfter := "pending_approval"
	if workflowComplete {
		statusAfter = "approved"
	}
	s.appendAudit(ctx, &repository.ApprovalAuditEntry{
		InvoiceID:           invoiceID,
		WorkflowID:          &workflowID,
		StepID:              &step.ID,
		EntityID:            wf.EntityID,
		Action:              "approved",
		PerformedBy:         actedBy,
		InvoiceStatusBefore: &statusBefore,
		InvoiceStatusAfter:  &statusAfter,
		Metadata: map[string]interface{}{
			"step_number":    stepNumber,
			"invoice_number": invoiceIfNotNil(invoice),
		},
	})

	return workflowComplete, nil
}

// ── Reject ────────────────────────────────────────────────────────────────────

// RejectWorkflow rejects the invoice at the given step, returning it to draft.
func (s *ApprovalRoutingService) RejectWorkflow(
	ctx context.Context,
	invoiceID, workflowID string,
	stepNumber int,
	actedBy, reason string,
) error {
	wf, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return err
	}
	if wf.Status != "in_progress" {
		return errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("workflow is not in_progress (status: %s)", wf.Status))
	}

	step, err := s.stepsRepo.GetCurrentStep(ctx, workflowID, stepNumber)
	if err != nil {
		return err
	}
	if step.Status != "pending" {
		return errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("step %d is not pending", stepNumber))
	}
	if err := s.assertCanAct(step, actedBy); err != nil {
		return err
	}
	if reason == "" {
		return errors.InvalidInput("reason", "rejection reason is required")
	}

	notesPtr := &reason
	if err := s.stepsRepo.UpdateStepAction(ctx, step.ID, "rejected", actedBy, notesPtr); err != nil {
		return err
	}

	now := time.Now()
	if err := s.workflowRepo.UpdateStatus(ctx, workflowID, "rejected", &now); err != nil {
		return err
	}

	// Return invoice to draft
	if err := s.invoiceRepo.UpdateStatus(ctx, invoiceID, wf.EntityID, "draft", &actedBy); err != nil {
		return err
	}

	statusBefore := "pending_approval"
	statusAfter := "draft"
	s.appendAudit(ctx, &repository.ApprovalAuditEntry{
		InvoiceID:           invoiceID,
		WorkflowID:          &workflowID,
		StepID:              &step.ID,
		EntityID:            wf.EntityID,
		Action:              "rejected",
		PerformedBy:         actedBy,
		InvoiceStatusBefore: &statusBefore,
		InvoiceStatusAfter:  &statusAfter,
		Metadata:            map[string]interface{}{"reason": reason, "step_number": stepNumber},
	})

	return nil
}

// ── Recall ────────────────────────────────────────────────────────────────────

// RecallWorkflow lets the original submitter cancel a pending workflow.
func (s *ApprovalRoutingService) RecallWorkflow(
	ctx context.Context,
	invoiceID, workflowID, recalledBy string,
) error {
	wf, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return err
	}

	if wf.SubmittedBy != recalledBy {
		return errors.New(errors.ErrCodeUnauthorized, "only the submitter can recall the workflow")
	}
	if wf.Status != "in_progress" && wf.Status != "pending" {
		return errors.New(errors.ErrCodeConflict,
			fmt.Sprintf("workflow cannot be recalled from status '%s'", wf.Status))
	}

	// Mark all pending steps as recalled
	if err := s.stepsRepo.RecallSteps(ctx, workflowID); err != nil {
		return err
	}

	now := time.Now()
	if err := s.workflowRepo.UpdateStatus(ctx, workflowID, "recalled", &now); err != nil {
		return err
	}

	// Return invoice to draft
	if err := s.invoiceRepo.UpdateStatus(ctx, invoiceID, wf.EntityID, "draft", &recalledBy); err != nil {
		return err
	}

	statusBefore := "pending_approval"
	statusAfter := "draft"
	s.appendAudit(ctx, &repository.ApprovalAuditEntry{
		InvoiceID:           invoiceID,
		WorkflowID:          &workflowID,
		EntityID:            wf.EntityID,
		Action:              "recalled",
		PerformedBy:         recalledBy,
		InvoiceStatusBefore: &statusBefore,
		InvoiceStatusAfter:  &statusAfter,
	})

	return nil
}

// ── Delegation ────────────────────────────────────────────────────────────────

// DelegateStep lets the assigned approver delegate their step to another user.
func (s *ApprovalRoutingService) DelegateStep(
	ctx context.Context,
	workflowID string,
	stepNumber int,
	delegatedBy, delegatedTo, reason string,
) error {
	step, err := s.stepsRepo.GetCurrentStep(ctx, workflowID, stepNumber)
	if err != nil {
		return err
	}
	if err := s.assertCanAct(step, delegatedBy); err != nil {
		return err
	}
	if reason == "" {
		return errors.InvalidInput("reason", "delegation reason is required")
	}

	if err := s.stepsRepo.DelegateStep(ctx, step.ID, delegatedTo, reason); err != nil {
		return err
	}

	wf, _ := s.workflowRepo.GetByID(ctx, workflowID)
	entityID := ""
	if wf != nil {
		entityID = wf.EntityID
	}

	s.appendAudit(ctx, &repository.ApprovalAuditEntry{
		InvoiceID:   step.InvoiceID,
		WorkflowID:  &workflowID,
		StepID:      &step.ID,
		EntityID:    entityID,
		Action:      "delegated",
		PerformedBy: delegatedBy,
		Metadata: map[string]interface{}{
			"delegated_to": delegatedTo,
			"reason":       reason,
			"step_number":  stepNumber,
		},
	})

	return nil
}

// ── Query helpers ─────────────────────────────────────────────────────────────

// GetPendingApprovals returns all steps currently awaiting action from a user.
func (s *ApprovalRoutingService) GetPendingApprovals(
	ctx context.Context,
	entityID, userID string,
) ([]*repository.ApprovalWorkflowStep, error) {
	return s.stepsRepo.GetPendingForUser(ctx, entityID, userID)
}

// GetApprovalHistory returns the full audit trail for an invoice.
func (s *ApprovalRoutingService) GetApprovalHistory(
	ctx context.Context,
	invoiceID, entityID string,
) ([]*repository.ApprovalAuditEntry, error) {
	return s.auditRepo.GetByInvoiceID(ctx, invoiceID, entityID)
}

// GetWorkflowSteps returns all steps for an active workflow on an invoice.
func (s *ApprovalRoutingService) GetWorkflowSteps(
	ctx context.Context,
	invoiceID string,
) ([]*repository.ApprovalWorkflowStep, error) {
	wf, err := s.workflowRepo.GetActiveByInvoiceID(ctx, invoiceID)
	if err != nil {
		return nil, err
	}
	if wf == nil {
		return nil, errors.NotFound("approval_workflow", invoiceID)
	}
	return s.stepsRepo.GetByWorkflowID(ctx, wf.ID)
}

// GetActiveWorkflow returns the active workflow for an invoice, or nil.
func (s *ApprovalRoutingService) GetActiveWorkflow(
	ctx context.Context,
	invoiceID string,
) (*repository.ApprovalWorkflow, error) {
	return s.workflowRepo.GetActiveByInvoiceID(ctx, invoiceID)
}

// ── Authorization helper ──────────────────────────────────────────────────────

// assertCanAct checks that userID is the assigned or delegated approver for a step.
func (s *ApprovalRoutingService) assertCanAct(step *repository.ApprovalWorkflowStep, userID string) error {
	if step.AssignedTo != nil && *step.AssignedTo == userID {
		return nil
	}
	if step.DelegatedTo != nil && *step.DelegatedTo == userID {
		return nil
	}
	// Unassigned steps can be acted on by anyone (no assignment yet)
	if step.AssignedTo == nil && step.DelegatedTo == nil {
		return nil
	}
	return errors.New(errors.ErrCodeUnauthorized,
		"user is not authorized to act on this approval step")
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// appendAudit writes an audit entry and logs a warning on failure (never returns error).
func (s *ApprovalRoutingService) appendAudit(ctx context.Context, entry *repository.ApprovalAuditEntry) {
	if err := s.auditRepo.Append(ctx, entry); err != nil {
		s.log.Warn().Err(err).
			Str("invoice_id", entry.InvoiceID).
			Str("action", entry.Action).
			Msg("Failed to write audit log entry")
	}
}

func invoiceIfNotNil(inv *repository.Invoice) string {
	if inv == nil {
		return ""
	}
	return inv.InvoiceNumber
}
