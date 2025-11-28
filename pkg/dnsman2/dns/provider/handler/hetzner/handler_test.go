// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package hetzner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("Handler", func() {
	var (
		ctx    context.Context
		server *httptest.Server
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	Describe("NewHandler", func() {
		It("should create a handler with valid config", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{
					"HETZNER_API_TOKEN": "test-token-12345",
					"HETZNER_ENDPOINT":  server.URL,
				},
				Log: logr.Discard(),
			}

			h, err := NewHandler(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(h).NotTo(BeNil())
			Expect(h.(*handler).ProviderType()).To(Equal("hetzner-dns"))
		})

		It("should fail when API token is missing", func() {
			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{},
				Log:        logr.Discard(),
			}

			handler, err := NewHandler(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing API token"))
			Expect(handler).To(BeNil())
		})

		It("should use default endpoint when not specified", func() {
			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{
					"HETZNER_API_TOKEN": "test-token-12345",
				},
				Log: logr.Discard(),
			}

			h, err := NewHandler(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(h).NotTo(BeNil())
			Expect(h.(*handler).client.baseURL).To(Equal(defaultBaseURL))
		})

		It("should use custom endpoint when specified", func() {
			customURL := "https://custom.hetzner.test/api/v1"
			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{
					"HETZNER_API_TOKEN": "test-token-12345",
					"HETZNER_ENDPOINT":  customURL,
				},
				Log: logr.Discard(),
			}

			h, err := NewHandler(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(h).NotTo(BeNil())
			Expect(h.(*handler).client.baseURL).To(Equal(customURL))
		})

		It("should use custom timeout when specified", func() {
			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{
					"HETZNER_API_TOKEN":    "test-token-12345",
					"HETZNER_HTTP_TIMEOUT": "60",
				},
				Log: logr.Discard(),
			}

			h, err := NewHandler(config)
			Expect(err).NotTo(HaveOccurred())
			Expect(h).NotTo(BeNil())
			Expect(h.(*handler).client.httpClient.Timeout.Seconds()).To(Equal(float64(60)))
		})

		It("should fail with invalid timeout", func() {
			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{
					"HETZNER_API_TOKEN":    "test-token-12345",
					"HETZNER_HTTP_TIMEOUT": "invalid",
				},
				Log: logr.Discard(),
			}

			handler, err := NewHandler(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid timeout"))
			Expect(handler).To(BeNil())
		})
	})

	Describe("GetZones", func() {
		var (
			handler provider.DNSHandler
			metrics *mockMetrics
			limiter *mockRateLimiter
		)

		BeforeEach(func() {
			metrics = &mockMetrics{}
			limiter = &mockRateLimiter{}
		})

		Context("with successful API response", func() {
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
								Name: "test.org",
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

			It("should list all zones", func() {
				zones, err := handler.GetZones(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(zones).To(HaveLen(2))

				// Check first zone
				Expect(zones[0].ZoneID().ProviderType).To(Equal("hetzner-dns"))
				Expect(zones[0].Domain()).To(Or(Equal("example.com."), Equal("example.com")))
				Expect(zones[0].Key()).To(Equal("zone1"))
				Expect(zones[0].IsPrivate()).To(BeFalse())

				// Check second zone
				Expect(zones[1].ZoneID().ProviderType).To(Equal("hetzner-dns"))
				Expect(zones[1].Domain()).To(Or(Equal("test.org."), Equal("test.org")))
				Expect(zones[1].Key()).To(Equal("zone2"))
				Expect(zones[1].IsPrivate()).To(BeFalse())
			})

			It("should track metrics", func() {
				_, err := handler.GetZones(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(metrics.requestCount).To(Equal(1))
				Expect(metrics.lastRequestType).To(Equal(provider.MetricsRequestTypeListZones))
			})

			It("should call rate limiter", func() {
				_, err := handler.GetZones(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(limiter.acceptCount).To(Equal(1))
			})
		})

		Context("with empty zone list", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(ZoneListResponse{
						Zones: []Zone{},
						Meta: Meta{
							Pagination: &Pagination{
								Page:         1,
								PerPage:      100,
								LastPage:     1,
								TotalEntries: 0,
							},
						},
					})
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

			It("should return empty zone list", func() {
				zones, err := handler.GetZones(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(zones).To(BeEmpty())
			})
		})

		Context("with API error", func() {
			BeforeEach(func() {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(ErrorResponse{
						Error: struct {
							Message string `json:"message"`
							Code    string `json:"code,omitempty"`
						}{
							Message: "Invalid API token",
							Code:    "UNAUTHORIZED",
						},
					})
				}))

				config := &provider.DNSHandlerConfig{
					Properties: utils.Properties{
						"HETZNER_API_TOKEN": "invalid-token",
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

			It("should return error from API", func() {
				zones, err := handler.GetZones(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to list zones"))
				Expect(zones).To(BeNil())
			})
		})
	})

	Describe("GetCustomQueryDNSFunc", func() {
		var handler provider.DNSHandler

		BeforeEach(func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{
					"HETZNER_API_TOKEN": "test-token",
					"HETZNER_ENDPOINT":  server.URL,
				},
				Log: logr.Discard(),
			}

			var err error
			handler, err = NewHandler(config)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return a custom query function", func() {
			mockFactory := func() (utils.QueryDNS, error) {
				return &mockQueryDNS{}, nil
			}

			zoneInfo := dns.ZoneInfo{}
			queryFunc, err := handler.GetCustomQueryDNSFunc(zoneInfo, mockFactory)
			Expect(err).NotTo(HaveOccurred())
			Expect(queryFunc).NotTo(BeNil())
		})

		It("should propagate factory errors", func() {
			mockFactory := func() (utils.QueryDNS, error) {
				return nil, context.DeadlineExceeded
			}

			zoneInfo := dns.ZoneInfo{}
			queryFunc, err := handler.GetCustomQueryDNSFunc(zoneInfo, mockFactory)
			Expect(err).To(HaveOccurred())
			Expect(err).To(Equal(context.DeadlineExceeded))
			Expect(queryFunc).To(BeNil())
		})
	})

	Describe("Release", func() {
		It("should not panic", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			config := &provider.DNSHandlerConfig{
				Properties: utils.Properties{
					"HETZNER_API_TOKEN": "test-token",
					"HETZNER_ENDPOINT":  server.URL,
				},
				Log: logr.Discard(),
			}

			handler, err := NewHandler(config)
			Expect(err).NotTo(HaveOccurred())

			Expect(func() {
				handler.Release()
			}).NotTo(Panic())
		})
	})
})

