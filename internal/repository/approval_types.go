package repository

import "time"

// ── Domain types for approval workflow ───────────────────────────────────────

// ApprovalRuleStep is one entry in an approval rule's approval_steps JSONB array.
type ApprovalRuleStep struct {
	Step     int    `json:"step"`
	Role     string `json:"role"`
	Required bool   `json:"required"`
}

// ApprovalRule is a configurable routing rule per entity.
type ApprovalRule struct {
	ID            string
	EntityID      string
	RuleName      string
	RuleType      string // amount_based | vendor_based | department_based
	IsActive      bool
	MinAmount     *int64  // cents; nil = no lower bound
	MaxAmount     *int64  // cents; nil = no upper bound
	VendorID      *string // optional vendor match
	Department    *string // optional dimension_2 match
	ApprovalSteps []ApprovalRuleStep
	Priority      int // lower = evaluated first
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ApprovalWorkflow is a workflow instance created on invoice submission.
type ApprovalWorkflow struct {
	ID              string
	InvoiceID       string
	EntityID        string
	RuleID          *string
	Status          string // pending | in_progress | approved | rejected | recalled
	TotalSteps      int
	CurrentStep     int
	SubmittedBy     string
	SubmittedAt     time.Time
	CompletedAt     *time.Time
	SubmissionNotes *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ApprovalWorkflowStep is a single approval step within a workflow.
type ApprovalWorkflowStep struct {
	ID              string
	WorkflowID      string
	InvoiceID       string
	EntityID        string
	StepNumber      int
	RequiredRole    string
	IsRequired      bool
	AssignedTo      *string
	AssignedAt      *time.Time
	DelegatedTo     *string
	DelegatedAt     *time.Time
	DelegatedReason *string
	Status          string // pending | approved | rejected | delegated | recalled | skipped
	ActedBy         *string
	ActedAt         *time.Time
	ActionNotes     *string
	DueAt           *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ApprovalAuditEntry is one immutable record in the audit log.
type ApprovalAuditEntry struct {
	ID                  string
	InvoiceID           string
	WorkflowID          *string
	StepID              *string
	EntityID            string
	Action              string // submitted | approved | rejected | recalled | delegated | reassigned
	PerformedBy         string
	PerformedAt         time.Time
	InvoiceStatusBefore *string
	InvoiceStatusAfter  *string
	Metadata            map[string]interface{} // arbitrary JSON context
}
