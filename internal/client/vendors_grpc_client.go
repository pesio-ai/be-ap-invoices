package client

import (
	"context"
	"fmt"

	pb "github.com/pesio-ai/be-go-proto/gen/go/ap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// VendorsGRPCClient is a gRPC client for the vendors service (AP-1)
type VendorsGRPCClient struct {
	conn   *grpc.ClientConn
	client pb.VendorsServiceClient
}

// NewVendorsGRPCClient creates a new vendors service gRPC client
func NewVendorsGRPCClient(addr string) (*VendorsGRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	return &VendorsGRPCClient{
		conn:   conn,
		client: pb.NewVendorsServiceClient(conn),
	}, nil
}

// Close closes the gRPC connection
func (c *VendorsGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ValidateVendor validates a vendor
func (c *VendorsGRPCClient) ValidateVendor(ctx context.Context, vendorID, entityID string) (bool, string, error) {
	resp, err := c.client.ValidateVendor(ctx, &pb.ValidateVendorRequest{
		Id:       vendorID,
		EntityId: entityID,
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to validate vendor: %w", err)
	}

	return resp.Valid, resp.Message, nil
}

// GetVendor retrieves a vendor by ID
func (c *VendorsGRPCClient) GetVendor(ctx context.Context, vendorID, entityID string) (*Vendor, error) {
	resp, err := c.client.GetVendor(ctx, &pb.GetVendorRequest{
		Id:       vendorID,
		EntityId: entityID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get vendor: %w", err)
	}

	return &Vendor{
		ID:           resp.Id,
		EntityID:     resp.EntityId,
		VendorCode:   resp.VendorCode,
		VendorName:   resp.VendorName,
		VendorType:   resp.VendorType,
		Status:       resp.Status,
		TaxID:        resp.TaxId,
		PaymentTerms: resp.PaymentTerms,
		Currency:     resp.Currency,
	}, nil
}

// GetVendorByCode retrieves a vendor by code
func (c *VendorsGRPCClient) GetVendorByCode(ctx context.Context, code, entityID string) (*Vendor, error) {
	// Use ListVendors with a filter (search through the list for the code)
	resp, err := c.client.ListVendors(ctx, &pb.ListVendorsRequest{
		EntityId: entityID,
		Page:     1,
		PageSize: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list vendors: %w", err)
	}

	for _, v := range resp.Vendors {
		if v.VendorCode == code {
			return &Vendor{
				ID:           v.Id,
				EntityID:     v.EntityId,
				VendorCode:   v.VendorCode,
				VendorName:   v.VendorName,
				VendorType:   v.VendorType,
				Status:       v.Status,
				TaxID:        v.TaxId,
				PaymentTerms: v.PaymentTerms,
				Currency:     v.Currency,
			}, nil
		}
	}

	return nil, fmt.Errorf("vendor not found with code %s", code)
}

// UpdateBalance updates the vendor's current balance
func (c *VendorsGRPCClient) UpdateBalance(ctx context.Context, vendorID, entityID string, amount int64) error {
	_, err := c.client.UpdateBalance(ctx, &pb.UpdateBalanceRequest{
		Id:       vendorID,
		EntityId: entityID,
		Amount:   amount,
	})
	if err != nil {
		return fmt.Errorf("failed to update vendor balance: %w", err)
	}

	return nil
}
