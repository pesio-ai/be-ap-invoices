package handler

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/pesio-ai/be-lib-proto/gen/go/common"
	pb "github.com/pesio-ai/be-lib-proto/gen/go/ap"
	"github.com/pesio-ai/be-lib-common/auth"
	"github.com/pesio-ai/be-ap-invoices/internal/client"
	"github.com/pesio-ai/be-ap-invoices/internal/repository"
	"github.com/pesio-ai/be-ap-invoices/internal/service"
)

// GRPCHandler implements the InvoicesService gRPC interface
type GRPCHandler struct {
	pb.UnimplementedInvoicesServiceServer
	invoiceService        *service.InvoiceService
	routingService        *service.ApprovalRoutingService
	notificationPublisher *client.NotificationPublisher
	logger                zerolog.Logger
}

// NewGRPCHandler creates a new gRPC handler
func NewGRPCHandler(invoiceService *service.InvoiceService, routingService *service.ApprovalRoutingService, notificationPublisher *client.NotificationPublisher, logger zerolog.Logger) *GRPCHandler {
	return &GRPCHandler{
		invoiceService:        invoiceService,
		routingService:        routingService,
		notificationPublisher: notificationPublisher,
		logger:                logger.With().Str("handler", "grpc").Logger(),
	}
}

// userID extracts the authenticated user ID from context, or returns empty string.
func userID(ctx context.Context) string {
	if uc, err := auth.GetUserContext(ctx); err == nil {
		return uc.UserID
	}
	return ""
}

// CreateInvoice creates a new invoice
func (h *GRPCHandler) CreateInvoice(ctx context.Context, req *pb.CreateInvoiceRequest) (*pb.Invoice, error) {
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("vendor_id", req.VendorId).
		Str("invoice_number", req.InvoiceNumber).
		Msg("gRPC CreateInvoice called")

	// Convert proto request to service request
	serviceReq := &service.CreateInvoiceRequest{
		EntityID:      req.EntityId,
		VendorID:      req.VendorId,
		InvoiceNumber: req.InvoiceNumber,
		InvoiceType:   "standard", // Default - proto doesn't have this field
		PaymentTerms:  "net30",    // Default - proto doesn't have this field
		Currency:      req.Currency,
	}

	// Set description if provided
	if req.Description != "" {
		serviceReq.Description = &req.Description
	}

	// Convert dates
	if req.InvoiceDate != nil {
		serviceReq.InvoiceDate = req.InvoiceDate.AsTime().Format("2006-01-02")
	} else {
		serviceReq.InvoiceDate = time.Now().Format("2006-01-02")
	}

	if req.DueDate != nil {
		serviceReq.DueDate = req.DueDate.AsTime().Format("2006-01-02")
	} else {
		// Default to 30 days from invoice date
		dueDate := time.Now().AddDate(0, 0, 30)
		serviceReq.DueDate = dueDate.Format("2006-01-02")
	}

	// Convert lines
	for i, line := range req.Lines {
		lineReq := &service.InvoiceLineRequest{
			LineNumber:  i + 1,
			AccountID:   line.AccountId,
			Description: line.Description,
			Quantity:    1, // Default quantity
			TaxCode:     &line.TaxCode,
		}

		// Extract amount from Money message
		if line.Amount != nil {
			lineReq.UnitPrice = line.Amount.Amount
			lineReq.LineAmount = line.Amount.Amount
		}

		// Extract tax from Money message
		if line.Tax != nil {
			lineReq.TaxAmount = line.Tax.Amount
		}

		serviceReq.Lines = append(serviceReq.Lines, lineReq)
	}

	// Call service
	invoice, err := h.invoiceService.CreateInvoice(ctx, serviceReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to create invoice")
		return nil, mapErrorToGRPC(err)
	}

	return invoiceToProto(invoice), nil
}

// GetInvoice retrieves an invoice by ID
func (h *GRPCHandler) GetInvoice(ctx context.Context, req *pb.GetInvoiceRequest) (*pb.Invoice, error) {
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Msg("gRPC GetInvoice called")

	invoice, err := h.invoiceService.GetInvoice(ctx, req.Id, req.EntityId)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get invoice")
		return nil, mapErrorToGRPC(err)
	}

	return invoiceToProto(invoice), nil
}

