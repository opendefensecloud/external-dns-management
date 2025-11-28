// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package hetzner

import (
	"context"
	"fmt"
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
	log     logr.Logger
	handler *handler
	zoneID  dns.ZoneID
	changes []*change
}

type change struct {
	action   changeAction
	req      provider.ChangeRequests
	rs       *dns.RecordSet
	recordID string // For deletes (Hetzner requires record ID)
}

func newExecution(log logr.Logger, h *handler, zoneID dns.ZoneID) *execution {
	return &execution{
		log:     log.WithValues("zone", zoneID.ID),
		handler: h,
		zoneID:  zoneID,
		changes: make([]*change, 0),
	}
}

func (ex *execution) addChange(action changeAction, reqs provider.ChangeRequests, rs *dns.RecordSet, recordID string) {
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
	zoneID := ex.zoneID.ID
	recordName := ch.req.Name.DNSName

	// Remove trailing dot from DNS name for Hetzner API
	recordName = strings.TrimSuffix(recordName, ".")

	// Create a record for each target (e.g., multiple A records with different IPs)
	for _, target := range ch.rs.Records {
		ttl := int(ch.rs.TTL)

		req := RecordCreateRequest{
			ZoneID: zoneID,
			Type:   string(ch.rs.Type),
			Name:   recordName,
			Value:  target.Value,
			TTL:    &ttl,
		}

		// Handle CAA record quirk: Hetzner doesn't allow spaces between parameters
		if ch.rs.Type == "CAA" {
			req.Value = strings.ReplaceAll(req.Value, " ", "")
		}

		ex.log.V(1).Info("creating record",
			"name", recordName,
			"type", req.Type,
			"value", req.Value,
			"ttl", ttl)

		_, err := ex.handler.client.CreateRecord(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to create record %s[%s]: %w", recordName, req.Type, err)
		}

		metrics.AddZoneRequests(zoneID, provider.MetricsRequestTypeCreateRecords, 1)
	}

	return nil
}

func (ex *execution) executeDelete(ctx context.Context, ch *change, metrics provider.Metrics) error {
	if ch.recordID == "" {
		return fmt.Errorf("missing record ID for deletion")
	}

	recordName := ch.req.Name.DNSName

	ex.log.V(1).Info("deleting record",
		"id", ch.recordID,
		"name", recordName,
		"type", ch.rs.Type)

	err := ex.handler.client.DeleteRecord(ctx, ch.recordID)
	if err != nil {
		// Check if it's a "not found" error - this is OK for idempotency
		if apiErr, ok := err.(*APIError); ok && apiErr.IsNotFound() {
			ex.log.V(1).Info("record already deleted", "id", ch.recordID)
			return nil
		}
		return fmt.Errorf("failed to delete record %s[%s]: %w", recordName, ch.rs.Type, err)
	}

	metrics.AddZoneRequests(ex.zoneID.ID, provider.MetricsRequestTypeDeleteRecords, 1)
	return nil
}

func makeRecordKey(name, recordType string) string {
	// Normalize name by removing trailing dot for consistent lookup
	name = strings.TrimSuffix(name, ".")
	return fmt.Sprintf("%s:%s", name, recordType)
}
