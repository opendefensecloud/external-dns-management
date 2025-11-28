// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package hetzner

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

type handler struct {
	provider.DefaultDNSHandler
	config provider.DNSHandlerConfig
	client *Client
}

// NewHandler creates a new Hetzner DNS handler.
func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	h := &handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
		config:            *c,
	}

	// Extract credentials
	token, err := c.GetRequiredProperty("HETZNER_API_TOKEN")
	if err != nil {
		return nil, fmt.Errorf("missing API token: %w", err)
	}

	// Optional endpoint override
	endpoint := c.GetDefaultedProperty("HETZNER_ENDPOINT", defaultBaseURL)

	// Optional timeout
	timeoutStr := c.GetDefaultedProperty("HETZNER_HTTP_TIMEOUT", "30")
	timeout := 30
	if timeoutStr != "" {
		if _, err := fmt.Sscanf(timeoutStr, "%d", &timeout); err != nil {
			return nil, fmt.Errorf("invalid timeout value: %w", err)
		}
	}

	// Initialize client
	h.client = NewClient(ClientConfig{
		BaseURL:     endpoint,
		Token:       token,
		HTTPTimeout: time.Duration(timeout) * time.Second,
		Logger:      c.Log,
	})

	return h, nil
}

// GetZones returns all DNS zones managed by this provider.
func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
	log, _ := logr.FromContext(ctx)
	log = log.WithValues("provider", h.ProviderType())

	// Rate limit at provider level (in addition to client level)
	h.config.RateLimiter.Accept()

	// Fetch zones from Hetzner
	zones, err := h.client.ListZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list zones: %w", err)
	}

	// Track metrics
	h.config.Metrics.AddGenericRequests(provider.MetricsRequestTypeListZones, 1)

	// Convert to DNSHostedZone
	var hostedZones []provider.DNSHostedZone
	for _, zone := range zones {
		zoneID := dns.NewZoneID(h.ProviderType(), zone.ID)
		domain := dns.NormalizeDomainName(zone.Name)

		hostedZone := provider.NewDNSHostedZone(
			h.ProviderType(),
			zoneID.ID,
			domain,
			zone.ID, // key
			false,   // Hetzner DNS is always public
		)

		hostedZones = append(hostedZones, hostedZone)
		log.V(1).Info("discovered zone", "zone", domain, "id", zone.ID)
	}

	log.Info("zones discovered", "count", len(hostedZones))
	return hostedZones, nil
}

// GetCustomQueryDNSFunc returns a custom DNS query function for this provider.
func (h *handler) GetCustomQueryDNSFunc(
	_ dns.ZoneInfo,
	factory utils.QueryDNSFactoryFunc,
) (provider.CustomQueryDNSFunc, error) {
	// Hetzner DNS zones are always public, use standard DNS queries
	defaultQueryFunc, err := factory()
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
		result := defaultQueryFunc.Query(ctx, setName, recordType)
		return result.RecordSet, result.Err
	}, nil
}

// ExecuteRequests executes DNS record changes.
func (h *handler) ExecuteRequests(
	ctx context.Context,
	zone provider.DNSHostedZone,
	reqs provider.ChangeRequests,
) error {
	log, _ := logr.FromContext(ctx)
	log = log.WithValues("provider", h.ProviderType(), "zone", zone.Domain())

	if len(reqs.Updates) == 0 {
		log.V(1).Info("no changes to execute")
		return nil
	}

	exec := newExecution(log, h, zone.ZoneID())

	// First, fetch existing records to get record IDs
	// Hetzner requires record IDs for updates and deletes
	h.config.RateLimiter.Accept()
	existingRecords, err := h.client.ListRecords(ctx, zone.Key())
	if err != nil {
		return fmt.Errorf("failed to list existing records: %w", err)
	}
	h.config.Metrics.AddZoneRequests(zone.ZoneID().ID, provider.MetricsRequestTypeListRecords, 1)

	// Build lookup map: (name, type) -> record
	// We need the record ID for deletions and updates
	recordMap := make(map[string][]Record)
	for _, rec := range existingRecords {
		key := makeRecordKey(rec.Name, rec.Type)
		recordMap[key] = append(recordMap[key], rec)
	}

	// Process change requests
	for recordType, update := range reqs.Updates {
		recordName := reqs.Name.DNSName

		if update.Old != nil && update.New != nil {
			// UPDATE: In Hetzner, we need to delete old and create new
			log.Info("updating record", "name", recordName, "type", recordType)

			// Delete old records
			key := makeRecordKey(recordName, string(recordType))
			if records, exists := recordMap[key]; exists {
				for _, rec := range records {
					exec.addChange(deleteRecord, reqs, update.Old, rec.ID)
				}
			}

			// Create new records
			exec.addChange(createRecord, reqs, update.New, "")
		} else if update.Old != nil {
			// DELETE
			log.Info("deleting record", "name", recordName, "type", recordType)

			key := makeRecordKey(recordName, string(recordType))
			if records, exists := recordMap[key]; exists {
				for _, rec := range records {
					exec.addChange(deleteRecord, reqs, update.Old, rec.ID)
				}
			} else {
				log.V(1).Info("record already deleted", "name", recordName, "type", recordType)
			}
		} else if update.New != nil {
			// CREATE
			log.Info("creating record", "name", recordName, "type", recordType)
			exec.addChange(createRecord, reqs, update.New, "")
		}
	}

	// Execute all changes
	return exec.submitChanges(ctx, h.config.Metrics)
}

// Release releases any resources held by this handler.
func (h *handler) Release() {
	// Nothing to release for now
}
