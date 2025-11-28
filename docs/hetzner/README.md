# Hetzner DNS Provider

## Overview

The Hetzner DNS provider enables external-dns-management to manage DNS records in Hetzner DNS zones. This provider uses the [Hetzner DNS API](https://dns.hetzner.com/api-docs) to create, update, and delete DNS records.

**Provider Type**: `hetzner-dns`

**Supported in**: Next-generation controller only (`dns-controller-manager-next-generation`)

## Features

- ✅ Create, update, and delete DNS records
- ✅ Support for all common DNS record types (A, AAAA, CNAME, TXT, MX, SRV, CAA, NS)
- ✅ Automatic rate limiting and retry logic
- ✅ Bulk record creation optimization
- ✅ CAA record format normalization (automatic space removal)
- ✅ Comprehensive error handling and logging
- ✅ Metrics tracking for all operations

## Prerequisites

1. **Hetzner Account**: You need a Hetzner account with DNS zones configured
2. **API Token**: Generate an API token from the [Hetzner DNS Console](https://dns.hetzner.com/)
   - Navigate to: DNS Console → API Tokens → Generate API Token
   - Store the token securely - it will only be shown once

## Configuration

### Step 1: Create a Secret with API Credentials

Create a Kubernetes Secret containing your Hetzner API token:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hetzner-dns-credentials
  namespace: default
type: Opaque
stringData:
  HETZNER_API_TOKEN: "your-api-token-here"
```

**Important**: The API token must not contain trailing whitespace or newlines.

### Step 2: Create a DNSProvider Resource

Create a DNSProvider resource that references the secret:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: hetzner-dns
  namespace: default
spec:
  type: hetzner-dns
  secretRef:
    name: hetzner-dns-credentials
  domains:
    include:
      - example.com
      - example.org
```

### Step 3: Create DNSEntry Resources

Create DNSEntry resources to manage DNS records:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  name: my-dns-entry
  namespace: default
spec:
  dnsName: test.example.com
  ttl: 300
  targets:
    - 192.0.2.1
    - 192.0.2.2
```

## Configuration Options

### Required Fields

- **HETZNER_API_TOKEN**: Your Hetzner DNS API token

### Optional Fields

- **HETZNER_ENDPOINT**: Custom API endpoint (default: `https://dns.hetzner.com/api/v1`)
  - Use this for testing or if Hetzner changes their API endpoint
  - Must be an HTTPS URL

- **HETZNER_HTTP_TIMEOUT**: HTTP request timeout in seconds (default: `30`)
  - Valid range: 10-300 seconds
  - Recommended: 30-60 seconds for production

### Example with Optional Fields

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hetzner-dns-credentials
  namespace: default
type: Opaque
stringData:
  HETZNER_API_TOKEN: "your-api-token-here"
  HETZNER_ENDPOINT: "https://dns.hetzner.com/api/v1"
  HETZNER_HTTP_TIMEOUT: "60"
```

## Supported Record Types

The Hetzner DNS provider supports the following DNS record types:

| Record Type | Supported | Notes |
|-------------|-----------|-------|
| A | ✅ Yes | IPv4 addresses |
| AAAA | ✅ Yes | IPv6 addresses |
| CNAME | ✅ Yes | Canonical name records |
| MX | ✅ Yes | Mail exchange records |
| TXT | ✅ Yes | Text records (SPF, DKIM, etc.) |
| SRV | ✅ Yes | Service records |
| CAA | ✅ Yes | Certification Authority Authorization (see notes below) |
| NS | ✅ Yes | Name server records |
| SOA | ❌ No | Managed by Hetzner, cannot be modified |

## Provider-Specific Behavior

### CAA Record Handling

Hetzner DNS has specific requirements for CAA records:
- **Spaces are automatically removed** from CAA record values
- Standard format: `0 issue "letsencrypt.org"`
- Hetzner format: `0issueletsencrypt.org`

The provider automatically normalizes CAA records to match Hetzner's requirements.

### Multiple Records per Name

Hetzner DNS supports multiple records with the same name and type:
- Each record is stored separately with a unique ID
- When updating a record set, all existing records are deleted and recreated
- This is transparent to the user but important for understanding API usage

### Zone Management

- Zones must be pre-created in the Hetzner DNS Console
- The provider only manages DNS records within existing zones
- Zone delegation (NS records) can be managed via DNSEntry resources

## Rate Limiting

The Hetzner DNS provider implements multi-level rate limiting:

### API Rate Limits

Hetzner DNS API has the following rate limits:
- **Default**: Not publicly documented
- **Rate limit headers**: The API returns rate limit information in response headers
  - `X-RateLimit-Limit`: Maximum requests allowed
  - `X-RateLimit-Remaining`: Remaining requests in current window
  - `X-RateLimit-Reset`: Unix timestamp when the rate limit resets

### Provider Rate Limiting

The provider implements automatic rate limiting:
1. **Client-level rate limiting**: Respects API rate limit headers
2. **Controller-level rate limiting**: Configured via `ConcurrentSyncs` setting
3. **Automatic retry**: Failed requests due to rate limiting (HTTP 429) are automatically retried with exponential backoff

### Retry Logic

- **Transient errors** (HTTP 500, 502, 503, 504): Automatic retry with exponential backoff
- **Rate limit errors** (HTTP 429): Automatic retry after rate limit reset
- **Authentication errors** (HTTP 401): No retry, immediate failure
- **Not found errors** (HTTP 404): No retry for deletions (idempotent), retry for other operations

## DNS Propagation

After records are created or updated:
- Changes are typically visible via Hetzner's nameservers within seconds
- Global DNS propagation depends on TTL and caching by recursive resolvers
- Recommended minimum TTL: 60 seconds (Hetzner default: 86400)

## Troubleshooting

### Common Issues

#### 1. Authentication Failures

**Symptom**: Error message `authentication failed` or HTTP 401

**Solutions**:
- Verify your API token is correct
- Check that the secret is in the same namespace as the DNSProvider
- Ensure the API token has not expired or been revoked
- Verify there are no trailing spaces or newlines in the token

#### 2. Zone Not Found

**Symptom**: Error message `zone not found` or HTTP 404

**Solutions**:
- Verify the zone exists in the Hetzner DNS Console
- Check that the domain in your DNSEntry matches a zone in Hetzner DNS
- Ensure the DNSProvider `domains.include` filter allows the zone

#### 3. Rate Limiting

**Symptom**: Error message `rate limit exceeded` or HTTP 429

**Solutions**:
- The provider automatically retries rate-limited requests
- If persistent, reduce the number of concurrent reconciliations
- Consider increasing the sync interval in the controller configuration

#### 4. CAA Record Issues

**Symptom**: CAA records not created correctly

**Solutions**:
- Ensure CAA records follow the format: `flags tag "value"`
- The provider automatically removes spaces for Hetzner compatibility
- Example: `0 issue "letsencrypt.org"` → `0issueletsencrypt.org`

### Debugging

Enable debug logging in the controller:

```bash
# Controller command-line flags
--log-level=debug
```

This will show:
- All API requests and responses
- Rate limit information
- Detailed error messages
- Record creation/update/deletion operations

### Provider Status

Check the DNSProvider status for errors:

```bash
kubectl describe dnsprovider hetzner-dns -n default
```

Look for:
- `Status.State`: Should be `Ready`
- `Status.Message`: Error messages if provider is not ready
- `Status.Domains`: Zones discovered by the provider

### DNSEntry Status

Check individual DNSEntry status:

```bash
kubectl describe dnsentry my-dns-entry -n default
```

Look for:
- `Status.State`: Should be `Ready`
- `Status.Provider`: Should show `hetzner-dns`
- `Status.Message`: Error messages if entry is not ready
- `Status.Targets`: Currently configured targets

## Metrics

The Hetzner DNS provider tracks the following metrics:

- `dns_provider_requests_total`: Total API requests by type (ListZones, ListRecords, CreateRecord, etc.)
- `dns_provider_requests_duration_seconds`: Request duration histogram
- Zone-specific metrics for record operations

These metrics are available via the controller's metrics endpoint (default: `:8080/metrics`).

## API Token Permissions

The Hetzner API token requires the following permissions:
- **Zone:Read**: List zones and read zone details
- **Record:Read**: List DNS records
- **Record:Write**: Create, update, and delete DNS records

When creating an API token, select **"Read & Write"** permissions.

## Security Considerations

1. **API Token Storage**: Store tokens in Kubernetes Secrets, never in plain text
2. **RBAC**: Limit access to Secrets containing API tokens
3. **Token Rotation**: Regularly rotate API tokens and update secrets
4. **Network Policies**: Consider restricting egress to Hetzner DNS API endpoints
5. **Audit Logging**: Enable audit logging for DNSProvider and DNSEntry changes

## Limitations

1. **No SOA Record Management**: SOA records are managed by Hetzner and cannot be modified
2. **No Infoblox Support**: This provider is only available in the next-generation controller
3. **Zone Creation**: Zones must be created manually via the Hetzner DNS Console
4. **DNSSEC**: DNSSEC signing is managed by Hetzner, cannot be configured via API
5. **IPv6 API Access**: Ensure your cluster can reach `dns.hetzner.com` via IPv4 or IPv6

## Migration from Other Providers

When migrating from another DNS provider:

1. **Backup existing records**: Export DNS records from your current provider
2. **Import zones to Hetzner**: Create zones in Hetzner DNS Console
3. **Import records**: Manually import existing records or let external-dns-management recreate them
4. **Update DNSProvider**: Change the provider type to `hetzner-dns` and update credentials
5. **Verify records**: Confirm all records are created correctly before changing nameservers
6. **Update nameservers**: Point your domain to Hetzner nameservers:
   - `helium.ns.hetzner.de`
   - `hydrogen.ns.hetzner.com`
   - `oxygen.ns.hetzner.com`

## Performance

- **Bulk operations**: The provider uses bulk creation when possible to reduce API calls
- **Caching**: The next-generation controller queries authoritative nameservers directly, reducing provider API usage
- **Parallel processing**: Multiple zones can be processed concurrently (configurable via `ConcurrentSyncs`)

Typical performance metrics:
- Zone listing: ~500ms per request
- Record listing: ~200ms per zone (depends on record count)
- Record creation: ~300ms per record (bulk creation is faster)
- Record deletion: ~200ms per record

## Support

For issues specific to:
- **Hetzner DNS API**: Contact [Hetzner Support](https://www.hetzner.com/support)
- **external-dns-management**: Open an issue on [GitHub](https://github.com/gardener/external-dns-management/issues)
- **This provider implementation**: Include logs with `--log-level=debug` when reporting issues

## References

- [Hetzner DNS API Documentation](https://dns.hetzner.com/api-docs)
- [Hetzner DNS Console](https://dns.hetzner.com/)
- [external-dns-management Documentation](https://github.com/gardener/external-dns-management)
- [DNSEntry API Reference](https://github.com/gardener/external-dns-management/blob/master/docs/usage/dnsentry.md)
- [DNSProvider API Reference](https://github.com/gardener/external-dns-management/blob/master/docs/usage/dnsprovider.md)