// UpdateInvoice updates an existing invoice
func (h *GRPCHandler) UpdateInvoice(ctx context.Context, req *pb.UpdateInvoiceRequest) (*pb.Invoice, error) {
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Msg("gRPC UpdateInvoice called")

	// NOTE: The service doesn't have an UpdateInvoice method yet
	// For now, return unimplemented
	return nil, status.Error(codes.Unimplemented, "UpdateInvoice not implemented")
}

// DeleteInvoice deletes an invoice
func (h *GRPCHandler) DeleteInvoice(ctx context.Context, req *pb.DeleteInvoiceRequest) (*commonpb.Response, error) {
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Msg("gRPC DeleteInvoice called")

	err := h.invoiceService.DeleteInvoice(ctx, req.Id, req.EntityId)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to delete invoice")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}

	return &commonpb.Response{Success: true, Message: "Invoice deleted"}, nil
}

// ListInvoices lists invoices with pagination and filtering
func (h *GRPCHandler) ListInvoices(ctx context.Context, req *pb.ListInvoicesRequest) (*pb.ListInvoicesResponse, error) {
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Msg("gRPC ListInvoices called")

	// Build filter parameters
	var vendorID *string
	if req.VendorId != "" {
		vendorID = &req.VendorId
	}

	var statusFilter *string
	if req.Status != pb.InvoiceStatus_INVOICE_STATUS_UNSPECIFIED {
		s := statusToString(req.Status)
		statusFilter = &s
	}

	// Handle date range
	var fromDate, toDate *string
	if req.DateRange != nil {
		if req.DateRange.Start != nil {
			fd := req.DateRange.Start.AsTime().Format("2006-01-02")
			fromDate = &fd
		}
		if req.DateRange.End != nil {
			td := req.DateRange.End.AsTime().Format("2006-01-02")
			toDate = &td
		}
	}

	// Pagination
	page := 1
	pageSize := 20
	if req.Pagination != nil {
		if req.Pagination.Page > 0 {
			page = int(req.Pagination.Page)
		}
		if req.Pagination.PageSize > 0 {
			pageSize = int(req.Pagination.PageSize)
		}
	}

	invoices, total, err := h.invoiceService.ListInvoices(ctx, req.EntityId, vendorID, statusFilter, fromDate, toDate, page, pageSize)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to list invoices")
		return nil, mapErrorToGRPC(err)
	}

	// Convert to proto
	pbInvoices := make([]*pb.Invoice, len(invoices))
	for i, inv := range invoices {
		pbInvoices[i] = invoiceToProto(inv)
	}

	totalPages := int32((int(total) + pageSize - 1) / pageSize)

	return &pb.ListInvoicesResponse{
		Invoices: pbInvoices,
		Pagination: &commonpb.PaginationResponse{
			Page:       int32(page),
			PageSize:   int32(pageSize),
			TotalItems: total,
			TotalPages: totalPages,
		},
	}, nil
}

