// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/dns/utils"
)

// ProviderType is the type identifier for the Bunny DNS handler.
const ProviderType = "bunny-dns"

// RegisterTo registers the Bunny DNS handler to the given registry.
func RegisterTo(registry *provider.DNSHandlerRegistry) {
	registry.Register(
		ProviderType,
		NewHandler,
		newAdapter(),
		&config.RateLimiterOptions{
			Enabled: true,
			QPS:     50, // Conservative estimate for Bunny's rate limits
			Burst:   20, // Allow small bursts
		},
		nil, // No custom targets mapping needed
	)
}

type adapter struct {
	checks *provider.DNSHandlerAdapterChecks
}

func newAdapter() provider.DNSHandlerAdapter {
	checks := provider.NewDNSHandlerAdapterChecks()

	// Required: API key
	checks.Add(provider.RequiredProperty("BUNNY_API_KEY").
		Validators(
			provider.NoTrailingWhitespaceValidator,
			provider.NoTrailingNewlineValidator,
			provider.MaxLengthValidator(256),
		).
		HideValue()) // Don't log sensitive token

	// Optional: Custom endpoint (for testing/staging)
	checks.Add(provider.OptionalProperty("BUNNY_ENDPOINT").
		Validators(
			provider.URLValidator("https"), // Only HTTPS
		).
		AllowEmptyValue())

	// Optional: HTTP timeout
	checks.Add(provider.OptionalProperty("BUNNY_HTTP_TIMEOUT").
		Validators(
			provider.IntValidator(1, 300), // 1-300 seconds
		).
		AllowEmptyValue())

	return &adapter{checks: checks}
}

func (a *adapter) ProviderType() string {
	return ProviderType
}

func (a *adapter) ValidateCredentialsAndProviderConfig(properties utils.Properties, config *runtime.RawExtension) error {
	// Bunny doesn't need complex provider config
	if config != nil && len(config.Raw) > 0 {
		return fmt.Errorf("provider config not supported for %s provider", a.ProviderType())
	}
	return a.checks.ValidateProperties(a.ProviderType(), properties)
}
