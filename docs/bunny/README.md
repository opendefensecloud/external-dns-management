# Bunny DNS Provider

## Overview

The Bunny DNS provider enables external-dns-management to manage DNS records in Bunny.net DNS zones. This provider uses the [Bunny.net API](https://docs.bunny.net/reference/bunnynet-api-overview) to create, update, and delete DNS records.

**Provider Type**: `bunny-dns`

**Supported in**: Next-generation controller only (`dns-controller-manager-next-generation`)

## Features

- Create, update, and delete DNS records
- Support for common DNS record types (A, AAAA, CNAME, TXT, MX, SRV, CAA, NS, PTR)
- Automatic rate limiting and retry logic
- Comprehensive error handling and logging
- Metrics tracking for all operations

## Prerequisites

1. **Bunny.net Account**: You need a Bunny.net account with DNS zones configured
2. **API Key**: Get your API key from the [Bunny.net Dashboard](https://panel.bunny.net/account)
   - Navigate to: Account → API → Copy your API Key
   - Store the key securely - treat it as a password

## Configuration

### Step 1: Create a Secret with API Credentials

Create a Kubernetes Secret containing your Bunny.net API key:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bunny-dns-credentials
  namespace: default
type: Opaque
stringData:
  BUNNY_API_KEY: "your-api-key-here"
```

**Important**: The API key must not contain trailing whitespace or newlines.

### Step 2: Create a DNSProvider Resource

Create a DNSProvider resource that references the secret:

```yaml
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: bunny-dns
  namespace: default
spec:
  type: bunny-dns
  secretRef:
    name: bunny-dns-credentials
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

- **BUNNY_API_KEY**: Your Bunny.net API key

### Optional Fields

- **BUNNY_ENDPOINT**: Custom API endpoint (default: `https://api.bunny.net`)
  - Use this for testing or if Bunny changes their API endpoint
  - Must be an HTTPS URL

- **BUNNY_HTTP_TIMEOUT**: HTTP request timeout in seconds (default: `30`)
  - Valid range: 1-300 seconds
  - Recommended: 30-60 seconds for production

### Example with Optional Fields

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bunny-dns-credentials
  namespace: default
type: Opaque
stringData:
  BUNNY_API_KEY: "your-api-key-here"
  BUNNY_ENDPOINT: "https://api.bunny.net"
  BUNNY_HTTP_TIMEOUT: "60"
```

## Supported Record Types

The Bunny DNS provider supports the following DNS record types:

| Record Type | Supported | Notes |
|-------------|-----------|-------|
| A | Yes | IPv4 addresses |
| AAAA | Yes | IPv6 addresses |
| CNAME | Yes | Canonical name records |
| MX | Yes | Mail exchange records |
| TXT | Yes | Text records (SPF, DKIM, etc.) |
| SRV | Yes | Service records |
| CAA | Yes | Certification Authority Authorization |
| NS | Yes | Name server records |
| PTR | Yes | Pointer records |
| Redirect | No | Bunny-specific, not supported |
| Flatten | No | Bunny-specific, not supported |
| PullZone | No | Bunny-specific, not supported |
| Script | No | Bunny-specific, not supported |

## Provider-Specific Behavior

### Record Creation

Bunny DNS uses PUT requests for creating records and returns the created record with its ID. The provider handles this automatically.

### Multiple Records per Name

Bunny DNS supports multiple records with the same name and type:
- Each record is stored separately with a unique ID
- When updating a record set, all existing records are deleted and recreated
- This is transparent to the user but important for understanding API usage

### Zone Management

- Zones must be pre-created in the Bunny.net Dashboard
- The provider only manages DNS records within existing zones
- Zone delegation (NS records) can be managed via DNSEntry resources

## Rate Limiting

The Bunny.net API implements rate limiting. The provider handles this automatically:

### Provider Rate Limiting

1. **Client-level rate limiting**: Respects API rate limit headers
2. **Controller-level rate limiting**: Configured via `ConcurrentSyncs` setting
3. **Automatic retry**: Failed requests due to rate limiting (HTTP 429) are automatically retried with exponential backoff

### Retry Logic

- **Transient errors** (HTTP 500, 502, 503, 504): Automatic retry with exponential backoff
- **Rate limit errors** (HTTP 429): Automatic retry after backoff
- **Authentication errors** (HTTP 401): No retry, immediate failure
- **Forbidden errors** (HTTP 403): No retry, immediate failure
- **Not found errors** (HTTP 404): No retry for deletions (idempotent), retry for other operations

## DNS Propagation

After records are created or updated:
- Changes are typically visible via Bunny's nameservers within seconds
- Global DNS propagation depends on TTL and caching by recursive resolvers
- Recommended minimum TTL: 60 seconds

## Troubleshooting

### Common Issues

#### 1. Authentication Failures

**Symptom**: Error message `authentication failed` or HTTP 401

**Solutions**:
- Verify your API key is correct
- Check that the secret is in the same namespace as the DNSProvider
- Ensure the API key has not been regenerated
- Verify there are no trailing spaces or newlines in the key

#### 2. Zone Not Found

**Symptom**: Error message `zone not found` or HTTP 404

**Solutions**:
- Verify the zone exists in the Bunny.net Dashboard
- Check that the domain in your DNSEntry matches a zone in Bunny DNS
- Ensure the DNSProvider `domains.include` filter allows the zone

#### 3. Rate Limiting

**Symptom**: Error message `rate limit exceeded` or HTTP 429

**Solutions**:
- The provider automatically retries rate-limited requests
- If persistent, reduce the number of concurrent reconciliations
- Consider increasing the sync interval in the controller configuration

#### 4. Permission Denied

**Symptom**: Error message indicating forbidden or HTTP 403

**Solutions**:
- Verify your API key has the correct permissions
- Check that your Bunny.net account has DNS access enabled

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
kubectl describe dnsprovider bunny-dns -n default
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
- `Status.Provider`: Should show `bunny-dns`
- `Status.Message`: Error messages if entry is not ready
- `Status.Targets`: Currently configured targets

## Metrics

The Bunny DNS provider tracks the following metrics:

- `dns_provider_requests_total`: Total API requests by type (ListZones, ListRecords, CreateRecord, etc.)
- `dns_provider_requests_duration_seconds`: Request duration histogram
- Zone-specific metrics for record operations

These metrics are available via the controller's metrics endpoint (default: `:8080/metrics`).

## Security Considerations

1. **API Key Storage**: Store keys in Kubernetes Secrets, never in plain text
2. **RBAC**: Limit access to Secrets containing API keys
3. **Key Rotation**: Regularly rotate API keys and update secrets
4. **Network Policies**: Consider restricting egress to Bunny.net API endpoints
5. **Audit Logging**: Enable audit logging for DNSProvider and DNSEntry changes

## Limitations

1. **Bunny-specific record types not supported**: Redirect, Flatten, PullZone, and Script records are Bunny-specific features and cannot be managed via this provider
2. **No Infoblox Support**: This provider is only available in the next-generation controller
3. **Zone Creation**: Zones must be created manually via the Bunny.net Dashboard
4. **Scriptable DNS**: Bunny's Scriptable DNS features are not supported

## Migration from Other Providers

When migrating from another DNS provider:

1. **Backup existing records**: Export DNS records from your current provider
2. **Create zones in Bunny**: Create zones in Bunny.net Dashboard
3. **Import records**: Manually import existing records or let external-dns-management recreate them
4. **Update DNSProvider**: Change the provider type to `bunny-dns` and update credentials
5. **Verify records**: Confirm all records are created correctly before changing nameservers
6. **Update nameservers**: Point your domain to Bunny nameservers:
   - `kiki.bunny.net`
   - `coco.bunny.net`

## Performance

- **Caching**: The next-generation controller queries authoritative nameservers directly, reducing provider API usage
- **Parallel processing**: Multiple zones can be processed concurrently (configurable via `ConcurrentSyncs`)

Typical performance metrics:
- Zone listing: ~200-500ms per request
- Record listing: ~100-300ms per zone (depends on record count)
- Record creation: ~200-400ms per record
- Record deletion: ~100-200ms per record

## Support

For issues specific to:
- **Bunny.net API**: Contact [Bunny.net Support](https://bunny.net/contact)
- **external-dns-management**: Open an issue on [GitHub](https://github.com/gardener/external-dns-management/issues)
- **This provider implementation**: Include logs with `--log-level=debug` when reporting issues

## References

- [Bunny.net API Documentation](https://docs.bunny.net/reference/bunnynet-api-overview)
- [Bunny.net DNS Documentation](https://docs.bunny.net/reference/dnszonepublic_index)
- [Bunny.net Dashboard](https://panel.bunny.net)
- [external-dns-management Documentation](https://github.com/gardener/external-dns-management)
- [DNSEntry API Reference](https://github.com/gardener/external-dns-management/blob/master/docs/usage/dnsentry.md)
- [DNSProvider API Reference](https://github.com/gardener/external-dns-management/blob/master/docs/usage/dnsprovider.md)