// SubmitForApproval submits an invoice for approval and creates its workflow.
func (h *GRPCHandler) SubmitForApproval(ctx context.Context, req *pb.SubmitForApprovalRequest) (*commonpb.Response, error) {
	uid := userID(ctx)
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Str("submitted_by", uid).
		Msg("gRPC SubmitForApproval called")

	// 1. Update invoice status to pending_approval
	if err := h.invoiceService.SubmitForApproval(ctx, req.Id, req.EntityId, uid); err != nil {
		h.logger.Error().Err(err).Msg("Failed to submit for approval")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}

	// 2. Create approval workflow (non-fatal: invoice is already submitted)
	invoice, err := h.invoiceService.GetInvoice(ctx, req.Id, req.EntityId)
	if err != nil {
		h.logger.Warn().Err(err).Str("invoice_id", req.Id).Msg("Could not fetch invoice for workflow creation")
	} else {
		wf, steps, err := h.routingService.CreateApprovalWorkflow(ctx, invoice, uid)
		if err != nil {
			h.logger.Warn().Err(err).Str("invoice_id", req.Id).Msg("Could not create approval workflow")
		} else {
			h.logger.Info().
				Str("invoice_id", req.Id).
				Str("workflow_id", wf.ID).
				Int("total_steps", wf.TotalSteps).
				Msg("Approval workflow created")

			// Notify approver(s) + submitter about the new pending approval
			if h.notificationPublisher != nil {
				// Always notify the submitter; add the first assigned approver if known
				recipients := []string{uid}
				if len(steps) > 0 && steps[0].AssignedTo != nil && *steps[0].AssignedTo != uid {
					recipients = append(recipients, *steps[0].AssignedTo)
				}
				payload := map[string]interface{}{
					"InvoiceNumber": invoice.InvoiceNumber,
					"VendorName":    invoice.VendorID,
					"Amount":        invoice.TotalAmount,
				}
				if len(steps) > 0 {
					payload["StepNumber"] = steps[0].StepNumber
				}
				h.notificationPublisher.PublishInvoiceEvent(ctx, "invoice_submitted",
					req.Id, req.EntityId, uid, recipients, payload,
				)
			}
		}
	}

	return &commonpb.Response{Success: true, Message: "Invoice submitted for approval"}, nil
}

// ApproveInvoice advances the approval workflow by one step for the current user.
func (h *GRPCHandler) ApproveInvoice(ctx context.Context, req *pb.ApproveInvoiceRequest) (*commonpb.Response, error) {
	uid := userID(ctx)
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Str("acted_by", uid).
		Msg("gRPC ApproveInvoice called")

	// Resolve active workflow
	wf, err := h.routingService.GetActiveWorkflow(ctx, req.Id)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get active workflow")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}

	var notes *string
	if req.Comments != "" {
		notes = &req.Comments
	}

	if wf != nil {
		// Multi-step workflow path
		_, err = h.routingService.ApproveStep(ctx, req.Id, wf.ID, wf.CurrentStep, uid, notes)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to approve workflow step")
			return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
		}
		return &commonpb.Response{Success: true, Message: "Approval step recorded"}, nil
	}

	// Legacy single-step path (no workflow)
	approvedBy := uid
	if approvedBy == "" {
		approvedBy = req.ApprovedBy
	}
	approveReq := &service.ApproveInvoiceRequest{
		ID:         req.Id,
		EntityID:   req.EntityId,
		ApprovedBy: approvedBy,
		Notes:      notes,
	}
	if _, err := h.invoiceService.ApproveInvoice(ctx, approveReq); err != nil {
		h.logger.Error().Err(err).Msg("Failed to approve invoice")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}
	return &commonpb.Response{Success: true, Message: "Invoice approved"}, nil
}

// RejectInvoice rejects the active workflow step and returns the invoice to draft.
func (h *GRPCHandler) RejectInvoice(ctx context.Context, req *pb.RejectInvoiceRequest) (*commonpb.Response, error) {
	uid := userID(ctx)
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Str("acted_by", uid).
		Msg("gRPC RejectInvoice called")

	if req.Reason == "" {
		return &commonpb.Response{Success: false, Message: "reason is required"}, status.Error(codes.InvalidArgument, "reason is required")
	}

	wf, err := h.routingService.GetActiveWorkflow(ctx, req.Id)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get active workflow")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}
	if wf == nil {
		return &commonpb.Response{Success: false, Message: "no active workflow found"}, status.Error(codes.NotFound, "no active workflow found")
	}

	rejectedBy := uid
	if rejectedBy == "" {
		rejectedBy = req.RejectedBy
	}
	if err := h.routingService.RejectWorkflow(ctx, req.Id, wf.ID, wf.CurrentStep, rejectedBy, req.Reason); err != nil {
		h.logger.Error().Err(err).Msg("Failed to reject invoice")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}

	// Notify submitter of rejection (non-fatal)
	if h.notificationPublisher != nil && wf.SubmittedBy != "" {
		h.notificationPublisher.PublishInvoiceEvent(ctx, "invoice_rejected",
			req.Id, req.EntityId, rejectedBy, []string{wf.SubmittedBy},
			map[string]interface{}{
				"InvoiceNumber": req.Id,
				"Reason":        req.Reason,
			},
		)
	}

	return &commonpb.Response{Success: true, Message: "Invoice rejected"}, nil
}

