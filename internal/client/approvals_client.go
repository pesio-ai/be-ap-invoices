package client

import (
	"context"

	platpb "github.com/pesio-ai/be-lib-proto/gen/go/platform"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// ApprovalsGRPCClient wraps the platform ApprovalService gRPC client.
type ApprovalsGRPCClient struct {
	client platpb.ApprovalServiceClient
	conn   *grpc.ClientConn
}

// NewApprovalsGRPCClient dials the approvals gRPC service and returns a client.
func NewApprovalsGRPCClient(addr string) (*ApprovalsGRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &ApprovalsGRPCClient{
		client: platpb.NewApprovalServiceClient(conn),
		conn:   conn,
	}, nil
}

// Close releases the underlying gRPC connection.
func (c *ApprovalsGRPCClient) Close() error {
	return c.conn.Close()
}

// CreateWorkflow creates a new approval workflow for an entity.
func (c *ApprovalsGRPCClient) CreateWorkflow(
	ctx context.Context,
	entityID, entityType, entityRef, contextJSON, submittedBy string,
) (*platpb.Workflow, error) {
	resp, err := c.client.CreateWorkflow(ctx, &platpb.CreateWorkflowRequest{
		EntityId:    entityID,
		EntityType:  entityType,
		EntityRef:   entityRef,
		ContextJson: contextJSON,
		SubmittedBy: submittedBy,
	})
	if err != nil {
		return nil, err
	}
	return resp.Workflow, nil
}

// GetActiveWorkflow returns the active workflow for an entity, or nil if none exists.
func (c *ApprovalsGRPCClient) GetActiveWorkflow(
	ctx context.Context,
	entityID, entityType, entityRef string,
) (*platpb.Workflow, error) {
	resp, err := c.client.GetActiveWorkflow(ctx, &platpb.GetActiveWorkflowRequest{
		EntityId:   entityID,
		EntityType: entityType,
		EntityRef:  entityRef,
	})
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	return resp.Workflow, nil
}

// ApproveStep approves a single step in a workflow. Returns true when all steps are complete.
func (c *ApprovalsGRPCClient) ApproveStep(
	ctx context.Context,
	entityID, workflowID string,
	stepNumber int32,
	actedBy, notes string,
) (bool, *platpb.Workflow, error) {
	resp, err := c.client.ApproveStep(ctx, &platpb.ApproveStepRequest{
		EntityId:   entityID,
		WorkflowId: workflowID,
		StepNumber: stepNumber,
		ActedBy:    actedBy,
		Notes:      notes,
	})
	if err != nil {
		return false, nil, err
	}
	return resp.WorkflowComplete, resp.Workflow, nil
}

// RejectWorkflow rejects the current step and marks the entire workflow rejected.
func (c *ApprovalsGRPCClient) RejectWorkflow(
	ctx context.Context,
	entityID, workflowID string,
	stepNumber int32,
	actedBy, reason string,
) error {
	_, err := c.client.RejectWorkflow(ctx, &platpb.RejectWorkflowRequest{
		EntityId:   entityID,
		WorkflowId: workflowID,
		StepNumber: stepNumber,
		ActedBy:    actedBy,
		Reason:     reason,
	})
	return err
}

// RecallWorkflow cancels an in-progress workflow (submitter only).
func (c *ApprovalsGRPCClient) RecallWorkflow(
	ctx context.Context,
	entityID, workflowID, recalledBy string,
) error {
	_, err := c.client.RecallWorkflow(ctx, &platpb.RecallWorkflowRequest{
		EntityId:   entityID,
		WorkflowId: workflowID,
		RecalledBy: recalledBy,
	})
	return err
}

// DelegateStep delegates an approval step to another user.
func (c *ApprovalsGRPCClient) DelegateStep(
	ctx context.Context,
	entityID, workflowID string,
	stepNumber int32,
	delegatedBy, delegatedTo, reason string,
) error {
	_, err := c.client.DelegateStep(ctx, &platpb.DelegateStepRequest{
		EntityId:    entityID,
		WorkflowId:  workflowID,
		StepNumber:  stepNumber,
		DelegatedBy: delegatedBy,
		DelegatedTo: delegatedTo,
		Reason:      reason,
	})
	return err
}

// GetPendingApprovals returns all pending approval steps for a user across all entities.
func (c *ApprovalsGRPCClient) GetPendingApprovals(
	ctx context.Context,
	entityID, userID string,
) ([]*platpb.PendingApprovalItem, error) {
	resp, err := c.client.GetPendingApprovals(ctx, &platpb.GetPendingApprovalsRequest{
		EntityId: entityID,
		UserId:   userID,
	})
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}
