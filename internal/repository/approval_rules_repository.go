package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/pesio-ai/be-lib-common/database"
	"github.com/pesio-ai/be-lib-common/errors"
)

// ApprovalRulesRepository handles CRUD for invoice_approval_rules.
type ApprovalRulesRepository struct {
	db *database.DB
}

// NewApprovalRulesRepository creates a new ApprovalRulesRepository.
func NewApprovalRulesRepository(db *database.DB) *ApprovalRulesRepository {
	return &ApprovalRulesRepository{db: db}
}

// Create inserts a new approval rule.
func (r *ApprovalRulesRepository) Create(ctx context.Context, rule *ApprovalRule) error {
	stepsJSON, err := json.Marshal(rule.ApprovalSteps)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to marshal approval steps")
	}

	query := `
		INSERT INTO invoice_approval_rules
		    (entity_id, rule_name, rule_type, is_active,
		     min_amount, max_amount, vendor_id, department,
		     approval_steps, priority)
		VALUES ($1, $2, $3::approval_rule_type, $4,
		        $5, $6, $7, $8,
		        $9, $10)
		RETURNING id, created_at, updated_at
	`

	return r.db.QueryRow(ctx, query,
		rule.EntityID,
		rule.RuleName,
		rule.RuleType,
		rule.IsActive,
		rule.MinAmount,
		rule.MaxAmount,
		rule.VendorID,
		rule.Department,
		stepsJSON,
		rule.Priority,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
}

// GetByID retrieves a rule by primary key.
func (r *ApprovalRulesRepository) GetByID(ctx context.Context, id, entityID string) (*ApprovalRule, error) {
	query := `
		SELECT id, entity_id, rule_name, rule_type, is_active,
		       min_amount, max_amount, vendor_id, department,
		       approval_steps, priority, created_at, updated_at
		FROM invoice_approval_rules
		WHERE id = $1 AND entity_id = $2
	`

	rule, err := r.scanRule(r.db.QueryRow(ctx, query, id, entityID))
	if err == pgx.ErrNoRows {
		return nil, errors.NotFound("approval_rule", id)
	}
	return rule, err
}

// List returns all rules for an entity, optionally filtered to active only.
func (r *ApprovalRulesRepository) List(ctx context.Context, entityID string, activeOnly bool) ([]*ApprovalRule, error) {
	query := `
		SELECT id, entity_id, rule_name, rule_type, is_active,
		       min_amount, max_amount, vendor_id, department,
		       approval_steps, priority, created_at, updated_at
		FROM invoice_approval_rules
		WHERE entity_id = $1
	`
	if activeOnly {
		query += " AND is_active = TRUE"
	}
	query += " ORDER BY priority ASC, rule_name ASC"

	rows, err := r.db.Query(ctx, query, entityID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to list approval rules")
	}
	defer rows.Close()

	var rules []*ApprovalRule
	for rows.Next() {
		rule, err := r.scanRuleRow(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

// FindMatchingRule evaluates active rules for an entity in priority order and
// returns the first rule whose criteria match the invoice attributes.
// Returns nil (no error) when no rule matches.
func (r *ApprovalRulesRepository) FindMatchingRule(
	ctx context.Context,
	entityID string,
	amount int64,
	vendorID *string,
	department *string,
) (*ApprovalRule, error) {
	// Load all active rules ordered by priority; evaluate in Go to keep SQL simple.
	rules, err := r.List(ctx, entityID, true)
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		if r.ruleMatches(rule, amount, vendorID, department) {
			return rule, nil
		}
	}
	return nil, nil
}

// ruleMatches returns true when the rule's criteria all match the invoice attributes.
func (r *ApprovalRulesRepository) ruleMatches(
	rule *ApprovalRule,
	amount int64,
	vendorID *string,
	department *string,
) bool {
	switch rule.RuleType {
	case "amount_based":
		if rule.MinAmount != nil && amount < *rule.MinAmount {
			return false
		}
		if rule.MaxAmount != nil && amount >= *rule.MaxAmount {
			return false
		}
		return true

	case "vendor_based":
		if rule.VendorID == nil || vendorID == nil {
			return false
		}
		return *rule.VendorID == *vendorID

	case "department_based":
		if rule.Department == nil || department == nil {
			return false
		}
		return *rule.Department == *department
	}
	return false
}

// Update persists changes to an existing rule.
func (r *ApprovalRulesRepository) Update(ctx context.Context, rule *ApprovalRule) error {
	stepsJSON, err := json.Marshal(rule.ApprovalSteps)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to marshal approval steps")
	}

	query := `
		UPDATE invoice_approval_rules
		SET rule_name      = $3,
		    rule_type      = $4::approval_rule_type,
		    is_active      = $5,
		    min_amount     = $6,
		    max_amount     = $7,
		    vendor_id      = $8,
		    department     = $9,
		    approval_steps = $10,
		    priority       = $11,
		    updated_at     = NOW()
		WHERE id = $1 AND entity_id = $2
		RETURNING updated_at
	`

	err = r.db.QueryRow(ctx, query,
		rule.ID,
		rule.EntityID,
		rule.RuleName,
		rule.RuleType,
		rule.IsActive,
		rule.MinAmount,
		rule.MaxAmount,
		rule.VendorID,
		rule.Department,
		stepsJSON,
		rule.Priority,
	).Scan(&rule.UpdatedAt)

	if err == pgx.ErrNoRows {
		return errors.NotFound("approval_rule", rule.ID)
	}
	return err
}

// Delete removes an approval rule. Only rules with no associated workflows can be deleted.
func (r *ApprovalRulesRepository) Delete(ctx context.Context, id, entityID string) error {
	query := `
		DELETE FROM invoice_approval_rules
		WHERE id = $1 AND entity_id = $2
	`

	tag, err := r.db.Exec(ctx, query, id, entityID)
	if err != nil {
		return errors.Wrap(err, errors.ErrCodeInternal, "failed to delete approval rule")
	}
	if tag.RowsAffected() == 0 {
		return errors.NotFound("approval_rule", id)
	}
	return nil
}

// ── scan helpers ─────────────────────────────────────────────────────────────

type ruleScanner interface {
	Scan(dest ...any) error
}

func (r *ApprovalRulesRepository) scanRule(row ruleScanner) (*ApprovalRule, error) {
	rule := &ApprovalRule{}
	var stepsJSON []byte

	err := row.Scan(
		&rule.ID,
		&rule.EntityID,
		&rule.RuleName,
		&rule.RuleType,
		&rule.IsActive,
		&rule.MinAmount,
		&rule.MaxAmount,
		&rule.VendorID,
		&rule.Department,
		&stepsJSON,
		&rule.Priority,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(stepsJSON, &rule.ApprovalSteps); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to unmarshal approval steps")
	}
	return rule, nil
}

func (r *ApprovalRulesRepository) scanRuleRow(rows pgx.Rows) (*ApprovalRule, error) {
	rule := &ApprovalRule{}
	var stepsJSON []byte

	err := rows.Scan(
		&rule.ID,
		&rule.EntityID,
		&rule.RuleName,
		&rule.RuleType,
		&rule.IsActive,
		&rule.MinAmount,
		&rule.MaxAmount,
		&rule.VendorID,
		&rule.Department,
		&stepsJSON,
		&rule.Priority,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to scan approval rule")
	}
	if err := json.Unmarshal(stepsJSON, &rule.ApprovalSteps); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeInternal, "failed to unmarshal approval steps")
	}
	return rule, nil
}
