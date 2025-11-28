// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package hetzner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHetznerClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Hetzner Client Suite")
}

var _ = Describe("Hetzner DNS Client", func() {
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

	Context("Client initialization", func() {
		It("should create a client with default values", func() {
			client = NewClient(ClientConfig{
				Token:  "test-token",
				Logger: logr.Discard(),
			})

			Expect(client).ToNot(BeNil())
			Expect(client.baseURL).To(Equal(defaultBaseURL))
			Expect(client.token).To(Equal("test-token"))
			Expect(client.httpClient.Timeout).To(Equal(defaultTimeout))
		})

		It("should create a client with custom values", func() {
			customURL := "https://custom.example.com"
			customTimeout := 60 * time.Second

			client = NewClient(ClientConfig{
				BaseURL:     customURL,
				Token:       "custom-token",
				HTTPTimeout: customTimeout,
				Logger:      logr.Discard(),
			})

			Expect(client.baseURL).To(Equal(customURL))
			Expect(client.token).To(Equal("custom-token"))
			Expect(client.httpClient.Timeout).To(Equal(customTimeout))
		})

		It("should trim trailing slash from base URL", func() {
			client = NewClient(ClientConfig{
				BaseURL: "https://example.com/",
				Token:   "test-token",
				Logger:  logr.Discard(),
			})

			Expect(client.baseURL).To(Equal("https://example.com"))
		})
	})

	Context("Rate limit tracking", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Ratelimit-Limit", "1000")
				w.Header().Set("Ratelimit-Remaining", "500")
				w.Header().Set("Ratelimit-Reset", fmt.Sprintf("%d", time.Now().Add(1*time.Hour).Unix()))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(ZoneListResponse{
					Zones: []Zone{},
					Meta:  Meta{},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should parse rate limit headers", func() {
			_, err := client.ListZones(ctx)
			Expect(err).ToNot(HaveOccurred())

			rateInfo := client.GetRateLimitInfo()
			Expect(rateInfo.Limit).To(Equal(1000))
			Expect(rateInfo.Remaining).To(Equal(500))
			Expect(rateInfo.Reset).ToNot(BeZero())
		})
	})

	Context("ListZones", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/zones"))
				Expect(r.Header.Get("Auth-API-Token")).To(Equal("test-token"))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(ZoneListResponse{
					Zones: []Zone{
						{
							ID:   "zone1",
							Name: "example.com",
							NS:   []string{"ns1.hetzner.com", "ns2.hetzner.com"},
						},
						{
							ID:   "zone2",
							Name: "test.com",
							NS:   []string{"ns1.hetzner.com"},
						},
					},
					Meta: Meta{
						Pagination: &Pagination{
							Page:         1,
							PerPage:      100,
							LastPage:     1,
							TotalEntries: 2,
						},
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should list all zones", func() {
			zones, err := client.ListZones(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(zones).To(HaveLen(2))
			Expect(zones[0].ID).To(Equal("zone1"))
			Expect(zones[0].Name).To(Equal("example.com"))
			Expect(zones[1].ID).To(Equal("zone2"))
		})
	})

	Context("ListZones with pagination", func() {
		BeforeEach(func() {
			callCount := 0
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))

				w.WriteHeader(http.StatusOK)
				switch page {
				case 1:
					_ = json.NewEncoder(w).Encode(ZoneListResponse{
						Zones: []Zone{
							{ID: "zone1", Name: "example1.com"},
							{ID: "zone2", Name: "example2.com"},
						},
						Meta: Meta{
							Pagination: &Pagination{
								Page:         1,
								PerPage:      2,
								LastPage:     2,
								TotalEntries: 3,
							},
						},
					})
				case 2:
					_ = json.NewEncoder(w).Encode(ZoneListResponse{
						Zones: []Zone{
							{ID: "zone3", Name: "example3.com"},
						},
						Meta: Meta{
							Pagination: &Pagination{
								Page:         2,
								PerPage:      2,
								LastPage:     2,
								TotalEntries: 3,
							},
						},
					})
				}
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should handle pagination correctly", func() {
			zones, err := client.ListZones(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(zones).To(HaveLen(3))
			Expect(zones[0].ID).To(Equal("zone1"))
			Expect(zones[1].ID).To(Equal("zone2"))
			Expect(zones[2].ID).To(Equal("zone3"))
		})
	})

	Context("GetZone", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/zones/zone123"))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(ZoneGetResponse{
					Zone: Zone{
						ID:           "zone123",
						Name:         "example.com",
						RecordsCount: 42,
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should get a zone by ID", func() {
			zone, err := client.GetZone(ctx, "zone123")
			Expect(err).ToNot(HaveOccurred())
			Expect(zone.ID).To(Equal("zone123"))
			Expect(zone.Name).To(Equal("example.com"))
			Expect(zone.RecordsCount).To(Equal(42))
		})
	})

	Context("ListRecords", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/records"))
				Expect(r.URL.Query().Get("zone_id")).To(Equal("zone123"))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(RecordListResponse{
					Records: []Record{
						{
							ID:     "rec1",
							Type:   "A",
							Name:   "www",
							Value:  "192.0.2.1",
							ZoneID: "zone123",
							TTL:    3600,
						},
						{
							ID:     "rec2",
							Type:   "CNAME",
							Name:   "blog",
							Value:  "www.example.com",
							ZoneID: "zone123",
							TTL:    3600,
						},
					},
					Meta: Meta{
						Pagination: &Pagination{
							Page:         1,
							PerPage:      100,
							LastPage:     1,
							TotalEntries: 2,
						},
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should list records for a zone", func() {
			records, err := client.ListRecords(ctx, "zone123")
			Expect(err).ToNot(HaveOccurred())
			Expect(records).To(HaveLen(2))
			Expect(records[0].Type).To(Equal("A"))
			Expect(records[0].Name).To(Equal("www"))
			Expect(records[1].Type).To(Equal("CNAME"))
		})
	})

	Context("CreateRecord", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/records"))

				var req RecordCreateRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				Expect(err).ToNot(HaveOccurred())

				Expect(req.ZoneID).To(Equal("zone123"))
				Expect(req.Type).To(Equal("A"))
				Expect(req.Name).To(Equal("test"))
				Expect(req.Value).To(Equal("192.0.2.1"))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(RecordGetResponse{
					Record: Record{
						ID:     "newrec",
						Type:   req.Type,
						Name:   req.Name,
						Value:  req.Value,
						ZoneID: req.ZoneID,
						TTL:    3600,
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should create a record", func() {
			ttl := 3600
			record, err := client.CreateRecord(ctx, RecordCreateRequest{
				ZoneID: "zone123",
				Type:   "A",
				Name:   "test",
				Value:  "192.0.2.1",
				TTL:    &ttl,
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(record.ID).To(Equal("newrec"))
			Expect(record.Type).To(Equal("A"))
			Expect(record.Name).To(Equal("test"))
		})
	})

	Context("CreateRecord with CAA type", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req RecordCreateRequest
				_ = json.NewDecoder(r.Body).Decode(&req)

				// Verify spaces were removed
				Expect(req.Value).To(Equal("0issue\"letsencrypt.org\""))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(RecordGetResponse{
					Record: Record{
						ID:    "caarec",
						Type:  "CAA",
						Value: req.Value,
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should remove spaces from CAA record values", func() {
			record, err := client.CreateRecord(ctx, RecordCreateRequest{
				ZoneID: "zone123",
				Type:   "CAA",
				Name:   "@",
				Value:  "0 issue \"letsencrypt.org\"",
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(record.Value).To(Equal("0issue\"letsencrypt.org\""))
		})
	})

	Context("GetRecord", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodGet))
				Expect(r.URL.Path).To(Equal("/records/rec123"))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(RecordGetResponse{
					Record: Record{
						ID:     "rec123",
						Type:   "A",
						Name:   "www",
						Value:  "192.0.2.1",
						ZoneID: "zone123",
						TTL:    3600,
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should get a record by ID", func() {
			record, err := client.GetRecord(ctx, "rec123")
			Expect(err).ToNot(HaveOccurred())
			Expect(record.ID).To(Equal("rec123"))
			Expect(record.Type).To(Equal("A"))
			Expect(record.Name).To(Equal("www"))
		})
	})

	Context("UpdateRecord", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPut))
				Expect(r.URL.Path).To(Equal("/records/rec123"))

				var req RecordUpdateRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				Expect(err).ToNot(HaveOccurred())

				Expect(req.Value).To(Equal("192.0.2.100"))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(RecordGetResponse{
					Record: Record{
						ID:     "rec123",
						Type:   req.Type,
						Name:   req.Name,
						Value:  req.Value,
						ZoneID: req.ZoneID,
						TTL:    3600,
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should update a record", func() {
			record, err := client.UpdateRecord(ctx, "rec123", RecordUpdateRequest{
				ZoneID: "zone123",
				Type:   "A",
				Name:   "www",
				Value:  "192.0.2.100",
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(record.ID).To(Equal("rec123"))
			Expect(record.Value).To(Equal("192.0.2.100"))
		})
	})

	Context("UpdateRecord with CAA type", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req RecordUpdateRequest
				_ = json.NewDecoder(r.Body).Decode(&req)

				// Verify spaces were removed
				Expect(req.Value).To(Equal("0issue\"letsencrypt.org\""))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(RecordGetResponse{
					Record: Record{
						ID:    "caarec",
						Type:  "CAA",
						Value: req.Value,
					},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should remove spaces from CAA record values on update", func() {
			record, err := client.UpdateRecord(ctx, "caarec", RecordUpdateRequest{
				ZoneID: "zone123",
				Type:   "CAA",
				Name:   "@",
				Value:  "0 issue \"letsencrypt.org\"",
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(record.Value).To(Equal("0issue\"letsencrypt.org\""))
		})
	})

	Context("DeleteRecord", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodDelete))
				Expect(r.URL.Path).To(Equal("/records/rec123"))

				w.WriteHeader(http.StatusOK)
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should delete a record", func() {
			err := client.DeleteRecord(ctx, "rec123")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Error handling", func() {
		Context("404 Not Found", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_ = json.NewEncoder(w).Encode(ErrorResponse{
						Error: struct {
							Message string `json:"message"`
							Code    string `json:"code,omitempty"`
						}{
							Message: "Zone not found",
							Code:    "NOT_FOUND",
						},
					})
				}))

				client = NewClient(ClientConfig{
					BaseURL: server.URL,
					Token:   "test-token",
					Logger:  logr.Discard(),
				})
			})

			It("should return APIError with IsNotFound true", func() {
				_, err := client.GetZone(ctx, "invalid")
				Expect(err).To(HaveOccurred())

				apiErr, ok := err.(*APIError)
				Expect(ok).To(BeTrue())
				Expect(apiErr.IsNotFound()).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(404))
				Expect(apiErr.Message).To(Equal("Zone not found"))
			})
		})

		Context("401 Unauthorized", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(ErrorResponse{
						Error: struct {
							Message string `json:"message"`
							Code    string `json:"code,omitempty"`
						}{
							Message: "Invalid API token",
						},
					})
				}))

				client = NewClient(ClientConfig{
					BaseURL: server.URL,
					Token:   "invalid-token",
					Logger:  logr.Discard(),
				})
			})

			It("should return APIError with IsAuthError true", func() {
				_, err := client.ListZones(ctx)
				Expect(err).To(HaveOccurred())

				apiErr, ok := err.(*APIError)
				Expect(ok).To(BeTrue())
				Expect(apiErr.IsAuthError()).To(BeTrue())
				Expect(apiErr.StatusCode).To(Equal(401))
			})
		})

		Context("429 Too Many Requests", func() {
			BeforeEach(func() {
				callCount := 0
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					callCount++

					if callCount <= 2 {
						w.Header().Set("Retry-After", "1")
						w.WriteHeader(http.StatusTooManyRequests)
						_ = json.NewEncoder(w).Encode(ErrorResponse{
							Error: struct {
								Message string `json:"message"`
								Code    string `json:"code,omitempty"`
							}{
								Message: "Rate limit exceeded",
							},
						})
					} else {
						// Third call succeeds
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(ZoneListResponse{
							Zones: []Zone{{ID: "zone1"}},
							Meta:  Meta{},
						})
					}
				}))

				client = NewClient(ClientConfig{
					BaseURL: server.URL,
					Token:   "test-token",
					Logger:  logr.Discard(),
				})
			})

			It("should retry on rate limit error", func() {
				zones, err := client.ListZones(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(zones).To(HaveLen(1))
			})
		})

		Context("500 Internal Server Error", func() {
			BeforeEach(func() {
				callCount := 0
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					callCount++

					if callCount == 1 {
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte("Internal server error"))
					} else {
						// Second call succeeds
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(ZoneListResponse{
							Zones: []Zone{{ID: "zone1"}},
							Meta:  Meta{},
						})
					}
				}))

				client = NewClient(ClientConfig{
					BaseURL: server.URL,
					Token:   "test-token",
					Logger:  logr.Discard(),
				})
			})

			It("should retry on server error", func() {
				zones, err := client.ListZones(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(zones).To(HaveLen(1))
			})
		})
	})

	Context("BulkCreateRecords", func() {
		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal(http.MethodPost))
				Expect(r.URL.Path).To(Equal("/records/bulk"))

				var req RecordBulkCreateRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				Expect(err).ToNot(HaveOccurred())
				Expect(req.Records).To(HaveLen(2))

				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(RecordBulkCreateResponse{
					Records: []Record{
						{ID: "rec1", Type: "A"},
						{ID: "rec2", Type: "A"},
					},
					ValidRecords:   req.Records,
					InvalidRecords: []RecordCreateRequest{},
				})
			}))

			client = NewClient(ClientConfig{
				BaseURL: server.URL,
				Token:   "test-token",
				Logger:  logr.Discard(),
			})
		})

		It("should bulk create records", func() {
			resp, err := client.BulkCreateRecords(ctx, []RecordCreateRequest{
				{ZoneID: "zone123", Type: "A", Name: "test1", Value: "192.0.2.1"},
				{ZoneID: "zone123", Type: "A", Name: "test2", Value: "192.0.2.2"},
			})

			Expect(err).ToNot(HaveOccurred())
			Expect(resp.Records).To(HaveLen(2))
			Expect(resp.InvalidRecords).To(BeEmpty())
		})
	})
})
