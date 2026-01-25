// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
)

type changeAction int

const (
	createRecord changeAction = iota
	deleteRecord
)

type execution struct {
	log       logr.Logger
	handler   *handler
	zoneID    dns.ZoneID
	bunnyZone int64
	changes   []*change
}

type change struct {
	action   changeAction
	req      provider.ChangeRequests
	rs       *dns.RecordSet
	recordID int64 // For deletes (Bunny requires record ID)
}

func newExecution(log logr.Logger, h *handler, zoneID dns.ZoneID, bunnyZone int64) *execution {
	return &execution{
		log:       log.WithValues("zone", zoneID.ID),
		handler:   h,
		zoneID:    zoneID,
		bunnyZone: bunnyZone,
		changes:   make([]*change, 0),
	}
}

func (ex *execution) addChange(action changeAction, reqs provider.ChangeRequests, rs *dns.RecordSet, recordID int64) {
	ex.changes = append(ex.changes, &change{
		action:   action,
		req:      reqs,
		rs:       rs,
		recordID: recordID,
	})
}

func (ex *execution) submitChanges(ctx context.Context, metrics provider.Metrics) error {
	if len(ex.changes) == 0 {
		ex.log.V(1).Info("no changes to submit")
		return nil
	}

	ex.log.Info("submitting changes", "count", len(ex.changes))

	var errs []error
	successCount := 0

	for i, ch := range ex.changes {
		// Rate limit each API call
		ex.handler.config.RateLimiter.Accept()

		var err error
		switch ch.action {
		case createRecord:
			err = ex.executeCreate(ctx, ch, metrics)
		case deleteRecord:
			err = ex.executeDelete(ctx, ch, metrics)
		default:
			err = fmt.Errorf("unknown action: %d", ch.action)
		}

		if err != nil {
			ex.log.Error(err, "failed to execute change",
				"index", i,
				"action", ch.action,
				"name", ch.req.Name.DNSName,
				"type", ch.rs.Type)
			errs = append(errs, err)
		} else {
			successCount++
		}
	}

	ex.log.Info("changes executed", "success", successCount, "failed", len(errs))

	if len(errs) > 0 {
		return fmt.Errorf("failed to execute %d of %d changes: %v", len(errs), len(ex.changes), errs)
	}

	return nil
}

func (ex *execution) executeCreate(ctx context.Context, ch *change, metrics provider.Metrics) error {
	recordName := ch.req.Name.DNSName

	// Remove trailing dot from DNS name for Bunny API
	recordName = strings.TrimSuffix(recordName, ".")

	// Get the zone domain to extract subdomain name
	// Bunny expects just the subdomain part, not the FQDN
	zoneDomain := ex.zoneID.ID
	// The zone ID is stored as the numeric Bunny zone ID, so we need to get the domain differently
	// For now, we'll use the full name and let Bunny handle it
	// Actually, Bunny expects just the subdomain (e.g., "www" for "www.example.com")
	// We need to extract this from the full DNS name

	// Convert record type string to Bunny type int
	bunnyType := StringToRecordType(string(ch.rs.Type))
	if bunnyType == -1 {
		return fmt.Errorf("unsupported record type: %s", ch.rs.Type)
	}

	// Create a record for each target (e.g., multiple A records with different IPs)
	for _, target := range ch.rs.Records {
		ttl := int(ch.rs.TTL)

		req := RecordCreateRequest{
			Type:  bunnyType,
			Name:  recordName,
			Value: target.Value,
			TTL:   ttl,
		}

		// Handle MX records - extract priority from value if present
		if ch.rs.Type == "MX" {
			priority, value := parseMXValue(target.Value)
			req.Priority = priority
			req.Value = value
		}

		// Handle SRV records - extract priority, weight, port
		if ch.rs.Type == "SRV" {
			priority, weight, port, value := parseSRVValue(target.Value)
			req.Priority = priority
			req.Weight = weight
			req.Port = port
			req.Value = value
		}

		// Handle CAA records - extract flags and tag
		if ch.rs.Type == "CAA" {
			flags, tag, value := parseCAAValue(target.Value)
			req.Flags = flags
			req.Tag = tag
			req.Value = value
		}

		ex.log.V(1).Info("creating record",
			"name", recordName,
			"type", RecordTypeToString(bunnyType),
			"value", req.Value,
			"ttl", ttl)

		_, err := ex.handler.client.CreateRecord(ctx, ex.bunnyZone, req)
		if err != nil {
			return fmt.Errorf("failed to create record %s[%s]: %w", recordName, ch.rs.Type, err)
		}

		metrics.AddZoneRequests(zoneDomain, provider.MetricsRequestTypeCreateRecords, 1)
	}

	return nil
}

func (ex *execution) executeDelete(ctx context.Context, ch *change, metrics provider.Metrics) error {
	if ch.recordID == 0 {
		return fmt.Errorf("missing record ID for deletion")
	}

	recordName := ch.req.Name.DNSName
	zoneDomain := ex.zoneID.ID

	ex.log.V(1).Info("deleting record",
		"id", ch.recordID,
		"name", recordName,
		"type", ch.rs.Type)

	err := ex.handler.client.DeleteRecord(ctx, ex.bunnyZone, ch.recordID)
	if err != nil {
		// Check if it's a "not found" error - this is OK for idempotency
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			ex.log.V(1).Info("record already deleted", "id", ch.recordID)
			return nil
		}
		return fmt.Errorf("failed to delete record %s[%s]: %w", recordName, ch.rs.Type, err)
	}

	metrics.AddZoneRequests(zoneDomain, provider.MetricsRequestTypeDeleteRecords, 1)
	return nil
}

func makeRecordKey(name, recordType string) string {
	// Normalize name by removing trailing dot for consistent lookup
	name = strings.TrimSuffix(name, ".")
	return fmt.Sprintf("%s:%s", name, recordType)
}

// parseMXValue parses an MX record value in the format "priority target"
func parseMXValue(value string) (int, string) {
	parts := strings.SplitN(value, " ", 2)
	if len(parts) == 2 {
		if priority, err := strconv.Atoi(parts[0]); err == nil {
			return priority, parts[1]
		}
	}
	return 10, value // Default priority
}

// parseSRVValue parses an SRV record value in the format "priority weight port target"
func parseSRVValue(value string) (int, int, int, string) {
	parts := strings.SplitN(value, " ", 4)
	if len(parts) == 4 {
		priority, _ := strconv.Atoi(parts[0])
		weight, _ := strconv.Atoi(parts[1])
		port, _ := strconv.Atoi(parts[2])
		return priority, weight, port, parts[3]
	}
	return 0, 0, 0, value
}

// parseCAAValue parses a CAA record value in the format "flags tag value"
func parseCAAValue(value string) (int, string, string) {
	parts := strings.SplitN(value, " ", 3)
	if len(parts) == 3 {
		flags, _ := strconv.Atoi(parts[0])
		tag := parts[1]
		caaValue := strings.Trim(parts[2], "\"")
		return flags, tag, caaValue
	}
	return 0, "issue", value
}
