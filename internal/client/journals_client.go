package client

import (
	"context"
	"fmt"

	"github.com/pesio-ai/be-go-common/httpclient"
)

// JournalsClient is a client for the journals service (GL-2)
type JournalsClient struct {
	client *httpclient.Client
}

// NewJournalsClient creates a new journals service client
func NewJournalsClient(baseURL string) *JournalsClient {
	return &JournalsClient{
		client: httpclient.NewClient(baseURL),
	}
}

// JournalLineRequest represents a journal line
type JournalLineRequest struct {
	LineNumber  int     `json:"line_number"`
	AccountID   string  `json:"account_id"`
	LineType    string  `json:"line_type"` // "debit" or "credit"
	Amount      int64   `json:"amount"`
	Description *string `json:"description,omitempty"`
	Dimension1  *string `json:"dimension_1,omitempty"`
	Dimension2  *string `json:"dimension_2,omitempty"`
	Dimension3  *string `json:"dimension_3,omitempty"`
	Dimension4  *string `json:"dimension_4,omitempty"`
	Reference   *string `json:"reference,omitempty"`
}

// CreateJournalRequest represents a create journal entry request
type CreateJournalRequest struct {
	EntityID      string                `json:"entity_id"`
	JournalNumber string                `json:"journal_number"`
	JournalDate   string                `json:"journal_date"`
	JournalType   string                `json:"journal_type"`
	Description   *string               `json:"description,omitempty"`
	Reference     *string               `json:"reference,omitempty"`
	Currency      string                `json:"currency"`
	Lines         []*JournalLineRequest `json:"lines"`
}

// CreateJournalResponse represents the create journal response
type CreateJournalResponse struct {
	ID            string `json:"id"`
	JournalNumber string `json:"journal_number"`
	Status        string `json:"status"`
}

// CreateJournal creates a journal entry
func (c *JournalsClient) CreateJournal(ctx context.Context, req *CreateJournalRequest) (string, error) {
	var resp CreateJournalResponse
	if err := c.client.Post(ctx, "/api/v1/journals", req, &resp); err != nil {
		return "", fmt.Errorf("failed to create journal entry: %w", err)
	}

	return resp.ID, nil
}

// PostJournalRequest represents a post journal request
type PostJournalRequest struct {
	ID       string `json:"id"`
	EntityID string `json:"entity_id"`
}

// PostJournal posts a journal entry
func (c *JournalsClient) PostJournal(ctx context.Context, journalID, entityID string) error {
	req := PostJournalRequest{
		ID:       journalID,
		EntityID: entityID,
	}

	if err := c.client.Post(ctx, "/api/v1/journals/post", req, nil); err != nil {
		return fmt.Errorf("failed to post journal entry: %w", err)
	}

	return nil
}