// RecallInvoice cancels a pending-approval invoice (submitter only).
func (h *GRPCHandler) RecallInvoice(ctx context.Context, req *pb.RecallInvoiceRequest) (*commonpb.Response, error) {
	uid := userID(ctx)
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Str("recalled_by", uid).
		Msg("gRPC RecallInvoice called")

	wf, err := h.routingService.GetActiveWorkflow(ctx, req.Id)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get active workflow for recall")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}

	if wf != nil {
		// Workflow path: recall via routing service (validates submitter, recalls steps)
		if err := h.routingService.RecallWorkflow(ctx, req.Id, wf.ID, uid); err != nil {
			h.logger.Error().Err(err).Msg("Failed to recall workflow")
			return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
		}
	} else {
		// Legacy path: direct status update
		if err := h.invoiceService.RecallInvoice(ctx, req.Id, req.EntityId, uid); err != nil {
			h.logger.Error().Err(err).Msg("Failed to recall invoice")
			return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
		}
	}

	return &commonpb.Response{Success: true, Message: "Invoice recalled"}, nil
}

// DelegateApproval delegates an approval step to another user.
func (h *GRPCHandler) DelegateApproval(ctx context.Context, req *pb.DelegateApprovalRequest) (*commonpb.Response, error) {
	uid := userID(ctx)
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("invoice_id", req.InvoiceId).
		Int32("step_number", req.StepNumber).
		Str("delegated_to", req.DelegatedTo).
		Msg("gRPC DelegateApproval called")

	if req.DelegatedTo == "" {
		return &commonpb.Response{Success: false, Message: "delegated_to is required"}, status.Error(codes.InvalidArgument, "delegated_to is required")
	}
	if req.Reason == "" {
		return &commonpb.Response{Success: false, Message: "reason is required"}, status.Error(codes.InvalidArgument, "reason is required")
	}

	wf, err := h.routingService.GetActiveWorkflow(ctx, req.InvoiceId)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get active workflow")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}
	if wf == nil {
		return &commonpb.Response{Success: false, Message: "no active workflow found"}, status.Error(codes.NotFound, "no active workflow found")
	}

	if err := h.routingService.DelegateStep(ctx, wf.ID, int(req.StepNumber), uid, req.DelegatedTo, req.Reason); err != nil {
		h.logger.Error().Err(err).Msg("Failed to delegate approval")
		return &commonpb.Response{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}

	return &commonpb.Response{Success: true, Message: "Approval delegated"}, nil
}

// GetApprovalHistory returns the full audit trail for an invoice.
func (h *GRPCHandler) GetApprovalHistory(ctx context.Context, req *pb.GetApprovalHistoryRequest) (*pb.GetApprovalHistoryResponse, error) {
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("invoice_id", req.InvoiceId).
		Msg("gRPC GetApprovalHistory called")

	entries, err := h.routingService.GetApprovalHistory(ctx, req.InvoiceId, req.EntityId)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get approval history")
		return nil, mapErrorToGRPC(err)
	}

	pbEntries := make([]*pb.ApprovalHistoryEntry, 0, len(entries))
	for _, e := range entries {
		entry := &pb.ApprovalHistoryEntry{
			Id:                  e.ID,
			InvoiceId:           e.InvoiceID,
			Action:              e.Action,
			PerformedBy:         e.PerformedBy,
			PerformedAt:         timestamppb.New(e.PerformedAt),
		}
		if e.WorkflowID != nil {
			entry.WorkflowId = *e.WorkflowID
		}
		if e.StepID != nil {
			entry.StepId = *e.StepID
		}
		if e.InvoiceStatusBefore != nil {
			entry.InvoiceStatusBefore = *e.InvoiceStatusBefore
		}
		if e.InvoiceStatusAfter != nil {
			entry.InvoiceStatusAfter = *e.InvoiceStatusAfter
		}
		// Extract notes from metadata if present
		if notes, ok := e.Metadata["reason"].(string); ok && notes != "" {
			entry.Notes = notes
		} else if notes, ok := e.Metadata["action_notes"].(string); ok && notes != "" {
			entry.Notes = notes
		}
		pbEntries = append(pbEntries, entry)
	}

	return &pb.GetApprovalHistoryResponse{Entries: pbEntries}, nil
}

