// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package hetzner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

const (
	defaultBaseURL = "https://dns.hetzner.com/api/v1"
	defaultTimeout = 30 * time.Second
	userAgent      = "external-dns-management/hetzner-dns"

	// API endpoints
	pathZones   = "/zones"
	pathRecords = "/records"

	// Retry configuration
	maxRetries        = 3
	initialBackoff    = 1 * time.Second
	maxBackoff        = 30 * time.Second
	backoffMultiplier = 2.0
)

// Client is an HTTP client for the Hetzner DNS API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	logger     logr.Logger

	// Rate limit tracking
	rateLimitMu sync.RWMutex
	rateLimit   *RateLimitInfo
}

// ClientConfig contains configuration for the Hetzner DNS client.
type ClientConfig struct {
	BaseURL     string
	Token       string
	HTTPTimeout time.Duration
	Logger      logr.Logger
}

// NewClient creates a new Hetzner DNS API client.
func NewClient(config ClientConfig) *Client {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = defaultTimeout
	}

	// Trim trailing slash from base URL
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")

	return &Client{
		baseURL: config.BaseURL,
		token:   config.Token,
		httpClient: &http.Client{
			Timeout: config.HTTPTimeout,
		},
		logger: config.Logger,
		rateLimit: &RateLimitInfo{
			Limit:     -1,
			Remaining: -1,
		},
	}
}

// doRequest performs an HTTP request with retry logic and rate limit handling.
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff duration
			backoff := time.Duration(math.Pow(backoffMultiplier, float64(attempt-1))) * initialBackoff
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			c.logger.V(1).Info("retrying request",
				"attempt", attempt,
				"backoff", backoff,
				"error", lastErr)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		err := c.doRequestOnce(ctx, method, path, body, result)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !shouldRetry(err) {
			return err
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doRequestOnce performs a single HTTP request.
func (c *Client) doRequestOnce(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	// Build request URL
	url := c.baseURL + path

	// Marshal request body if provided
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Auth-API-Token", c.token)
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Update rate limit info from headers
	c.updateRateLimitInfo(resp.Header)

	// Check for errors
	if resp.StatusCode >= 400 {
		return c.handleErrorResponse(resp.StatusCode, respBody)
	}

	// Unmarshal response if result is provided
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// updateRateLimitInfo updates the rate limit information from response headers.
func (c *Client) updateRateLimitInfo(headers http.Header) {
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()

	if limitStr := headers.Get("Ratelimit-Limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			c.rateLimit.Limit = limit
		}
	}

	if remainingStr := headers.Get("Ratelimit-Remaining"); remainingStr != "" {
		if remaining, err := strconv.Atoi(remainingStr); err == nil {
			c.rateLimit.Remaining = remaining
		}
	}

	if resetStr := headers.Get("Ratelimit-Reset"); resetStr != "" {
		if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			c.rateLimit.Reset = time.Unix(resetUnix, 0)
		}
	}
}

// GetRateLimitInfo returns the current rate limit information.
func (c *Client) GetRateLimitInfo() RateLimitInfo {
	c.rateLimitMu.RLock()
	defer c.rateLimitMu.RUnlock()
	return *c.rateLimit
}

// handleErrorResponse parses and returns an error from an HTTP error response.
func (c *Client) handleErrorResponse(statusCode int, body []byte) error {
	var errResp ErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return &APIError{
			StatusCode: statusCode,
			Message:    errResp.Error.Message,
			Code:       errResp.Error.Code,
		}
	}

	// Fallback error message
	return &APIError{
		StatusCode: statusCode,
		Message:    string(body),
	}
}

// ListZones lists all DNS zones.
func (c *Client) ListZones(ctx context.Context) ([]Zone, error) {
	var allZones []Zone
	page := 1
	perPage := 100

	for {
		path := fmt.Sprintf("%s?page=%d&per_page=%d", pathZones, page, perPage)

		var resp ZoneListResponse
		if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}

		allZones = append(allZones, resp.Zones...)

		// Check if we've retrieved all zones
		if resp.Meta.Pagination == nil || page >= resp.Meta.Pagination.LastPage {
			break
		}

		page++
	}

	return allZones, nil
}

