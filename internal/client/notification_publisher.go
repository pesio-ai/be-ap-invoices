package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	natsclient "github.com/pesio-ai/be-lib-common/nats"
)

// NotificationPublisher publishes approval workflow events to NATS JetStream
// for consumption by the be-plt-notifications service.
//
// Subject convention: notifications.ap.<event_type>
// Event types: invoice_submitted, invoice_approval_required, invoice_approved,
//              invoice_rejected, invoice_recalled
//
// All publish operations are non-fatal — errors are logged but never propagated
// to the caller, so notification failures never interrupt approval operations.
type NotificationPublisher struct {
	nats *natsclient.Client
	log  zerolog.Logger
}

// NotificationEvent is the JSON schema published to NATS.
type NotificationEvent struct {
	EventType    string                 `json:"event_type"`
	EntityID     string                 `json:"entity_id"`
	ActorID      string                 `json:"actor_id"`
	Recipients   []string               `json:"recipients"`
	ResourceType string                 `json:"resource_type,omitempty"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	IsActionable bool                   `json:"is_actionable,omitempty"`
	ActionURL    string                 `json:"action_url,omitempty"`
	Severity     string                 `json:"severity,omitempty"`
	Category     string                 `json:"category,omitempty"`
	Payload      map[string]interface{} `json:"payload,omitempty"`
}

// NewNotificationPublisher creates a publisher backed by the given NATS client.
func NewNotificationPublisher(nats *natsclient.Client, log zerolog.Logger) *NotificationPublisher {
	return &NotificationPublisher{nats: nats, log: log}
}

// PublishInvoiceEvent publishes an AP invoice approval event to NATS.
// Subject: notifications.ap.<eventType>
func (p *NotificationPublisher) PublishInvoiceEvent(ctx context.Context, eventType, invoiceID, entityID, actorID string, recipients []string, payload map[string]interface{}) {
	if p.nats == nil {
		return
	}
	if len(recipients) == 0 {
		return
	}

	event := &NotificationEvent{
		EventType:    eventType,
		EntityID:     entityID,
		ActorID:      actorID,
		Recipients:   recipients,
		ResourceType: "invoice",
		ResourceID:   invoiceID,
		IsActionable: true,
		Severity:     "info",
		Category:     "ap_approval",
		Payload:      payload,
	}

	data, err := json.Marshal(event)
	if err != nil {
		p.log.Warn().Err(err).Str("event_type", eventType).Msg("notification: failed to marshal event")
		return
	}

	subject := fmt.Sprintf("notifications.ap.%s", eventType)
	if err := p.nats.Publish(ctx, subject, data); err != nil {
		p.log.Warn().Err(err).
			Str("subject", subject).
			Str("invoice_id", invoiceID).
			Msg("notification: failed to publish NATS event (non-fatal)")
		return
	}

	p.log.Debug().
		Str("subject", subject).
		Str("invoice_id", invoiceID).
		Int("recipients", len(recipients)).
		Msg("notification: event published")
}
