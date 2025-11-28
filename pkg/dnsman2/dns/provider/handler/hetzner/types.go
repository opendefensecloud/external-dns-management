// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package hetzner

import "time"

// Zone represents a DNS zone in Hetzner DNS.
type Zone struct {
	ID              string    `json:"id"`
	Created         time.Time `json:"created"`
	Modified        time.Time `json:"modified"`
	LegacyDNSHost   string    `json:"legacy_dns_host"`
	LegacyNS        []string  `json:"legacy_ns"`
	Name            string    `json:"name"`
	NS              []string  `json:"ns"`
	Owner           string    `json:"owner"`
	Paused          bool      `json:"paused"`
	Permission      string    `json:"permission"`
	Project         string    `json:"project"`
	Registrar       string    `json:"registrar"`
	Status          string    `json:"status"`
	TTL             int       `json:"ttl"`
	Verified        time.Time `json:"verified"`
	RecordsCount    int       `json:"records_count"`
	IsSecondaryDNS  bool      `json:"is_secondary_dns"`
	TxtVerification struct {
		Name  string `json:"name"`
		Token string `json:"token"`
	} `json:"txt_verification"`
}

// ZoneListResponse is the response for listing zones.
type ZoneListResponse struct {
	Zones []Zone `json:"zones"`
	Meta  Meta   `json:"meta"`
}

// ZoneGetResponse is the response for getting a single zone.
type ZoneGetResponse struct {
	Zone Zone `json:"zone"`
}

// Record represents a DNS record in Hetzner DNS.
type Record struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
	ZoneID   string    `json:"zone_id"`
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	TTL      int       `json:"ttl"`
}

// RecordListResponse is the response for listing records.
type RecordListResponse struct {
	Records []Record `json:"records"`
	Meta    Meta     `json:"meta"`
}

// RecordGetResponse is the response for getting a single record.
type RecordGetResponse struct {
	Record Record `json:"record"`
}

// RecordCreateRequest is the request for creating a record.
type RecordCreateRequest struct {
	ZoneID string `json:"zone_id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	TTL    *int   `json:"ttl,omitempty"`
}

// RecordUpdateRequest is the request for updating a record.
type RecordUpdateRequest struct {
	ZoneID string `json:"zone_id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Value  string `json:"value"`
	TTL    *int   `json:"ttl,omitempty"`
}

// RecordBulkCreateRequest is the request for bulk creating records.
type RecordBulkCreateRequest struct {
	Records []RecordCreateRequest `json:"records"`
}

// RecordBulkCreateResponse is the response for bulk creating records.
type RecordBulkCreateResponse struct {
	Records        []Record              `json:"records"`
	ValidRecords   []RecordCreateRequest `json:"valid_records"`
	InvalidRecords []RecordCreateRequest `json:"invalid_records"`
}

// Meta contains metadata from API responses.
type Meta struct {
	Pagination *Pagination `json:"pagination,omitempty"`
}

// Pagination contains pagination information.
type Pagination struct {
	Page         int `json:"page"`
	PerPage      int `json:"per_page"`
	LastPage     int `json:"last_page"`
	TotalEntries int `json:"total_entries"`
}

// ErrorResponse represents an error response from the API.
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// RateLimitInfo contains rate limiting information from response headers.
type RateLimitInfo struct {
	Limit     int       // Total requests allowed in current window
	Remaining int       // Remaining requests in current window
	Reset     time.Time // When the limit resets
}