// GetPendingApprovals returns approval steps awaiting action from the user.
func (h *GRPCHandler) GetPendingApprovals(ctx context.Context, req *pb.GetPendingApprovalsRequest) (*pb.GetPendingApprovalsResponse, error) {
	uid := req.UserId
	if uid == "" {
		uid = userID(ctx)
	}
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("user_id", uid).
		Msg("gRPC GetPendingApprovals called")

	steps, err := h.routingService.GetPendingApprovals(ctx, req.EntityId, uid)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get pending approvals")
		return nil, mapErrorToGRPC(err)
	}

	items := make([]*pb.PendingApprovalItem, 0, len(steps))
	for _, s := range steps {
		item := &pb.PendingApprovalItem{
			StepId:       s.ID,
			WorkflowId:   s.WorkflowID,
			InvoiceId:    s.InvoiceID,
			EntityId:     s.EntityID,
			StepNumber:   int32(s.StepNumber),
			RequiredRole: s.RequiredRole,
			CreatedAt:    timestamppb.New(s.CreatedAt),
		}
		if s.AssignedTo != nil {
			item.AssignedTo = *s.AssignedTo
		}
		if s.DelegatedTo != nil {
			item.DelegatedTo = *s.DelegatedTo
		}
		if s.DueAt != nil {
			item.DueAt = timestamppb.New(*s.DueAt)
		}
		items = append(items, item)
	}

	return &pb.GetPendingApprovalsResponse{
		Items: items,
		Total: int32(len(items)),
	}, nil
}

// PostToGL posts an invoice to the general ledger
func (h *GRPCHandler) PostToGL(ctx context.Context, req *pb.PostToGLRequest) (*pb.PostToGLResponse, error) {
	h.logger.Info().
		Str("entity_id", req.EntityId).
		Str("id", req.Id).
		Str("period_id", req.PeriodId).
		Msg("gRPC PostToGL called")

	postReq := &service.PostInvoiceRequest{
		ID:       req.Id,
		EntityID: req.EntityId,
		PostedBy: userID(ctx),
	}

	invoice, err := h.invoiceService.PostInvoice(ctx, postReq)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to post to GL")
		return &pb.PostToGLResponse{Success: false, Message: err.Error()}, mapErrorToGRPC(err)
	}

	journalID := ""
	if invoice.GLJournalID != nil {
		journalID = *invoice.GLJournalID
	}

	return &pb.PostToGLResponse{
		Success:        true,
		JournalEntryId: journalID,
		Message:        "Invoice posted to GL",
	}, nil
}

// Helper functions

