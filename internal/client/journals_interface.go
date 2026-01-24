package client

import "context"

// JournalsClientInterface defines the interface for journals service clients
type JournalsClientInterface interface {
	CreateJournal(ctx context.Context, req *CreateJournalRequest) (string, error)
	PostJournal(ctx context.Context, journalID, entityID string) error
}
