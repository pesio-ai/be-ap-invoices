package client

import (
	"context"
	"fmt"

	"github.com/pesio-ai/be-go-common/httpclient"
)

// AccountsClient is a client for the accounts service (GL-1)
type AccountsClient struct {
	client *httpclient.Client
}

// NewAccountsClient creates a new accounts service client
func NewAccountsClient(baseURL string) *AccountsClient {
	return &AccountsClient{
		client: httpclient.NewClient(baseURL),
	}
}

// ValidateAccount validates a GL account
func (c *AccountsClient) ValidateAccount(ctx context.Context, accountID, entityID string) (bool, string, error) {
	path := fmt.Sprintf("/api/v1/accounts/validate?id=%s&entity_id=%s", accountID, entityID)

	var resp ValidateAccountResponse
	if err := c.client.Get(ctx, path, &resp); err != nil {
		return false, "", fmt.Errorf("failed to validate account: %w", err)
	}

	return resp.Valid, resp.Message, nil
}

// GetAccountByCode retrieves an account by its code
func (c *AccountsClient) GetAccountByCode(ctx context.Context, code, entityID string) (*Account, error) {
	path := fmt.Sprintf("/api/v1/accounts?entity_id=%s&code=%s", entityID, code)

	var resp ListAccountsResponse
	if err := c.client.Get(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("failed to get account by code: %w", err)
	}

	if len(resp.Accounts) == 0 {
		return nil, fmt.Errorf("account not found with code %s", code)
	}

	return &resp.Accounts[0], nil
}
