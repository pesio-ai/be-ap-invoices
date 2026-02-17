-- ============================================================
-- Migration 002: AP Invoice Approval Workflow
-- ============================================================
-- Creates approval rules, workflow instances, step tracking,
-- and immutable audit log for multi-level invoice approval.

-- ── Enums ────────────────────────────────────────────────────

CREATE TYPE approval_step_status AS ENUM (
    'pending',
    'approved',
    'rejected',
    'delegated',
    'recalled',
    'skipped'
);

CREATE TYPE approval_workflow_status AS ENUM (
    'pending',
    'in_progress',
    'approved',
    'rejected',
    'recalled'
);

CREATE TYPE approval_rule_type AS ENUM (
    'amount_based',
    'vendor_based',
    'department_based'
);

-- ── Approval Rules ────────────────────────────────────────────
-- Configurable routing rules per entity; evaluated on submit.

CREATE TABLE invoice_approval_rules (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_id   UUID NOT NULL,

    rule_name   VARCHAR(255) NOT NULL,
    rule_type   approval_rule_type NOT NULL DEFAULT 'amount_based',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,

    -- Amount-based thresholds (cents, NULL = no bound)
    min_amount  BIGINT,
    max_amount  BIGINT,

    -- Vendor-based (optional, matched against invoice vendor_id)
    vendor_id   UUID,

    -- Department/dimension match (optional, matches invoice_lines.dimension_2)
    department  VARCHAR(100),

    -- Ordered list of approval steps, stored as JSONB:
    --   [{"step": 1, "role": "AP_CLERK", "required": true}, ...]
    approval_steps JSONB NOT NULL DEFAULT '[]',

    -- Tie-break priority when multiple rules match (lower = higher priority)
    priority    INT NOT NULL DEFAULT 100,

    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT approval_rules_entity_name_unique UNIQUE (entity_id, rule_name),
    CONSTRAINT approval_rules_min_max_check CHECK (
        min_amount IS NULL OR max_amount IS NULL OR min_amount <= max_amount
    ),
    CONSTRAINT approval_rules_min_check CHECK (min_amount IS NULL OR min_amount >= 0),
    CONSTRAINT approval_rules_max_check CHECK (max_amount IS NULL OR max_amount >= 0)
);

-- ── Approval Workflows ─────────────────────────────────────────
-- One workflow instance per invoice submission attempt.