func invoiceToProto(inv *repository.Invoice) *pb.Invoice {
	if inv == nil {
		return nil
	}

	pbInvoice := &pb.Invoice{
		Id:            inv.ID,
		EntityId:      inv.EntityID,
		InvoiceNumber: inv.InvoiceNumber,
		VendorId:      inv.VendorID,
		Status:        stringToStatus(inv.Status),
		Currency:      inv.Currency,
		Subtotal:      &commonpb.Money{Amount: inv.Subtotal, Currency: inv.Currency},
		Tax:           &commonpb.Money{Amount: inv.TaxAmount, Currency: inv.Currency},
		Total:         &commonpb.Money{Amount: inv.TotalAmount, Currency: inv.Currency},
		InvoiceDate:   timestamppb.New(inv.InvoiceDate),
		DueDate:       timestamppb.New(inv.DueDate),
	}

	// Set description if present
	if inv.Description != nil {
		pbInvoice.Description = *inv.Description
	}

	// Set GL journal ID if posted
	if inv.GLJournalID != nil {
		pbInvoice.GlJournalEntryId = *inv.GLJournalID
	}

	// Set posted info
	if inv.PostedDate != nil {
		pbInvoice.PostedAt = timestamppb.New(*inv.PostedDate)
	}
	if inv.PostedBy != nil {
		pbInvoice.PostedBy = *inv.PostedBy
	}

	// Set audit info
	pbInvoice.Audit = &commonpb.AuditInfo{
		CreatedAt: timestamppb.New(inv.CreatedAt),
		UpdatedAt: timestamppb.New(inv.UpdatedAt),
	}
	if inv.CreatedBy != nil {
		pbInvoice.Audit.CreatedBy = *inv.CreatedBy
	}
	if inv.UpdatedBy != nil {
		pbInvoice.Audit.UpdatedBy = *inv.UpdatedBy
	}

	// Convert lines
	for _, line := range inv.Lines {
		pbInvoice.Lines = append(pbInvoice.Lines, &pb.InvoiceLine{
			Id:          line.ID,
			LineNumber:  int32(line.LineNumber),
			Description: line.Description,
			AccountId:   line.AccountID,
			Amount:      &commonpb.Money{Amount: line.LineAmount, Currency: inv.Currency},
			Tax:         &commonpb.Money{Amount: line.TaxAmount, Currency: inv.Currency},
		})
	}

	return pbInvoice
}

func stringToStatus(s string) pb.InvoiceStatus {
	switch s {
	case "draft":
		return pb.InvoiceStatus_INVOICE_STATUS_DRAFT
	case "pending_approval":
		return pb.InvoiceStatus_INVOICE_STATUS_PENDING_APPROVAL
	case "approved":
		return pb.InvoiceStatus_INVOICE_STATUS_APPROVED
	case "rejected":
		return pb.InvoiceStatus_INVOICE_STATUS_REJECTED
	case "posted":
		return pb.InvoiceStatus_INVOICE_STATUS_POSTED
	case "paid":
		return pb.InvoiceStatus_INVOICE_STATUS_PAID
	case "cancelled":
		return pb.InvoiceStatus_INVOICE_STATUS_CANCELLED
	default:
		return pb.InvoiceStatus_INVOICE_STATUS_UNSPECIFIED
	}
}

func statusToString(s pb.InvoiceStatus) string {
	switch s {
	case pb.InvoiceStatus_INVOICE_STATUS_DRAFT:
		return "draft"
	case pb.InvoiceStatus_INVOICE_STATUS_PENDING_APPROVAL:
		return "pending_approval"
	case pb.InvoiceStatus_INVOICE_STATUS_APPROVED:
		return "approved"
	case pb.InvoiceStatus_INVOICE_STATUS_REJECTED:
		return "rejected"
	case pb.InvoiceStatus_INVOICE_STATUS_POSTED:
		return "posted"
	case pb.InvoiceStatus_INVOICE_STATUS_PAID:
		return "paid"
	case pb.InvoiceStatus_INVOICE_STATUS_CANCELLED:
		return "cancelled"
	default:
		return ""
	}
}

func mapErrorToGRPC(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	switch {
	case contains(errMsg, "not found"):
		return status.Error(codes.NotFound, errMsg)
	case contains(errMsg, "already exists"):
		return status.Error(codes.AlreadyExists, errMsg)
	case contains(errMsg, "invalid"):
		return status.Error(codes.InvalidArgument, errMsg)
	case contains(errMsg, "unauthorized"):
		return status.Error(codes.Unauthenticated, errMsg)
	case contains(errMsg, "forbidden"):
		return status.Error(codes.PermissionDenied, errMsg)
	case contains(errMsg, "conflict"), contains(errMsg, "cannot"):
		return status.Error(codes.FailedPrecondition, errMsg)
	default:
		return status.Error(codes.Internal, errMsg)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
