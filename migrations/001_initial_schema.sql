-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Invoice status enum
CREATE TYPE invoice_status AS ENUM ('draft', 'pending_approval', 'approved', 'posted', 'paid', 'cancelled');

-- Invoice type enum
CREATE TYPE invoice_type AS ENUM ('standard', 'credit_memo', 'debit_memo', 'prepayment', 'recurring');

-- Invoices (Header)
CREATE TABLE invoices (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_id UUID NOT NULL,
    vendor_id UUID NOT NULL,  -- Reference to vendors service (AP-1)
    invoice_number VARCHAR(100) NOT NULL,
    invoice_date DATE NOT NULL,
    due_date DATE NOT NULL,
    invoice_type invoice_type NOT NULL DEFAULT 'standard',
    status invoice_status NOT NULL DEFAULT 'draft',

    -- Payment Terms
    payment_terms VARCHAR(50) NOT NULL,
    discount_percent NUMERIC(5, 2),
    discount_due_date DATE,

    -- Amounts (in smallest currency unit - cents)
    currency VARCHAR(3) NOT NULL,
    subtotal BIGINT NOT NULL DEFAULT 0,
    tax_amount BIGINT NOT NULL DEFAULT 0,
    total_amount BIGINT NOT NULL DEFAULT 0,
    amount_paid BIGINT NOT NULL DEFAULT 0,
    amount_due BIGINT NOT NULL GENERATED ALWAYS AS (total_amount - amount_paid) STORED,

    -- Posting Information
    posted_to_gl BOOLEAN NOT NULL DEFAULT FALSE,
    gl_journal_id UUID,  -- Reference to journal entry in GL-2
    posted_date DATE,
    posted_by UUID,

    -- Approval Workflow
    approved_by UUID,
    approved_at TIMESTAMP WITH TIME ZONE,
    approval_notes TEXT,

    -- Payment Information
    payment_method VARCHAR(50),
    payment_reference VARCHAR(100),
    payment_date DATE,

    -- Document References
    po_number VARCHAR(100),  -- Purchase order number
    reference_number VARCHAR(100),  -- External reference
    description TEXT,
    notes TEXT,

    -- Attachments (references to document storage)
    attachment_urls TEXT[],

    -- Audit fields
    created_by UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_by UUID,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT invoices_entity_vendor_number_unique UNIQUE (entity_id, vendor_id, invoice_number),
    CONSTRAINT invoices_subtotal_check CHECK (subtotal >= 0),
    CONSTRAINT invoices_tax_amount_check CHECK (tax_amount >= 0),
    CONSTRAINT invoices_total_amount_check CHECK (total_amount >= 0),
    CONSTRAINT invoices_amount_paid_check CHECK (amount_paid >= 0),
    CONSTRAINT invoices_discount_check CHECK (discount_percent IS NULL OR (discount_percent >= 0 AND discount_percent <= 100))
);

-- Invoice Lines (Detail)
CREATE TABLE invoice_lines (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    line_number INT NOT NULL,

    -- Account Information (GL-1 integration)
    account_id UUID NOT NULL,  -- GL account to charge

    -- Line Details
    description TEXT NOT NULL,
    quantity NUMERIC(15, 4) NOT NULL DEFAULT 1,
    unit_price BIGINT NOT NULL,  -- Price per unit in smallest currency unit
    line_amount BIGINT NOT NULL,  -- quantity * unit_price

    -- Tax
    tax_code VARCHAR(20),
    tax_rate NUMERIC(5, 2),
    tax_amount BIGINT NOT NULL DEFAULT 0,

    -- Dimensions for reporting
    dimension_1 VARCHAR(50),  -- Cost center
    dimension_2 VARCHAR(50),  -- Department
    dimension_3 VARCHAR(50),  -- Project
    dimension_4 VARCHAR(50),  -- Custom dimension

    -- Item/Service Reference (optional)
    item_code VARCHAR(100),
    item_name VARCHAR(255),

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT invoice_lines_invoice_line_unique UNIQUE (invoice_id, line_number),
    CONSTRAINT invoice_lines_quantity_check CHECK (quantity > 0),
    CONSTRAINT invoice_lines_unit_price_check CHECK (unit_price >= 0),
    CONSTRAINT invoice_lines_line_amount_check CHECK (line_amount >= 0),
    CONSTRAINT invoice_lines_tax_amount_check CHECK (tax_amount >= 0),
    CONSTRAINT invoice_lines_tax_rate_check CHECK (tax_rate IS NULL OR (tax_rate >= 0 AND tax_rate <= 100))
);

