package client

import (
	"context"
	"fmt"

	pb "github.com/pesio-ai/be-lib-proto/gen/go/gl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// JournalsGRPCClient is a gRPC client for the journals service (GL-2)
type JournalsGRPCClient struct {
	client pb.JournalsServiceClient
	conn   *grpc.ClientConn
}

// NewJournalsGRPCClient creates a new journals gRPC client
func NewJournalsGRPCClient(address string) (*JournalsGRPCClient, error) {
	// Create gRPC connection
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to journals service: %w", err)
	}

	client := pb.NewJournalsServiceClient(conn)

	return &JournalsGRPCClient{
		client: client,
		conn:   conn,
	}, nil
}

// Close closes the gRPC connection
func (c *JournalsGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// CreateJournal creates a journal entry via gRPC
func (c *JournalsGRPCClient) CreateJournal(ctx context.Context, req *CreateJournalRequest) (string, error) {
	// Convert to proto request
	lines := make([]*pb.CreateJournalLineRequest, len(req.Lines))
	for i, line := range req.Lines {
		lines[i] = &pb.CreateJournalLineRequest{
			LineNumber:  int32(line.LineNumber),
			AccountId:   line.AccountID,
			LineType:    lineTypeToProto(line.LineType),
			Amount:      line.Amount,
			Description: stringPtrToString(line.Description),
			Dimension1:  stringPtrToString(line.Dimension1),
			Dimension2:  stringPtrToString(line.Dimension2),
			Dimension3:  stringPtrToString(line.Dimension3),
			Dimension4:  stringPtrToString(line.Dimension4),
			Reference:   stringPtrToString(line.Reference),
		}
	}

	grpcReq := &pb.CreateJournalEntryRequest{
		EntityId:      req.EntityID,
		JournalNumber: req.JournalNumber,
		JournalDate:   req.JournalDate,
		JournalType:   journalTypeToProto(req.JournalType),
		Description:   stringPtrToString(req.Description),
		Reference:     stringPtrToString(req.Reference),
		Currency:      req.Currency,
		Lines:         lines,
	}

	// Call gRPC service
	resp, err := c.client.CreateJournalEntry(ctx, grpcReq)
	if err != nil {
		return "", fmt.Errorf("failed to create journal entry: %w", err)
	}

	return resp.Id, nil
}

// PostJournal posts a journal entry via gRPC
func (c *JournalsGRPCClient) PostJournal(ctx context.Context, journalID, entityID string) error {
	req := &pb.PostJournalEntryRequest{
		Id:       journalID,
		EntityId: entityID,
	}

	_, err := c.client.PostJournalEntry(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to post journal entry: %w", err)
	}

	return nil
}

// Helper functions

func lineTypeToProto(lineType string) pb.LineType {
	switch lineType {
	case "debit":
		return pb.LineType_LINE_TYPE_DEBIT
	case "credit":
		return pb.LineType_LINE_TYPE_CREDIT
	default:
		return pb.LineType_LINE_TYPE_UNSPECIFIED
	}
}

func journalTypeToProto(journalType string) pb.JournalEntryType {
	switch journalType {
	case "standard":
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_MANUAL
	case "ap_invoice":
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_AP_INVOICE
	case "ap_payment":
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_AP_PAYMENT
	case "ar_invoice":
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_AR_INVOICE
	case "ar_payment":
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_AR_PAYMENT
	case "adjusting":
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_ADJUSTMENT
	case "closing":
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_CLOSING
	default:
		return pb.JournalEntryType_JOURNAL_ENTRY_TYPE_MANUAL
	}
}

func stringPtrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
