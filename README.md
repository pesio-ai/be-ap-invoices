# be-invoices-service (AP-2)

Invoice Management Service for Pesio Finance ERP - Manages vendor invoices, approval workflow, and posting to general ledger.

## Overview

This service implements invoice management functionality as defined in FPRD AP-2. It provides:

- **Invoice Management**: Create, read, update, delete invoices with line items
- **Approval Workflow**: Draft → Pending Approval → Approved → Posted → Paid
- **GL Posting**: Integration with GL-2 to post invoices to general ledger
- **Payment Tracking**: Record partial and full payments
- **Multi-dimensional Accounting**: Cost center, department, project tracking
- **Tax Calculation**: Line-level tax support
- **Vendor Integration**: Validates vendors via AP-1

## Features

### Invoice Types
- **Standard**: Regular vendor invoices
- **Credit Memo**: Vendor credits/refunds
- **Debit Memo**: Vendor debits/adjustments
- **Prepayment**: Advance payments to vendors
- **Recurring**: Recurring invoices (utilities, subscriptions)

### Invoice Status Workflow
- **Draft**: Invoice created, can be edited/deleted
- **Pending Approval**: Submitted for approval, awaiting reviewer
- **Approved**: Approved for posting, ready for GL
- **Posted**: Posted to GL, journal entry created, immutable
- **Paid**: Fully paid
- **Cancelled**: Cancelled invoice

### Business Rules
- Invoice numbers must be unique per vendor per entity
- Draft invoices can be edited or deleted
- Invoices must be approved before posting
- Posted invoices are immutable (create credit memo to reverse)
- Payments can only be recorded for posted invoices
- Total amount = subtotal + tax_amount
- Amount due = total_amount - amount_paid
- All amounts stored in smallest currency unit (cents)

## API Endpoints

### Health Check
```
GET /health
```

### Invoice Operations

#### List Invoices
```
GET /api/v1/invoices?entity_id={uuid}&vendor_id={uuid}&status={status}&from_date={date}&to_date={date}&page={int}&page_size={int}
```

#### Get Invoice by ID
```
GET /api/v1/invoices/get?id={uuid}&entity_id={uuid}
```

#### Create Invoice
```
POST /api/v1/invoices
Content-Type: application/json

{
  "entity_id": "uuid",
  "vendor_id": "uuid",
  "invoice_number": "INV-2024-001",
  "invoice_date": "2024-01-15",
  "due_date": "2024-02-14",
  "invoice_type": "standard",
  "payment_terms": "NET30",
  "currency": "USD",
  "lines": [
    {
      "line_number": 1,
      "account_id": "uuid",
      "description": "Office supplies",
      "quantity": 10.0,
      "unit_price": 2500,
      "line_amount": 25000,
      "tax_rate": 8.5,
      "tax_amount": 2125,
      "dimension_1": "CC001",
      "dimension_2": "ADMIN"
    }
  ]
}
```

#### Submit for Approval
```
POST /api/v1/invoices/submit
{"id": "uuid", "entity_id": "uuid"}
```

#### Approve Invoice
```
POST /api/v1/invoices/approve
{
  "id": "uuid",
  "entity_id": "uuid",
  "notes": "Approved - amounts verified"
}
```

#### Post to GL
```
POST /api/v1/invoices/post
{"id": "uuid", "entity_id": "uuid"}
```
Creates journal entry in GL-2 and updates vendor balance.

#### Record Payment
```
POST /api/v1/invoices/payment
{
  "invoice_id": "uuid",
  "entity_id": "uuid",
  "payment_date": "2024-02-10",
  "payment_amount": 27125,
  "payment_method": "ach",
  "payment_reference": "ACH-2024-0210"
}
```

#### Delete Invoice
```
DELETE /api/v1/invoices/delete?id={uuid}&entity_id={uuid}
```
Can only delete draft invoices.

## Database Schema

### Tables

#### invoices
- Invoice header with vendor, dates, totals
- Status: draft, pending_approval, approved, posted, paid, cancelled
- Computed amount_due = total_amount - amount_paid
- References: vendor_id (AP-1), gl_journal_id (GL-2)

#### invoice_lines
- Line items with GL account distribution
- Quantity, unit price, line amount
- Tax code, tax rate, tax amount
- 4 dimensions for reporting
- Cascading delete with parent invoice

#### invoice_payments
- Payment history (partial and full payments)
- Trigger auto-updates invoice.amount_paid and status

### Database Triggers

#### update_invoice_totals
Updates subtotal, tax_amount, total_amount when lines change.

#### update_invoice_payment_status
Updates amount_paid and status when payments recorded.

## Configuration

```bash
SERVICE_NAME=be-invoices-service
SERVICE_PORT=8085
DB_NAME=ap_invoices_db
```

## Integration with Other Services

### be-vendors-service (AP-1)
- **CRITICAL**: Validates vendor before invoice creation
- Updates vendor.current_balance when invoice posted/paid
- TODO: Implement vendor validation API call

### be-accounts-service (GL-1)
- **CRITICAL**: Validates GL accounts before posting
- Checks accounts allow posting (not summary accounts)
- TODO: Implement account validation API call

### be-journals-service (GL-2)
- **CRITICAL**: Creates journal entry when invoice posted
- Journal entry structure:
  - Debit: GL accounts from invoice lines
  - Credit: Accounts Payable (vendor liability)
- TODO: Implement journal creation API call

### be-identity-service (PLT-1)
- Future: JWT authentication
- User ID for created_by, approved_by, posted_by

## Workflow Example

### 1. Create Draft Invoice
```
POST /api/v1/invoices
```
Status: draft

### 2. Submit for Approval
```
POST /api/v1/invoices/submit
```
Status: pending_approval

### 3. Approve Invoice
```
POST /api/v1/invoices/approve
```
Status: approved

### 4. Post to GL
```
POST /api/v1/invoices/post
```
- Creates journal entry in GL-2
- Updates vendor balance in AP-1
- Status: posted

### 5. Record Payment
```
POST /api/v1/invoices/payment
```
- Creates payment record
- Updates amount_paid
- Status: paid (when fully paid)

## TODO: Critical Integration Points

1. **Vendor Validation** (service/invoice_service.go:105)
   - Call AP-1 `/api/v1/vendors/validate`
   - Verify vendor exists, is active, within credit limit

2. **Account Validation** (service/invoice_service.go:168, 231)
   - Call GL-1 `/api/v1/accounts/validate`
   - Verify accounts exist and allow posting

3. **GL Posting** (service/invoice_service.go:262)
   - Call GL-2 `/api/v1/journals` to create journal entry
   - Debit: GL accounts from lines
   - Credit: Accounts Payable

4. **Vendor Balance Update** (service/invoice_service.go:266, 338)
   - Call AP-1 to update vendor.current_balance
   - Increment on posting, decrement on payment

5. **Payment Journal Entry** (service/invoice_service.go:336)
   - Call GL-2 to create payment journal entry
   - Debit: Cash/Bank account
   - Credit: Accounts Payable

## License

Proprietary - Pesio AI
