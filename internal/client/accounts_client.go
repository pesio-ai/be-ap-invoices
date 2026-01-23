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

// ValidateAccountResponse represents the account validation response
type ValidateAccountResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
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
