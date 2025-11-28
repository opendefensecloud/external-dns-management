// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package hetzner

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

var _ = Describe("Factory", func() {
	var adpt *adapter

	BeforeEach(func() {
		adpt = newAdapter().(*adapter)
	})

	Describe("ProviderType", func() {
		It("should return the correct provider type", func() {
			Expect(adpt.ProviderType()).To(Equal("hetzner-dns"))
		})
	})

	Describe("ValidateCredentialsAndProviderConfig", func() {
		Context("with valid credentials", func() {
			It("should succeed with valid API token", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-sufficient-length",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should succeed with valid API token and endpoint", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-sufficient-length",
					"HETZNER_ENDPOINT":  "https://dns.hetzner.test/api/v1",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should succeed with valid API token and timeout", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN":    "valid-token-with-sufficient-length",
					"HETZNER_HTTP_TIMEOUT": "60",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should succeed with all optional fields", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN":    "valid-token-with-sufficient-length",
					"HETZNER_ENDPOINT":     "https://dns.hetzner.test/api/v1",
					"HETZNER_HTTP_TIMEOUT": "120",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("with invalid credentials", func() {
			It("should fail when API token is missing", func() {
				props := utils.Properties{}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("HETZNER_API_TOKEN"))
			})

			It("should fail when API token has trailing whitespace", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-suffix ",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("whitespace"))
			})

			It("should fail when API token has trailing newline", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-suffix\n",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				// NoTrailingWhitespaceValidator catches newlines before NoTrailingNewlineValidator
				Expect(err.Error()).To(ContainSubstring("whitespace"))
			})

			It("should fail when API token exceeds maximum length", func() {
				// Create a token longer than 256 characters
				longToken := make([]byte, 257)
				for i := range longToken {
					longToken[i] = 'a'
				}
				props := utils.Properties{
					"HETZNER_API_TOKEN": string(longToken),
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("maximum length"))
			})

			It("should fail when endpoint is not HTTPS", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-sufficient-length",
					"HETZNER_ENDPOINT":  "http://dns.hetzner.test/api/v1",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("https"))
			})

			It("should fail when endpoint is not a valid URL", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-sufficient-length",
					"HETZNER_ENDPOINT":  "not-a-valid-url",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
			})

			It("should fail when timeout is not a number", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN":    "valid-token-with-sufficient-length",
					"HETZNER_HTTP_TIMEOUT": "not-a-number",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("integer"))
			})

			It("should fail when timeout is below minimum", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN":    "valid-token-with-sufficient-length",
					"HETZNER_HTTP_TIMEOUT": "0",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("between"))
			})

			It("should fail when timeout exceeds maximum", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN":    "valid-token-with-sufficient-length",
					"HETZNER_HTTP_TIMEOUT": "301",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("between"))
			})
		})

		Context("with provider config", func() {
			It("should fail when provider config is provided", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-sufficient-length",
				}
				config := &runtime.RawExtension{
					Raw: []byte(`{"someConfig": "value"}`),
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, config)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("provider config not supported"))
			})

			It("should succeed when provider config is nil", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-sufficient-length",
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should succeed when provider config has empty raw data", func() {
				props := utils.Properties{
					"HETZNER_API_TOKEN": "valid-token-with-sufficient-length",
				}
				config := &runtime.RawExtension{
					Raw: []byte{},
				}
				err := adpt.ValidateCredentialsAndProviderConfig(props, config)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