// GetZone retrieves a single zone by ID.
func (c *Client) GetZone(ctx context.Context, zoneID string) (*Zone, error) {
	path := fmt.Sprintf("%s/%s", pathZones, zoneID)

	var resp ZoneGetResponse
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp.Zone, nil
}

// ListRecords lists all DNS records for a zone.
func (c *Client) ListRecords(ctx context.Context, zoneID string) ([]Record, error) {
	var allRecords []Record
	page := 1
	perPage := 100

	for {
		path := fmt.Sprintf("%s?zone_id=%s&page=%d&per_page=%d", pathRecords, zoneID, page, perPage)

		var resp RecordListResponse
		if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}

		allRecords = append(allRecords, resp.Records...)

		// Check if we've retrieved all records
		if resp.Meta.Pagination == nil || page >= resp.Meta.Pagination.LastPage {
			break
		}

		page++
	}

	return allRecords, nil
}

// GetRecord retrieves a single record by ID.
func (c *Client) GetRecord(ctx context.Context, recordID string) (*Record, error) {
	path := fmt.Sprintf("%s/%s", pathRecords, recordID)

	var resp RecordGetResponse
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}

	return &resp.Record, nil
}

// CreateRecord creates a new DNS record.
func (c *Client) CreateRecord(ctx context.Context, req RecordCreateRequest) (*Record, error) {
	// Handle CAA record quirk: remove spaces
	if req.Type == "CAA" {
		req.Value = strings.ReplaceAll(req.Value, " ", "")
	}

	var resp RecordGetResponse
	if err := c.doRequest(ctx, http.MethodPost, pathRecords, req, &resp); err != nil {
		return nil, err
	}

	return &resp.Record, nil
}

// UpdateRecord updates an existing DNS record.
func (c *Client) UpdateRecord(ctx context.Context, recordID string, req RecordUpdateRequest) (*Record, error) {
	// Handle CAA record quirk: remove spaces
	if req.Type == "CAA" {
		req.Value = strings.ReplaceAll(req.Value, " ", "")
	}

	path := fmt.Sprintf("%s/%s", pathRecords, recordID)

	var resp RecordGetResponse
	if err := c.doRequest(ctx, http.MethodPut, path, req, &resp); err != nil {
		return nil, err
	}

	return &resp.Record, nil
}

// DeleteRecord deletes a DNS record.
func (c *Client) DeleteRecord(ctx context.Context, recordID string) error {
	path := fmt.Sprintf("%s/%s", pathRecords, recordID)

	return c.doRequest(ctx, http.MethodDelete, path, nil, nil)
}

// BulkCreateRecords creates multiple DNS records in a single request.
func (c *Client) BulkCreateRecords(ctx context.Context, records []RecordCreateRequest) (*RecordBulkCreateResponse, error) {
	// Handle CAA record quirk for all records
	for i := range records {
		if records[i].Type == "CAA" {
			records[i].Value = strings.ReplaceAll(records[i].Value, " ", "")
		}
	}

	req := RecordBulkCreateRequest{Records: records}
	path := fmt.Sprintf("%s/bulk", pathRecords)

	var resp RecordBulkCreateResponse
	if err := c.doRequest(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// APIError represents an error from the Hetzner DNS API.
type APIError struct {
	StatusCode int
	Message    string
	Code       string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("hetzner dns api error (status %d, code %s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("hetzner dns api error (status %d): %s", e.StatusCode, e.Message)
}

// IsNotFound returns true if the error is a 404 Not Found error.
func (e *APIError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// IsRateLimitError returns true if the error is a 429 Too Many Requests error.
func (e *APIError) IsRateLimitError() bool {
	return e.StatusCode == http.StatusTooManyRequests
}

// IsAuthError returns true if the error is a 401 Unauthorized error.
func (e *APIError) IsAuthError() bool {
	return e.StatusCode == http.StatusUnauthorized
}

// shouldRetry determines if a request should be retried based on the error.
func shouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Retry on rate limit errors
	if apiErr, ok := err.(*APIError); ok {
		if apiErr.IsRateLimitError() {
			return true
		}
		// Retry on server errors (5xx)
		if apiErr.StatusCode >= 500 {
			return true
		}
		// Don't retry on client errors (4xx except 429)
		return false
	}

	// Retry on network/timeout errors
	return true
}