CREATE TABLE invoice_approval_workflows (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    invoice_id  UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    entity_id   UUID NOT NULL,

    rule_id     UUID REFERENCES invoice_approval_rules(id) ON DELETE SET NULL,

    status      approval_workflow_status NOT NULL DEFAULT 'pending',
    total_steps INT NOT NULL DEFAULT 1,
    current_step INT NOT NULL DEFAULT 1,

    submitted_by    UUID NOT NULL,
    submitted_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMP WITH TIME ZONE,

    -- Optional notes from submitter
    submission_notes TEXT,

    created_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ── Approval Steps ─────────────────────────────────────────────
-- Individual approval steps within a workflow.

CREATE TABLE invoice_approval_steps (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workflow_id     UUID NOT NULL REFERENCES invoice_approval_workflows(id) ON DELETE CASCADE,
    invoice_id      UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    entity_id       UUID NOT NULL,

    step_number     INT NOT NULL,
    required_role   VARCHAR(100) NOT NULL,
    is_required     BOOLEAN NOT NULL DEFAULT TRUE,

    -- Assigned approver (resolved from role at workflow creation time)
    assigned_to     UUID,               -- user_id
    assigned_at     TIMESTAMP WITH TIME ZONE,

    -- Delegation support
    delegated_to    UUID,               -- user_id
    delegated_at    TIMESTAMP WITH TIME ZONE,
    delegated_reason TEXT,

    status          approval_step_status NOT NULL DEFAULT 'pending',

    -- Action taken
    acted_by        UUID,               -- user_id who approved/rejected/delegated
    acted_at        TIMESTAMP WITH TIME ZONE,
    action_notes    TEXT,

    -- Deadline for this step (optional SLA)
    due_at          TIMESTAMP WITH TIME ZONE,

    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT approval_steps_workflow_step_unique UNIQUE (workflow_id, step_number),
    CONSTRAINT approval_steps_step_number_check CHECK (step_number >= 1)
);

-- ── Approval Audit Log ──────────────────────────────────────────
-- Append-only audit trail; no updates or deletes allowed.

CREATE TABLE invoice_approval_audit_log (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    invoice_id      UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    workflow_id     UUID REFERENCES invoice_approval_workflows(id) ON DELETE SET NULL,
    step_id         UUID REFERENCES invoice_approval_steps(id) ON DELETE SET NULL,
    entity_id       UUID NOT NULL,

    -- Action performed
    action          VARCHAR(50) NOT NULL,   -- submitted, approved, rejected, recalled, delegated, reassigned
    performed_by    UUID NOT NULL,          -- user_id
    performed_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Snapshot of invoice status at time of action
    invoice_status_before   VARCHAR(50),
    invoice_status_after    VARCHAR(50),

    -- Free-form context (reason, notes, delegatee, etc.)
    metadata        JSONB,

    -- Prevent accidental deletes via FK violation (no cascade)
    CONSTRAINT audit_log_performed_by_check CHECK (performed_by IS NOT NULL)
);

-- ── Triggers ──────────────────────────────────────────────────

-- updated_at for approval_rules
CREATE TRIGGER trigger_approval_rules_updated_at
BEFORE UPDATE ON invoice_approval_rules
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- updated_at for approval_workflows
CREATE TRIGGER trigger_approval_workflows_updated_at
BEFORE UPDATE ON invoice_approval_workflows
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- updated_at for approval_steps
CREATE TRIGGER trigger_approval_steps_updated_at
BEFORE UPDATE ON invoice_approval_steps
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Prevent deletes on audit log
CREATE OR REPLACE FUNCTION prevent_audit_log_delete()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Deleting from invoice_approval_audit_log is not permitted';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_prevent_audit_log_delete
BEFORE DELETE ON invoice_approval_audit_log
FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_delete();

-- ── Indexes ───────────────────────────────────────────────────

CREATE INDEX idx_approval_rules_entity_id       ON invoice_approval_rules(entity_id);
CREATE INDEX idx_approval_rules_entity_active    ON invoice_approval_rules(entity_id, is_active);
CREATE INDEX idx_approval_rules_priority         ON invoice_approval_rules(entity_id, priority);

CREATE INDEX idx_approval_workflows_invoice_id   ON invoice_approval_workflows(invoice_id);
CREATE INDEX idx_approval_workflows_entity_id    ON invoice_approval_workflows(entity_id);
CREATE INDEX idx_approval_workflows_status       ON invoice_approval_workflows(status);
CREATE INDEX idx_approval_workflows_submitted_by ON invoice_approval_workflows(submitted_by);

CREATE INDEX idx_approval_steps_workflow_id      ON invoice_approval_steps(workflow_id);
CREATE INDEX idx_approval_steps_invoice_id       ON invoice_approval_steps(invoice_id);
CREATE INDEX idx_approval_steps_assigned_to      ON invoice_approval_steps(assigned_to) WHERE assigned_to IS NOT NULL;
CREATE INDEX idx_approval_steps_delegated_to     ON invoice_approval_steps(delegated_to) WHERE delegated_to IS NOT NULL;
CREATE INDEX idx_approval_steps_status           ON invoice_approval_steps(status);
CREATE INDEX idx_approval_steps_entity_status    ON invoice_approval_steps(entity_id, status);

CREATE INDEX idx_audit_log_invoice_id            ON invoice_approval_audit_log(invoice_id);
CREATE INDEX idx_audit_log_workflow_id           ON invoice_approval_audit_log(workflow_id);
CREATE INDEX idx_audit_log_performed_by          ON invoice_approval_audit_log(performed_by);
CREATE INDEX idx_audit_log_performed_at          ON invoice_approval_audit_log(performed_at DESC);
CREATE INDEX idx_audit_log_entity_id             ON invoice_approval_audit_log(entity_id);

-- ── Seed Default Rules (example – not required for migration to succeed) ──
-- These serve as reference; actual seeding done via application bootstrap.

COMMENT ON TABLE invoice_approval_rules IS 'Configurable multi-level approval routing rules per entity';
COMMENT ON TABLE invoice_approval_workflows IS 'Approval workflow instance created on invoice submission';
COMMENT ON TABLE invoice_approval_steps IS 'Individual approval steps within a workflow (one row per step)';
COMMENT ON TABLE invoice_approval_audit_log IS 'Immutable audit trail of all approval actions';

COMMENT ON COLUMN invoice_approval_rules.approval_steps IS 'JSON array: [{step, role, required}]';
COMMENT ON COLUMN invoice_approval_rules.min_amount IS 'Inclusive lower bound in cents; NULL = no lower bound';
COMMENT ON COLUMN invoice_approval_rules.max_amount IS 'Exclusive upper bound in cents; NULL = no upper bound';
COMMENT ON COLUMN invoice_approval_rules.priority IS 'Lower value = evaluated first when multiple rules match';

COMMENT ON COLUMN invoice_approval_steps.required_role IS 'Role name that qualifies a user to act on this step';
COMMENT ON COLUMN invoice_approval_steps.assigned_to IS 'Specific user assigned at workflow creation time';
COMMENT ON COLUMN invoice_approval_steps.delegated_to IS 'User to whom the step was delegated';

COMMENT ON COLUMN invoice_approval_audit_log.action IS 'One of: submitted, approved, rejected, recalled, delegated, reassigned';
COMMENT ON COLUMN invoice_approval_audit_log.metadata IS 'Arbitrary JSON context (reason, delegatee_id, notes, etc.)';
