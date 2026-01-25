// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("Client", func() {
	var (
		server *httptest.Server
		client *Client
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("NewClient", func() {
		It("should create a client with default settings", func() {
			c := NewClient(ClientConfig{
				APIKey: "test-api-key",
				Logger: log.Log,
			})
			Expect(c.baseURL).To(Equal(defaultBaseURL))
			Expect(c.apiKey).To(Equal("test-api-key"))
		})

		It("should create a client with custom settings", func() {
			c := NewClient(ClientConfig{
				BaseURL:     "https://custom.api.bunny.net",
				APIKey:      "test-api-key",
				HTTPTimeout: 60 * time.Second,
				Logger:      log.Log,
			})
			Expect(c.baseURL).To(Equal("https://custom.api.bunny.net"))
			Expect(c.apiKey).To(Equal("test-api-key"))
		})

		It("should trim trailing slash from base URL", func() {
			c := NewClient(ClientConfig{
				BaseURL: "https://api.bunny.net/",
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})
			Expect(c.baseURL).To(Equal("https://api.bunny.net"))
		})
	})

	Describe("ListZones", func() {
		It("should list zones successfully", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/dnszone"))
				Expect(r.Header.Get("AccessKey")).To(Equal("test-api-key"))

				resp := ZoneListResponse{
					Items: []Zone{
						{ID: 1, Domain: "example.com"},
						{ID: 2, Domain: "example.org"},
					},
					CurrentPage:  1,
					TotalItems:   2,
					HasMoreItems: false,
				}
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(resp)
				Expect(err).ToNot(HaveOccurred())
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})

			zones, err := client.ListZones(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(zones).To(HaveLen(2))
			Expect(zones[0].Domain).To(Equal("example.com"))
			Expect(zones[1].Domain).To(Equal("example.org"))
		})

		It("should handle pagination", func() {
			callCount := 0
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				var resp ZoneListResponse
				if callCount == 1 {
					resp = ZoneListResponse{
						Items:        []Zone{{ID: 1, Domain: "example.com"}},
						CurrentPage:  1,
						TotalItems:   2,
						HasMoreItems: true,
					}
				} else {
					resp = ZoneListResponse{
						Items:        []Zone{{ID: 2, Domain: "example.org"}},
						CurrentPage:  2,
						TotalItems:   2,
						HasMoreItems: false,
					}
				}
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(resp)
				Expect(err).ToNot(HaveOccurred())
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})

			zones, err := client.ListZones(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(zones).To(HaveLen(2))
			Expect(callCount).To(Equal(2))
		})

		It("should handle authentication errors", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				resp := ErrorResponse{Message: "Invalid API key"}
				err := json.NewEncoder(w).Encode(resp)
				Expect(err).ToNot(HaveOccurred())
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "invalid-key",
				Logger:  log.Log,
			})

			_, err := client.ListZones(ctx)
			Expect(err).To(HaveOccurred())
			apiErr, ok := err.(*APIError)
			Expect(ok).To(BeTrue())
			Expect(apiErr.IsAuthError()).To(BeTrue())
		})
	})

	Describe("GetZone", func() {
		It("should get a zone successfully", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/dnszone/123"))

				zone := Zone{
					ID:     123,
					Domain: "example.com",
					Records: []Record{
						{ID: 1, Type: RecordTypeA, Name: "www", Value: "1.2.3.4", TTL: 300},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(zone)
				Expect(err).ToNot(HaveOccurred())
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})

			zone, err := client.GetZone(ctx, 123)
			Expect(err).ToNot(HaveOccurred())
			Expect(zone.Domain).To(Equal("example.com"))
			Expect(zone.Records).To(HaveLen(1))
		})

		It("should handle not found errors", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				resp := ErrorResponse{Message: "Zone not found"}
				err := json.NewEncoder(w).Encode(resp)
				Expect(err).ToNot(HaveOccurred())
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})

			_, err := client.GetZone(ctx, 999)
			Expect(err).To(HaveOccurred())
			apiErr, ok := err.(*APIError)
			Expect(ok).To(BeTrue())
			Expect(apiErr.IsNotFound()).To(BeTrue())
		})
	})

	Describe("CreateRecord", func() {
		It("should create a record successfully", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPut))
				Expect(r.URL.Path).To(Equal("/dnszone/123/records"))

				var req RecordCreateRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				Expect(err).ToNot(HaveOccurred())
				Expect(req.Type).To(Equal(RecordTypeA))
				Expect(req.Name).To(Equal("www"))
				Expect(req.Value).To(Equal("1.2.3.4"))
				Expect(req.TTL).To(Equal(300))

				record := Record{
					ID:    1,
					Type:  req.Type,
					Name:  req.Name,
					Value: req.Value,
					TTL:   req.TTL,
				}
				w.Header().Set("Content-Type", "application/json")
				err = json.NewEncoder(w).Encode(record)
				Expect(err).ToNot(HaveOccurred())
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})

			req := RecordCreateRequest{
				Type:  RecordTypeA,
				Name:  "www",
				Value: "1.2.3.4",
				TTL:   300,
			}
			record, err := client.CreateRecord(ctx, 123, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(record.ID).To(Equal(int64(1)))
			Expect(record.Value).To(Equal("1.2.3.4"))
		})
	})

	Describe("DeleteRecord", func() {
		It("should delete a record successfully", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodDelete))
				Expect(r.URL.Path).To(Equal("/dnszone/123/records/456"))
				w.WriteHeader(http.StatusOK)
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})

			err := client.DeleteRecord(ctx, 123, 456)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Rate limiting", func() {
		It("should track rate limit headers", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-RateLimit-Limit", "100")
				w.Header().Set("X-RateLimit-Remaining", "99")
				w.Header().Set("X-RateLimit-Reset", "1700000000")

				resp := ZoneListResponse{Items: []Zone{}, HasMoreItems: false}
				w.Header().Set("Content-Type", "application/json")
				err := json.NewEncoder(w).Encode(resp)
				Expect(err).ToNot(HaveOccurred())
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				APIKey:  "test-api-key",
				Logger:  log.Log,
			})

			_, err := client.ListZones(ctx)
			Expect(err).ToNot(HaveOccurred())

			rateLimit := client.GetRateLimitInfo()
			Expect(rateLimit.Limit).To(Equal(100))
			Expect(rateLimit.Remaining).To(Equal(99))
		})
	})

	Describe("APIError", func() {
		It("should format error messages correctly", func() {
			err := &APIError{
				StatusCode: 400,
				Message:    "Bad request",
				ErrorKey:   "INVALID_INPUT",
			}
			Expect(err.Error()).To(ContainSubstring("400"))
			Expect(err.Error()).To(ContainSubstring("INVALID_INPUT"))
			Expect(err.Error()).To(ContainSubstring("Bad request"))
		})

		It("should identify error types correctly", func() {
			notFoundErr := &APIError{StatusCode: 404}
			Expect(notFoundErr.IsNotFound()).To(BeTrue())
			Expect(notFoundErr.IsAuthError()).To(BeFalse())

			authErr := &APIError{StatusCode: 401}
			Expect(authErr.IsAuthError()).To(BeTrue())
			Expect(authErr.IsNotFound()).To(BeFalse())

			rateLimitErr := &APIError{StatusCode: 429}
			Expect(rateLimitErr.IsRateLimitError()).To(BeTrue())

			forbiddenErr := &APIError{StatusCode: 403}
			Expect(forbiddenErr.IsForbiddenError()).To(BeTrue())
		})
	})
})
