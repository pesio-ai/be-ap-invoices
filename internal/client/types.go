package client

// Account represents a GL account
type Account struct {
	ID            string `json:"id"`
	EntityID      string `json:"entity_id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	AccountType   string `json:"account_type"`
	NormalBalance string `json:"normal_balance"`
	IsActive      bool   `json:"is_active"`
	AllowPosting  bool   `json:"allow_posting"`
	Currency      string `json:"currency"`
}

// Vendor represents a vendor/supplier
type Vendor struct {
	ID           string `json:"id"`
	EntityID     string `json:"entity_id"`
	VendorCode   string `json:"vendor_code"`
	VendorName   string `json:"vendor_name"`
	VendorType   string `json:"vendor_type"`
	Status       string `json:"status"`
	TaxID        string `json:"tax_id"`
	PaymentTerms string `json:"payment_terms"`
	Currency     string `json:"currency"`
}

// ValidateAccountResponse represents the account validation response
type ValidateAccountResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// ValidateVendorResponse represents the vendor validation response
type ValidateVendorResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// ListAccountsResponse represents the list accounts response
type ListAccountsResponse struct {
	Accounts []Account `json:"accounts"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}
