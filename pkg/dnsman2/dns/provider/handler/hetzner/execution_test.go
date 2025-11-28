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
	"strings"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("Execution", func() {
	var (
		ctx     context.Context
		server  *httptest.Server
		handler provider.DNSHandler
		metrics *mockMetrics
		limiter *mockRateLimiter
		zone    provider.DNSHostedZone
	)

	BeforeEach(func() {
		ctx = context.Background()
		metrics = &mockMetrics{}
		limiter = &mockRateLimiter{}

		// Create a test zone
		zoneID := dns.NewZoneID("hetzner-dns", "zone123")
		zone = provider.NewDNSHostedZone("hetzner-dns", zoneID.ID, "example.com.", "zone123", false)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("ExecuteRequests", func() {
		Context("CREATE operations", func() {
			var recordsCreated []RecordCreateRequest

			BeforeEach(func() {
				recordsCreated = make([]RecordCreateRequest, 0)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/records"):
						// ListRecords - return empty list for create scenario
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(RecordListResponse{
							Records: []Record{},
							Meta:    Meta{},
						})

					case r.Method == http.MethodPost && r.URL.Path == "/records":
						// CreateRecord
						var req RecordCreateRequest
						_ = json.NewDecoder(r.Body).Decode(&req)
						recordsCreated = append(recordsCreated, req)

						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(RecordGetResponse{
							Record: Record{
								ID:     fmt.Sprintf("rec%d", len(recordsCreated)),
								ZoneID: req.ZoneID,
								Type:   req.Type,
								Name:   req.Name,
								Value:  req.Value,
								TTL:    *req.TTL,
							},
						})

					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := &provider.DNSHandlerConfig{
					Properties: utils.Properties{
						"HETZNER_API_TOKEN": "test-token",
						"HETZNER_ENDPOINT":  server.URL,
					},
					Log:         logr.Discard(),
					Metrics:     metrics,
					RateLimiter: limiter,
				}

				var err error
				handler, err = NewHandler(config)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should create a single A record", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("A"): {
							New: &dns.RecordSet{
								Type: "A",
								TTL:  300,
								Records: []*dns.Record{
									{Value: "192.0.2.1"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())

				Expect(recordsCreated).To(HaveLen(1))
				Expect(recordsCreated[0].Name).To(Equal("test.example.com"))
				Expect(recordsCreated[0].Type).To(Equal("A"))
				Expect(recordsCreated[0].Value).To(Equal("192.0.2.1"))
				Expect(*recordsCreated[0].TTL).To(Equal(300))
			})

			It("should create multiple A records with different values", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("A"): {
							New: &dns.RecordSet{
								Type: "A",
								TTL:  600,
								Records: []*dns.Record{
									{Value: "192.0.2.1"},
									{Value: "192.0.2.2"},
									{Value: "192.0.2.3"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())

				Expect(recordsCreated).To(HaveLen(3))
				Expect(recordsCreated[0].Value).To(Equal("192.0.2.1"))
				Expect(recordsCreated[1].Value).To(Equal("192.0.2.2"))
				Expect(recordsCreated[2].Value).To(Equal("192.0.2.3"))
			})

			It("should handle CAA records by removing spaces", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("CAA"): {
							New: &dns.RecordSet{
								Type: "CAA",
								TTL:  300,
								Records: []*dns.Record{
									{Value: "0 issue letsencrypt.org"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())

				Expect(recordsCreated).To(HaveLen(1))
				Expect(recordsCreated[0].Type).To(Equal("CAA"))
				// Spaces should be removed
				Expect(recordsCreated[0].Value).To(Equal("0issueletsencrypt.org"))
			})

			It("should create different record types", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("CNAME"): {
							New: &dns.RecordSet{
								Type: "CNAME",
								TTL:  300,
								Records: []*dns.Record{
									{Value: "target.example.com"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())

				Expect(recordsCreated).To(HaveLen(1))
				Expect(recordsCreated[0].Type).To(Equal("CNAME"))
				Expect(recordsCreated[0].Value).To(Equal("target.example.com"))
			})
		})

		Context("DELETE operations", func() {
			var recordsDeleted []string

			BeforeEach(func() {
				recordsDeleted = make([]string, 0)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/records"):
						// ListRecords - return existing records
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(RecordListResponse{
							Records: []Record{
								{
									ID:     "rec1",
									ZoneID: "zone123",
									Type:   "A",
									Name:   "test.example.com",
									Value:  "192.0.2.1",
									TTL:    300,
								},
								{
									ID:     "rec2",
									ZoneID: "zone123",
									Type:   "A",
									Name:   "test.example.com",
									Value:  "192.0.2.2",
									TTL:    300,
								},
							},
							Meta: Meta{},
						})

					case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/records/"):
						// DeleteRecord
						recordID := strings.TrimPrefix(r.URL.Path, "/records/")
						recordsDeleted = append(recordsDeleted, recordID)
						w.WriteHeader(http.StatusOK)

					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := &provider.DNSHandlerConfig{
					Properties: utils.Properties{
						"HETZNER_API_TOKEN": "test-token",
						"HETZNER_ENDPOINT":  server.URL,
					},
					Log:         logr.Discard(),
					Metrics:     metrics,
					RateLimiter: limiter,
				}

				var err error
				handler, err = NewHandler(config)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should delete existing records", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("A"): {
							Old: &dns.RecordSet{
								Type: "A",
								TTL:  300,
								Records: []*dns.Record{
									{Value: "192.0.2.1"},
									{Value: "192.0.2.2"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())

				Expect(recordsDeleted).To(HaveLen(2))
				Expect(recordsDeleted).To(ContainElements("rec1", "rec2"))
			})

			It("should handle deletion of non-existent records gracefully", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "nonexistent.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("A"): {
							Old: &dns.RecordSet{
								Type: "A",
								TTL:  300,
								Records: []*dns.Record{
									{Value: "192.0.2.99"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())

				// No deletions should occur for non-existent records
				Expect(recordsDeleted).To(BeEmpty())
			})
		})

		Context("UPDATE operations", func() {
			var recordsCreated []RecordCreateRequest
			var recordsDeleted []string

			BeforeEach(func() {
				recordsCreated = make([]RecordCreateRequest, 0)
				recordsDeleted = make([]string, 0)

				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/records"):
						// ListRecords - return existing record
						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(RecordListResponse{
							Records: []Record{
								{
									ID:     "rec1",
									ZoneID: "zone123",
									Type:   "A",
									Name:   "test.example.com",
									Value:  "192.0.2.1",
									TTL:    300,
								},
							},
							Meta: Meta{},
						})

					case r.Method == http.MethodPost && r.URL.Path == "/records":
						// CreateRecord
						var req RecordCreateRequest
						_ = json.NewDecoder(r.Body).Decode(&req)
						recordsCreated = append(recordsCreated, req)

						w.WriteHeader(http.StatusOK)
						_ = json.NewEncoder(w).Encode(RecordGetResponse{
							Record: Record{
								ID:     fmt.Sprintf("newrec%d", len(recordsCreated)),
								ZoneID: req.ZoneID,
								Type:   req.Type,
								Name:   req.Name,
								Value:  req.Value,
								TTL:    *req.TTL,
							},
						})

					case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/records/"):
						// DeleteRecord
						recordID := strings.TrimPrefix(r.URL.Path, "/records/")
						recordsDeleted = append(recordsDeleted, recordID)
						w.WriteHeader(http.StatusOK)

					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := &provider.DNSHandlerConfig{
					Properties: utils.Properties{
						"HETZNER_API_TOKEN": "test-token",
						"HETZNER_ENDPOINT":  server.URL,
					},
					Log:         logr.Discard(),
					Metrics:     metrics,
					RateLimiter: limiter,
				}

				var err error
				handler, err = NewHandler(config)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should update a record by deleting old and creating new", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("A"): {
							Old: &dns.RecordSet{
								Type: "A",
								TTL:  300,
								Records: []*dns.Record{
									{Value: "192.0.2.1"},
								},
							},
							New: &dns.RecordSet{
								Type: "A",
								TTL:  600,
								Records: []*dns.Record{
									{Value: "192.0.2.100"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())

				// Old record should be deleted
				Expect(recordsDeleted).To(ContainElement("rec1"))

				// New record should be created
				Expect(recordsCreated).To(HaveLen(1))
				Expect(recordsCreated[0].Value).To(Equal("192.0.2.100"))
				Expect(*recordsCreated[0].TTL).To(Equal(600))
			})
		})

		Context("Error handling", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/records"):
						// ListRecords fails
						w.WriteHeader(http.StatusInternalServerError)
						_ = json.NewEncoder(w).Encode(ErrorResponse{
							Error: struct {
								Message string `json:"message"`
								Code    string `json:"code,omitempty"`
							}{
								Message: "Internal server error",
							},
						})

					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))

				config := &provider.DNSHandlerConfig{
					Properties: utils.Properties{
						"HETZNER_API_TOKEN": "test-token",
						"HETZNER_ENDPOINT":  server.URL,
					},
					Log:         logr.Discard(),
					Metrics:     metrics,
					RateLimiter: limiter,
				}

				var err error
				handler, err = NewHandler(config)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should fail if ListRecords fails", func() {
				reqs := provider.ChangeRequests{
					Name: dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{
						dns.RecordType("A"): {
							New: &dns.RecordSet{
								Type: "A",
								TTL:  300,
								Records: []*dns.Record{
									{Value: "192.0.2.1"},
								},
							},
						},
					},
				}

				err := handler.ExecuteRequests(ctx, zone, reqs)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to list existing records"))
			})
		})

		Context("Empty change sets", func() {
			It("should handle empty updates gracefully", func() {
				config := &provider.DNSHandlerConfig{
					Properties: utils.Properties{
						"HETZNER_API_TOKEN": "test-token",
					},
					Log:         logr.Discard(),
					Metrics:     metrics,
					RateLimiter: limiter,
				}

				h, err := NewHandler(config)
				Expect(err).NotTo(HaveOccurred())

				reqs := provider.ChangeRequests{
					Name:    dns.DNSSetName{DNSName: "test.example.com", SetIdentifier: ""},
					Updates: map[dns.RecordType]*provider.ChangeRequestUpdate{},
				}

				err = h.ExecuteRequests(ctx, zone, reqs)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