// Mock implementations for testing

type mockMetrics struct {
	requestCount    int
	lastRequestType provider.MetricsRequestType
}

func (m *mockMetrics) AddGenericRequests(requestType provider.MetricsRequestType, count int) {
	m.requestCount += count
	m.lastRequestType = requestType
}

func (m *mockMetrics) AddZoneRequests(_ string, _ provider.MetricsRequestType, _ int) {
	// Not used in handler tests
}

type mockRateLimiter struct {
	acceptCount int
}

func (m *mockRateLimiter) Accept() {
	m.acceptCount++
}

func (m *mockRateLimiter) QPS() float32 {
	return 50.0
}

func (m *mockRateLimiter) Stop() {
	// Not needed for tests
}

func (m *mockRateLimiter) Wait(_ context.Context) error {
	return nil
}

func (m *mockRateLimiter) TryAccept() bool {
	m.Accept()
	return true
}

type mockQueryDNS struct{}

func (m *mockQueryDNS) Query(_ context.Context, _ dns.DNSSetName, _ dns.RecordType) utils.QueryDNSResult {
	return utils.QueryDNSResult{
		RecordSet: &dns.RecordSet{
			Type: "A",
			TTL:  300,
			Records: []*dns.Record{
				{Value: "192.0.2.1"},
			},
		},
		Err: nil,
	}
}
