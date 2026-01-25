// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

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
	defaultBaseURL = "https://api.bunny.net"
	defaultTimeout = 30 * time.Second
	userAgent      = "external-dns-management/bunny-dns"

	// API endpoints
	pathZones = "/dnszone"

	// Retry configuration
	maxRetries        = 3
	initialBackoff    = 1 * time.Second
	maxBackoff        = 30 * time.Second
	backoffMultiplier = 2.0
)

// Client is an HTTP client for the Bunny DNS API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     logr.Logger

	// Rate limit tracking
	rateLimitMu sync.RWMutex
	rateLimit   *RateLimitInfo
}

// ClientConfig contains configuration for the Bunny DNS client.
type ClientConfig struct {
	BaseURL     string
	APIKey      string
	HTTPTimeout time.Duration
	Logger      logr.Logger
}

// NewClient creates a new Bunny DNS API client.
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
		apiKey:  config.APIKey,
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
	req.Header.Set("AccessKey", c.apiKey)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
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

	if limitStr := headers.Get("X-RateLimit-Limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			c.rateLimit.Limit = limit
		}
	}

	if remainingStr := headers.Get("X-RateLimit-Remaining"); remainingStr != "" {
		if remaining, err := strconv.Atoi(remainingStr); err == nil {
			c.rateLimit.Remaining = remaining
		}
	}

	if resetStr := headers.Get("X-RateLimit-Reset"); resetStr != "" {
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
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Message != "" {
		return &APIError{
			StatusCode: statusCode,
			Message:    errResp.Message,
			ErrorKey:   errResp.ErrorKey,
			Field:      errResp.Field,
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
	perPage := 1000

	for {
		path := fmt.Sprintf("%s?page=%d&perPage=%d", pathZones, page, perPage)

		var resp ZoneListResponse
		if err := c.doRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}

		allZones = append(allZones, resp.Items...)

		// Check if we've retrieved all zones
		if !resp.HasMoreItems {
			break
		}

		page++
	}

	return allZones, nil
}

// GetZone retrieves a single zone by ID.
func (c *Client) GetZone(ctx context.Context, zoneID int64) (*Zone, error) {
	path := fmt.Sprintf("%s/%d", pathZones, zoneID)

	var zone Zone
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &zone); err != nil {
		return nil, err
	}

	return &zone, nil
}

// ListRecords lists all DNS records for a zone.
// Bunny DNS returns records as part of the zone response.
func (c *Client) ListRecords(ctx context.Context, zoneID int64) ([]Record, error) {
	zone, err := c.GetZone(ctx, zoneID)
	if err != nil {
		return nil, err
	}

	return zone.Records, nil
}

// CreateRecord creates a new DNS record.
func (c *Client) CreateRecord(ctx context.Context, zoneID int64, req RecordCreateRequest) (*Record, error) {
	path := fmt.Sprintf("%s/%d/records", pathZones, zoneID)

	var record Record
	if err := c.doRequest(ctx, http.MethodPut, path, req, &record); err != nil {
		return nil, err
	}

	return &record, nil
}

// UpdateRecord updates an existing DNS record.
func (c *Client) UpdateRecord(ctx context.Context, zoneID int64, recordID int64, req RecordUpdateRequest) error {
	path := fmt.Sprintf("%s/%d/records/%d", pathZones, zoneID, recordID)

	return c.doRequest(ctx, http.MethodPost, path, req, nil)
}

// DeleteRecord deletes a DNS record.
func (c *Client) DeleteRecord(ctx context.Context, zoneID int64, recordID int64) error {
	path := fmt.Sprintf("%s/%d/records/%d", pathZones, zoneID, recordID)

	return c.doRequest(ctx, http.MethodDelete, path, nil, nil)
}

// APIError represents an error from the Bunny DNS API.
type APIError struct {
	StatusCode int
	Message    string
	ErrorKey   string
	Field      string
}

func (e *APIError) Error() string {
	if e.ErrorKey != "" {
		return fmt.Sprintf("bunny dns api error (status %d, key %s): %s", e.StatusCode, e.ErrorKey, e.Message)
	}
	return fmt.Sprintf("bunny dns api error (status %d): %s", e.StatusCode, e.Message)
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

// IsForbiddenError returns true if the error is a 403 Forbidden error.
func (e *APIError) IsForbiddenError() bool {
	return e.StatusCode == http.StatusForbidden
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
