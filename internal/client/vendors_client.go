package client

import (
	"context"
	"fmt"

	"github.com/pesio-ai/be-lib-common/httpclient"
)

// VendorsClient is a client for the vendors service (AP-1)
type VendorsClient struct {
	client *httpclient.Client
}

// NewVendorsClient creates a new vendors service client
func NewVendorsClient(baseURL string) *VendorsClient {
	return &VendorsClient{
		client: httpclient.NewClient(baseURL),
	}
}

// ValidateVendor validates a vendor
func (c *VendorsClient) ValidateVendor(ctx context.Context, vendorID, entityID string) (bool, string, error) {
	path := fmt.Sprintf("/api/v1/vendors/validate?id=%s&entity_id=%s", vendorID, entityID)

	var resp ValidateVendorResponse
	if err := c.client.Get(ctx, path, &resp); err != nil {
		return false, "", fmt.Errorf("failed to validate vendor: %w", err)
	}

	return resp.Valid, resp.Message, nil
}

// UpdateBalanceRequest represents the update balance request
type UpdateBalanceRequest struct {
	VendorID string `json:"vendor_id"`
	EntityID string `json:"entity_id"`
	Amount   int64  `json:"amount"`
}

// UpdateBalance updates the vendor's current balance
func (c *VendorsClient) UpdateBalance(ctx context.Context, vendorID, entityID string, amount int64) error {
	req := UpdateBalanceRequest{
		VendorID: vendorID,
		EntityID: entityID,
		Amount:   amount,
	}

	if err := c.client.Post(ctx, "/api/v1/vendors/balance", req, nil); err != nil {
		return fmt.Errorf("failed to update vendor balance: %w", err)
	}

	return nil
}
