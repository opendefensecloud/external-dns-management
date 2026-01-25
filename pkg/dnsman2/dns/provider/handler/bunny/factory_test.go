// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

func TestBunny(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bunny DNS Provider Suite")
}

var _ = Describe("Factory", func() {
	Describe("ProviderType", func() {
		It("should return the correct provider type", func() {
			a := newAdapter()
			Expect(a.ProviderType()).To(Equal("bunny-dns"))
		})
	})

	Describe("ValidateCredentialsAndProviderConfig", func() {
		var a *adapter

		BeforeEach(func() {
			a = newAdapter().(*adapter)
		})

		Context("with valid credentials", func() {
			It("should accept valid API key", func() {
				props := utils.Properties{
					"BUNNY_API_KEY": "valid-api-key-12345",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should accept valid API key with optional endpoint", func() {
				props := utils.Properties{
					"BUNNY_API_KEY":  "valid-api-key-12345",
					"BUNNY_ENDPOINT": "https://custom.api.bunny.net",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).ToNot(HaveOccurred())
			})

			It("should accept valid API key with optional timeout", func() {
				props := utils.Properties{
					"BUNNY_API_KEY":      "valid-api-key-12345",
					"BUNNY_HTTP_TIMEOUT": "60",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("with invalid credentials", func() {
			It("should reject missing API key", func() {
				props := utils.Properties{}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("BUNNY_API_KEY"))
			})

			It("should reject API key with trailing whitespace", func() {
				props := utils.Properties{
					"BUNNY_API_KEY": "valid-api-key ",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
			})

			It("should reject API key with trailing newline", func() {
				props := utils.Properties{
					"BUNNY_API_KEY": "valid-api-key\n",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
			})

			It("should reject invalid endpoint URL", func() {
				props := utils.Properties{
					"BUNNY_API_KEY":  "valid-api-key-12345",
					"BUNNY_ENDPOINT": "http://insecure.endpoint.com",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
			})

			It("should reject invalid timeout value", func() {
				props := utils.Properties{
					"BUNNY_API_KEY":      "valid-api-key-12345",
					"BUNNY_HTTP_TIMEOUT": "invalid",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
			})

			It("should reject timeout out of range", func() {
				props := utils.Properties{
					"BUNNY_API_KEY":      "valid-api-key-12345",
					"BUNNY_HTTP_TIMEOUT": "500",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("with provider config", func() {
			It("should reject non-empty provider config", func() {
				props := utils.Properties{
					"BUNNY_API_KEY": "valid-api-key-12345",
				}
				config := &runtime.RawExtension{
					Raw: []byte(`{"some": "config"}`),
				}
				err := a.ValidateCredentialsAndProviderConfig(props, config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("provider config not supported"))
			})

			It("should accept nil provider config", func() {
				props := utils.Properties{
					"BUNNY_API_KEY": "valid-api-key-12345",
				}
				err := a.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
})