-- Invoice Payments (Track partial payments)
CREATE TABLE invoice_payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    payment_date DATE NOT NULL,
    payment_amount BIGINT NOT NULL,
    payment_method VARCHAR(50),
    payment_reference VARCHAR(100),
    notes TEXT,
    created_by UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT invoice_payments_amount_check CHECK (payment_amount > 0)
);

-- Indexes for performance
CREATE INDEX idx_invoices_entity_id ON invoices(entity_id);
CREATE INDEX idx_invoices_vendor_id ON invoices(vendor_id);
CREATE INDEX idx_invoices_invoice_number ON invoices(invoice_number);
CREATE INDEX idx_invoices_invoice_date ON invoices(invoice_date);
CREATE INDEX idx_invoices_due_date ON invoices(due_date);
CREATE INDEX idx_invoices_status ON invoices(status);
CREATE INDEX idx_invoices_gl_journal_id ON invoices(gl_journal_id);
CREATE INDEX idx_invoice_lines_invoice_id ON invoice_lines(invoice_id);
CREATE INDEX idx_invoice_lines_account_id ON invoice_lines(account_id);
CREATE INDEX idx_invoice_payments_invoice_id ON invoice_payments(invoice_id);

-- Function to update invoice totals
CREATE OR REPLACE FUNCTION update_invoice_totals()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE invoices
    SET
        subtotal = (
            SELECT COALESCE(SUM(line_amount), 0)
            FROM invoice_lines
            WHERE invoice_id = NEW.invoice_id
        ),
        tax_amount = (
            SELECT COALESCE(SUM(tax_amount), 0)
            FROM invoice_lines
            WHERE invoice_id = NEW.invoice_id
        ),
        total_amount = (
            SELECT COALESCE(SUM(line_amount + tax_amount), 0)
            FROM invoice_lines
            WHERE invoice_id = NEW.invoice_id
        ),
        updated_at = NOW()
    WHERE id = NEW.invoice_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-update totals when lines change
CREATE TRIGGER trigger_update_invoice_totals
AFTER INSERT OR UPDATE OR DELETE ON invoice_lines
FOR EACH ROW
EXECUTE FUNCTION update_invoice_totals();

-- Function to update invoice payment status
CREATE OR REPLACE FUNCTION update_invoice_payment_status()
RETURNS TRIGGER AS $$
DECLARE
    v_total_paid BIGINT;
    v_total_amount BIGINT;
BEGIN
    -- Calculate total paid
    SELECT COALESCE(SUM(payment_amount), 0)
    INTO v_total_paid
    FROM invoice_payments
    WHERE invoice_id = NEW.invoice_id;

    -- Get invoice total
    SELECT total_amount
    INTO v_total_amount
    FROM invoices
    WHERE id = NEW.invoice_id;

    -- Update invoice
    UPDATE invoices
    SET
        amount_paid = v_total_paid,
        status = CASE
            WHEN v_total_paid >= v_total_amount THEN 'paid'::invoice_status
            ELSE status
        END,
        updated_at = NOW()
    WHERE id = NEW.invoice_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-update payment status
CREATE TRIGGER trigger_update_invoice_payment_status
AFTER INSERT ON invoice_payments
FOR EACH ROW
EXECUTE FUNCTION update_invoice_payment_status();

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers for updated_at
CREATE TRIGGER trigger_invoices_updated_at
BEFORE UPDATE ON invoices
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_invoice_lines_updated_at
BEFORE UPDATE ON invoice_lines
FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

-- Comments
COMMENT ON TABLE invoices IS 'Vendor invoices/bills for accounts payable';
COMMENT ON TABLE invoice_lines IS 'Invoice line items with GL account distribution';
COMMENT ON TABLE invoice_payments IS 'Invoice payment history (partial and full payments)';
COMMENT ON COLUMN invoices.subtotal IS 'Sum of all line amounts (before tax)';
COMMENT ON COLUMN invoices.tax_amount IS 'Sum of all line tax amounts';
COMMENT ON COLUMN invoices.total_amount IS 'Subtotal + tax_amount';
COMMENT ON COLUMN invoices.amount_due IS 'Computed column: total_amount - amount_paid';
COMMENT ON COLUMN invoices.posted_to_gl IS 'TRUE when journal entry created in GL-2';
COMMENT ON COLUMN invoices.gl_journal_id IS 'Reference to journal entry in GL-2 journals service';
COMMENT ON COLUMN invoice_lines.line_amount IS 'quantity * unit_price';
