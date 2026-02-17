package client

import (
	"context"
	"fmt"

	pb "github.com/pesio-ai/be-lib-proto/gen/go/gl"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AccountsGRPCClient is a gRPC client for the accounts service (GL-1)
type AccountsGRPCClient struct {
	conn   *grpc.ClientConn
	client pb.AccountsServiceClient
}

// NewAccountsGRPCClient creates a new accounts service gRPC client
func NewAccountsGRPCClient(addr string) (*AccountsGRPCClient, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(forwardMetadata),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	return &AccountsGRPCClient{
		conn:   conn,
		client: pb.NewAccountsServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *AccountsGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ValidateAccount validates a GL account
func (c *AccountsGRPCClient) ValidateAccount(ctx context.Context, accountID, entityID string) (bool, string, error) {
	resp, err := c.client.ValidateAccount(ctx, &pb.ValidateAccountRequest{
		Id:       accountID,
		EntityId: entityID,
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to validate account: %w", err)
	}

	return resp.Valid, resp.Message, nil
}

// GetAccount retrieves an account by ID
func (c *AccountsGRPCClient) GetAccount(ctx context.Context, accountID, entityID string) (*Account, error) {
	resp, err := c.client.GetAccount(ctx, &pb.GetAccountRequest{
		Id:       accountID,
		EntityId: entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &Account{
		ID:            resp.Id,
		EntityID:      resp.EntityId,
		Code:          resp.Code,
		Name:          resp.Name,
		AccountType:   accountTypeToString(resp.AccountType),
		NormalBalance: normalBalanceToString(resp.NormalBalance),
		IsActive:      resp.IsActive,
		AllowPosting:  resp.AllowPosting,
		Currency:      resp.Currency,
	}, nil
}

// ListAccounts retrieves accounts with filtering
func (c *AccountsGRPCClient) ListAccounts(ctx context.Context, entityID, accountType string, page, pageSize int) ([]*Account, int64, error) {
	resp, err := c.client.ListAccounts(ctx, &pb.ListAccountsRequest{
		EntityId:    entityID,
		AccountType: stringToAccountType(accountType),
		Page:        int32(page),
		PageSize:    int32(pageSize),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list accounts: %w", err)
	}

	accounts := make([]*Account, len(resp.Accounts))
	for i, acc := range resp.Accounts {
		accounts[i] = &Account{
			ID:            acc.Id,
			EntityID:      acc.EntityId,
			Code:          acc.Code,
			Name:          acc.Name,
			AccountType:   accountTypeToString(acc.AccountType),
			NormalBalance: normalBalanceToString(acc.NormalBalance),
			IsActive:      acc.IsActive,
			AllowPosting:  acc.AllowPosting,
			Currency:      acc.Currency,
		}
	}

	return accounts, resp.Total, nil
}

// GetAccountByCode retrieves an account by its code
func (c *AccountsGRPCClient) GetAccountByCode(ctx context.Context, code, entityID string) (*Account, error) {
	// Use ListAccounts with a filter (the proto doesn't have a GetByCode method)
	// We'll need to search through the list
	accounts, _, err := c.ListAccounts(ctx, entityID, "", 1, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to get account by code: %w", err)
	}

	for _, acc := range accounts {
		if acc.Code == code {
			return acc, nil
		}
	}

	return nil, fmt.Errorf("account not found with code %s", code)
}

// Helper functions for type conversion

func accountTypeToString(t pb.AccountType) string {
	switch t {
	case pb.AccountType_ACCOUNT_TYPE_ASSET:
		return "asset"
	case pb.AccountType_ACCOUNT_TYPE_LIABILITY:
		return "liability"
	case pb.AccountType_ACCOUNT_TYPE_EQUITY:
		return "equity"
	case pb.AccountType_ACCOUNT_TYPE_REVENUE:
		return "revenue"
	case pb.AccountType_ACCOUNT_TYPE_EXPENSE:
		return "expense"
	default:
		return ""
	}
}

func stringToAccountType(s string) pb.AccountType {
	switch s {
	case "asset":
		return pb.AccountType_ACCOUNT_TYPE_ASSET
	case "liability":
		return pb.AccountType_ACCOUNT_TYPE_LIABILITY
	case "equity":
		return pb.AccountType_ACCOUNT_TYPE_EQUITY
	case "revenue":
		return pb.AccountType_ACCOUNT_TYPE_REVENUE
	case "expense":
		return pb.AccountType_ACCOUNT_TYPE_EXPENSE
	default:
		return pb.AccountType_ACCOUNT_TYPE_UNSPECIFIED
	}
}

func normalBalanceToString(b pb.NormalBalance) string {
	switch b {
	case pb.NormalBalance_NORMAL_BALANCE_DEBIT:
		return "debit"
	case pb.NormalBalance_NORMAL_BALANCE_CREDIT:
		return "credit"
	default:
		return ""
	}
}
