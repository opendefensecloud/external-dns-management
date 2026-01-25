// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("Handler", func() {
	var (
		server  *httptest.Server
		h       provider.DNSHandler
		ctx     context.Context
		metrics *mockMetrics
	)

	BeforeEach(func() {
		ctx = context.Background()
		metrics = &mockMetrics{}
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if h != nil {
			h.Release()
		}
	})

	Describe("NewHandler", func() {
		It("should create a handler with valid config", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			config := &provider.DNSHandlerConfig{
				Log: log.Log,
				Properties: utils.Properties{
					"BUNNY_API_KEY":  "test-api-key",
					"BUNNY_ENDPOINT": server.URL,
				},
				RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				Metrics:     metrics,
			}

			var err error
			h, err = NewHandler(config)
			Expect(err).ToNot(HaveOccurred())
			Expect(h).ToNot(BeNil())
			Expect(h.ProviderType()).To(Equal(ProviderType))
		})

		It("should fail without API key", func() {
			config := &provider.DNSHandlerConfig{
				Log:         log.Log,
				Properties:  utils.Properties{},
				RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				Metrics:     metrics,
			}

			_, err := NewHandler(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("API key"))
		})

		It("should use custom timeout when specified", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			config := &provider.DNSHandlerConfig{
				Log: log.Log,
				Properties: utils.Properties{
					"BUNNY_API_KEY":      "test-api-key",
					"BUNNY_ENDPOINT":     server.URL,
					"BUNNY_HTTP_TIMEOUT": "60",
				},
				RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				Metrics:     metrics,
			}

			var err error
			h, err = NewHandler(config)
			Expect(err).ToNot(HaveOccurred())
			Expect(h).ToNot(BeNil())
		})
	})

	Describe("GetZones", func() {
		It("should return zones from the API", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/dnszone" {
					resp := ZoneListResponse{
						Items: []Zone{
							{ID: 1, Domain: "example.com"},
							{ID: 2, Domain: "example.org"},
						},
						HasMoreItems: false,
					}
					w.Header().Set("Content-Type", "application/json")
					err := json.NewEncoder(w).Encode(resp)
					Expect(err).ToNot(HaveOccurred())
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))

			config := &provider.DNSHandlerConfig{
				Log: log.Log,
				Properties: utils.Properties{
					"BUNNY_API_KEY":  "test-api-key",
					"BUNNY_ENDPOINT": server.URL,
				},
				RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				Metrics:     metrics,
			}

			var err error
			h, err = NewHandler(config)
			Expect(err).ToNot(HaveOccurred())

			zones, err := h.GetZones(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(zones).To(HaveLen(2))
			Expect(zones[0].Domain()).To(Equal("example.com"))
			Expect(zones[1].Domain()).To(Equal("example.org"))
		})

		It("should handle API errors", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				resp := ErrorResponse{Message: "Internal server error"}
				err := json.NewEncoder(w).Encode(resp)
				Expect(err).ToNot(HaveOccurred())
			}))

			config := &provider.DNSHandlerConfig{
				Log: log.Log,
				Properties: utils.Properties{
					"BUNNY_API_KEY":  "test-api-key",
					"BUNNY_ENDPOINT": server.URL,
				},
				RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				Metrics:     metrics,
			}

			var err error
			h, err = NewHandler(config)
			Expect(err).ToNot(HaveOccurred())

			_, err = h.GetZones(ctx)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("ProviderType", func() {
		It("should return bunny-dns", func() {
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			config := &provider.DNSHandlerConfig{
				Log: log.Log,
				Properties: utils.Properties{
					"BUNNY_API_KEY":  "test-api-key",
					"BUNNY_ENDPOINT": server.URL,
				},
				RateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
				Metrics:     metrics,
			}

			var err error
			h, err = NewHandler(config)
			Expect(err).ToNot(HaveOccurred())
			Expect(h.ProviderType()).To(Equal("bunny-dns"))
		})
	})
})

// Mock implementations for testing

type mockMetrics struct{}

func (m *mockMetrics) AddGenericRequests(reqType provider.MetricsRequestType, count int) {}

func (m *mockMetrics) AddZoneRequests(zoneID string, reqType provider.MetricsRequestType, count int) {
}
