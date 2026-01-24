package client

import "context"

// VendorsClientInterface defines the interface for vendors service client
type VendorsClientInterface interface {
	ValidateVendor(ctx context.Context, vendorID, entityID string) (bool, string, error)
	UpdateBalance(ctx context.Context, vendorID, entityID string, amount int64) error
}

// AccountsClientInterface defines the interface for accounts service client
type AccountsClientInterface interface {
	ValidateAccount(ctx context.Context, accountID, entityID string) (bool, string, error)
	GetAccountByCode(ctx context.Context, code, entityID string) (*Account, error)
}
