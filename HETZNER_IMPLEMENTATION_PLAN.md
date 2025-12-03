# Hetzner DNS Provider Implementation Plan

## Executive Summary

This plan outlines the implementation of a **nextgen (dnsman2)** Hetzner DNS provider for the external-dns-management project. The implementation will follow the controller-runtime pattern used by nextgen providers, ensuring future-proof architecture with comprehensive testing and high code quality.

## Why Nextgen Implementation?

- **Future-proof**: Uses controller-runtime, the standard Kubernetes controller framework
- **Better performance**: Queries authoritative nameservers directly instead of caching zone state
- **Reduced API calls**: Minimizes provider API usage, critical for Hetzner's strict rate limits
- **Modern architecture**: Cleaner separation of concerns and simpler codebase

## Hetzner DNS API Overview

Based on research of existing implementations and documentation:

### API Characteristics
- **Base URL**: `https://dns.hetzner.com/api/v1`
- **Authentication**: Bearer token via `Auth-API-Token` header
- **Rate Limiting**: Strict limits with header-based quota information
  - Headers: `Ratelimit-Limit`, `Ratelimit-Remaining`, `Ratelimit-Reset`
  - Recommended: Burst through half quota, then spread remaining requests
  - Respects HTTP 429 (Too Many Requests) with `Retry-After` header
- **Pagination**: Supported for zones and records listing
- **Record Types**: Standard DNS records (A, AAAA, CNAME, MX, TXT, SRV, etc.)

### Known Limitations
- **CAA Records**: No spaces allowed between parameters (API quirk)
- **SOA Records**: Cannot be modified via API
- **Rate Limits**: Heavily rate-limited (exact limits TBD, likely ~3600 req/hour)

### API Endpoints (Expected)
```
GET    /zones                      # List all zones
GET    /zones/{zoneId}             # Get zone details
GET    /zones/{zoneId}/records     # List records in zone
POST   /zones/{zoneId}/records     # Create record
PUT    /records/{recordId}         # Update record
DELETE /records/{recordId}         # Delete record
GET    /records/{recordId}         # Get record details
```

## Implementation Architecture

### File Structure
```
pkg/dnsman2/dns/provider/handler/hetzner/
├── factory.go           # Provider registration and adapter
├── handler.go           # Main DNSHandler implementation
├── client.go            # Hetzner API client wrapper
├── execution.go         # Change execution logic
├── types.go             # Hetzner API types and structs
├── handler_test.go      # Unit tests for handler
├── client_test.go       # Unit tests for client
└── integration_test.go  # Integration tests
```

### Dependencies
```go
// Consider existing Go libraries:
// Option 1: Use existing library (e.g., jobstoit/hetzner-dns-go)
// Option 2: Implement custom client for full control
// Recommendation: Custom client for better error handling and metrics
```

## Detailed Implementation Plan

### Phase 1: Research & API Client (Week 1)

#### Task 1.1: API Documentation Review
- [ ] Access official Hetzner DNS API docs at https://dns.hetzner.com/api-docs
- [ ] Document all endpoints with request/response schemas
- [ ] Identify exact rate limit values
- [ ] Document error response formats
- [ ] Test API manually with curl/Postman

**Deliverables**:
- `docs/hetzner/API_SPECIFICATION.md` with complete API reference
- Example API requests/responses

#### Task 1.2: Design Client Wrapper
- [ ] Create `client.go` with HTTP client setup
- [ ] Implement authentication header injection
- [ ] Design rate limit tracking based on response headers
- [ ] Implement retry logic for 429 responses
- [ ] Add comprehensive error handling
- [ ] Design pagination handling for zones/records

**Key Decisions**:
- Use standard `net/http` client with custom transport
- Implement exponential backoff for retries
- Parse and respect `Ratelimit-*` headers
- Context-aware operations for cancellation

**Code Structure**:
```go
type Client struct {
    baseURL     string
    token       string
    httpClient  *http.Client
    rateLimiter RateLimitTracker
    logger      logr.Logger
}

type RateLimitTracker struct {
    limit     int
    remaining int
    resetTime time.Time
    mu        sync.RWMutex
}

func (c *Client) ListZones(ctx context.Context) ([]Zone, error)
func (c *Client) GetZone(ctx context.Context, zoneID string) (*Zone, error)
func (c *Client) ListRecords(ctx context.Context, zoneID string) ([]Record, error)
func (c *Client) CreateRecord(ctx context.Context, zoneID string, record *Record) error
func (c *Client) UpdateRecord(ctx context.Context, recordID string, record *Record) error
func (c *Client) DeleteRecord(ctx context.Context, recordID string) error
```

#### Task 1.3: Client Unit Tests
- [ ] Mock HTTP responses using `httptest`
- [ ] Test rate limit header parsing
- [ ] Test retry logic on 429 responses
- [ ] Test pagination handling
- [ ] Test error scenarios (401, 404, 500, etc.)
- [ ] Test context cancellation

**Target**: 90%+ code coverage on `client.go`

### Phase 2: Provider Factory & Validation (Week 1-2)

#### Task 2.1: Implement Factory (`factory.go`)
```go
const ProviderType = "hetzner-dns"

func RegisterTo(registry *provider.DNSHandlerRegistry) {
    registry.Register(
        ProviderType,
        NewHandler,
        newAdapter(),
        &config.RateLimiterOptions{
            Enabled: true,
            QPS:     50,    // Conservative estimate, adjust after testing
            Burst:   20,    // Allow small bursts
        },
        nil,  // No custom targets mapping needed
    )
}
```

#### Task 2.2: Implement Credential Validation
```go
func newAdapter() provider.DNSHandlerAdapter {
    checks := provider.NewDNSHandlerAdapterChecks()

    // Required: API token
    checks.Add(provider.RequiredProperty("HETZNER_API_TOKEN").
        Validators(
            provider.NoTrailingWhitespaceValidator,
            provider.NoTrailingNewlineValidator,
            provider.MinLengthValidator(32),    // Tokens are typically long
            provider.MaxLengthValidator(256),
        ).
        HideValue())  // Don't log sensitive token

    // Optional: Custom endpoint (for testing/staging)
    checks.Add(provider.OptionalProperty("HETZNER_ENDPOINT").
        Validators(
            provider.URLValidator("https"),  // Only HTTPS
        ).
        AllowEmptyValue())

    // Optional: HTTP timeout
    checks.Add(provider.OptionalProperty("HETZNER_HTTP_TIMEOUT").
        Validators(
            provider.IntValidator(1, 300),  // 1-300 seconds
        ).
        AllowEmptyValue())

    return &adapter{checks: checks}
}

func (a *adapter) ValidateCredentialsAndProviderConfig(
    properties utils.Properties,
    config *runtime.RawExtension,
) error {
    // Hetzner doesn't need complex provider config
    if config != nil && len(config.Raw) > 0 {
        return fmt.Errorf("provider config not supported for %s", a.ProviderType())
    }
    return a.checks.ValidateProperties(a.ProviderType(), properties)
}
```

#### Task 2.3: Factory Tests
- [ ] Test successful registration
- [ ] Test validation with valid credentials
- [ ] Test validation failures (missing token, invalid format)
- [ ] Test URL validation for endpoint
- [ ] Test timeout validation

### Phase 3: DNSHandler Implementation (Week 2)

#### Task 3.1: Handler Initialization (`handler.go`)
```go
type handler struct {
    provider.DefaultDNSHandler
    config provider.DNSHandlerConfig
    client *Client
}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
    h := &handler{
        DefaultDNSHandler: provider.NewDefaultDNSHandler(ProviderType),
        config:            *c,
    }

    // Extract credentials
    token, err := c.GetRequiredProperty("HETZNER_API_TOKEN")
    if err != nil {
        return nil, fmt.Errorf("missing API token: %w", err)
    }

    // Optional endpoint override
    endpoint := c.GetDefaultedProperty("HETZNER_ENDPOINT", "https://dns.hetzner.com/api/v1")

    // Optional timeout
    timeout, _ := c.GetDefaultedIntProperty("HETZNER_HTTP_TIMEOUT", 30)

    // Initialize client
    h.client = NewClient(ClientConfig{
        BaseURL:     endpoint,
        Token:       token,
        HTTPTimeout: time.Duration(timeout) * time.Second,
        Logger:      c.Log,
    })

    return h, nil
}
```

#### Task 3.2: Implement GetZones()
```go
func (h *handler) GetZones(ctx context.Context) ([]provider.DNSHostedZone, error) {
    log, _ := logr.FromContext(ctx)
    log = log.WithValues("provider", h.ProviderType())

    // Rate limit at provider level (in addition to client level)
    h.config.RateLimiter.Accept()

    // Fetch zones from Hetzner
    zones, err := h.client.ListZones(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to list zones: %w", err)
    }

    // Track metrics
    h.config.Metrics.AddGenericRequests(provider.MetricsRequestTypeListZones, 1)

    // Convert to DNSHostedZone
    var hostedZones []provider.DNSHostedZone
    for _, zone := range zones {
        zoneID := dns.NewZoneID(h.ProviderType(), zone.ID)
        domain := dns.NormalizeDomainName(zone.Name)

        hostedZone := provider.NewDNSHostedZone(
            h.ProviderType(),
            zoneID,
            domain,
            zone.ID,      // key
            false,        // Hetzner DNS is always public
        )

        hostedZones = append(hostedZones, hostedZone)
        log.V(1).Info("discovered zone", "zone", domain, "id", zone.ID)
    }

    log.Info("zones discovered", "count", len(hostedZones))
    return hostedZones, nil
}
```

#### Task 3.3: Implement GetCustomQueryDNSFunc()
```go
func (h *handler) GetCustomQueryDNSFunc(
    zoneInfo dns.ZoneInfo,
    factory utils.QueryDNSFactoryFunc,
) (provider.CustomQueryDNSFunc, error) {
    // Hetzner DNS zones are always public, use standard DNS queries
    defaultQueryFunc, err := factory()
    if err != nil {
        return nil, err
    }

    return func(ctx context.Context, _ dns.ZoneInfo, setName dns.DNSSetName, recordType dns.RecordType) (*dns.RecordSet, error) {
        result := defaultQueryFunc.Query(ctx, setName, recordType)
        return result.RecordSet, result.Err
    }, nil
}
```

#### Task 3.4: Handler Tests
- [ ] Test handler initialization with valid config
- [ ] Test GetZones() with various zone counts
- [ ] Test GetZones() error handling
- [ ] Test metrics tracking
- [ ] Test logging output

### Phase 4: Change Execution (Week 2-3)

#### Task 4.1: Execution Structure (`execution.go`)
```go
type changeAction int

const (
    createRecord changeAction = iota
    updateRecord
    deleteRecord
)

type execution struct {
    log         logr.Logger
    handler     *handler
    zoneID      dns.ZoneID
    changes     []*change
}

type change struct {
    action   changeAction
    req      provider.ChangeRequests
    rs       *dns.RecordSet
    recordID string  // For updates/deletes
}

func newExecution(log logr.Logger, h *handler, zoneID dns.ZoneID) *execution {
    return &execution{
        log:     log.WithValues("zone", zoneID.ID),
        handler: h,
        zoneID:  zoneID,
        changes: make([]*change, 0),
    }
}
```

#### Task 4.2: Implement ExecuteRequests()
```go
func (h *handler) ExecuteRequests(
    ctx context.Context,
    zone provider.DNSHostedZone,
    reqs provider.ChangeRequests,
) error {
    log, _ := logr.FromContext(ctx)
    log = log.WithValues("provider", h.ProviderType(), "zone", zone.Domain())

    if len(reqs.Updates) == 0 {
        log.V(1).Info("no changes to execute")
        return nil
    }

    exec := newExecution(log, h, zone.ZoneID())

    // First, fetch existing records to get record IDs
    existingRecords, err := h.client.ListRecords(ctx, zone.ZoneID().ID)
    if err != nil {
        return fmt.Errorf("failed to list existing records: %w", err)
    }

    // Build lookup map: (name, type) -> record ID
    recordMap := make(map[string]string)
    for _, rec := range existingRecords {
        key := makeRecordKey(rec.Name, rec.Type)
        recordMap[key] = rec.ID
    }

    // Process change requests
    for _, update := range reqs.Updates {
        recordName := reqs.Name.DNSName

        if update.Old != nil && update.New != nil {
            // UPDATE
            log.Info("updating record", "name", recordName, "type", update.New.Type)

            key := makeRecordKey(recordName, update.New.Type)
            recordID := recordMap[key]

            // Hetzner doesn't have native update, so delete + create
            if recordID != "" {
                exec.addChange(deleteRecord, reqs, update.Old, recordID)
            }
            exec.addChange(createRecord, reqs, update.New, "")

        } else if update.Old != nil {
            // DELETE
            log.Info("deleting record", "name", recordName, "type", update.Old.Type)

            key := makeRecordKey(recordName, update.Old.Type)
            recordID := recordMap[key]

            if recordID != "" {
                exec.addChange(deleteRecord, reqs, update.Old, recordID)
            } else {
                log.V(1).Info("record already deleted", "name", recordName, "type", update.Old.Type)
            }

        } else if update.New != nil {
            // CREATE
            log.Info("creating record", "name", recordName, "type", update.New.Type)
            exec.addChange(createRecord, reqs, update.New, "")
        }
    }

    // Execute all changes
    return exec.submitChanges(ctx, h.config.Metrics)
}

func makeRecordKey(name, recordType string) string {
    return fmt.Sprintf("%s:%s", name, recordType)
}
```

#### Task 4.3: Change Submission with Error Handling
```go
func (ex *execution) submitChanges(ctx context.Context, metrics provider.Metrics) error {
    if len(ex.changes) == 0 {
        ex.log.V(1).Info("no changes to submit")
        return nil
    }

    ex.log.Info("submitting changes", "count", len(ex.changes))

    var errs []error
    successCount := 0

    for i, ch := range ex.changes {
        // Rate limit
        ex.handler.config.RateLimiter.Accept()

        var err error
        switch ch.action {
        case createRecord:
            err = ex.executeCreate(ctx, ch, metrics)
        case deleteRecord:
            err = ex.executeDelete(ctx, ch, metrics)
        default:
            err = fmt.Errorf("unknown action: %d", ch.action)
        }

        if err != nil {
            ex.log.Error(err, "failed to execute change",
                "index", i,
                "action", ch.action,
                "name", ch.req.Name.DNSName)
            errs = append(errs, err)
        } else {
            successCount++
        }
    }

    ex.log.Info("changes executed", "success", successCount, "failed", len(errs))

    if len(errs) > 0 {
        return fmt.Errorf("failed to execute %d changes: %v", len(errs), errs)
    }

    return nil
}

func (ex *execution) executeCreate(ctx context.Context, ch *change, metrics provider.Metrics) error {
    zoneID := ex.zoneID.ID
    recordName := ch.req.Name.DNSName

    for _, target := range ch.rs.Records {
        record := &Record{
            ZoneID: zoneID,
            Name:   recordName,
            Type:   string(ch.rs.Type),
            Value:  target.Value,
            TTL:    int(ch.rs.TTL),
        }

        // Handle CAA record quirk (no spaces)
        if ch.rs.Type == dns.RS_CAA {
            record.Value = strings.ReplaceAll(record.Value, " ", "")
        }

        err := ex.handler.client.CreateRecord(ctx, zoneID, record)
        if err != nil {
            return err
        }

        metrics.AddGenericRequests(provider.MetricsRequestTypeCreateRecords, 1)
    }

    return nil
}

func (ex *execution) executeDelete(ctx context.Context, ch *change, metrics provider.Metrics) error {
    if ch.recordID == "" {
        return fmt.Errorf("missing record ID for deletion")
    }

    err := ex.handler.client.DeleteRecord(ctx, ch.recordID)
    if err != nil {
        return err
    }

    metrics.AddGenericRequests(provider.MetricsRequestTypeDeleteRecords, 1)
    return nil
}
```

#### Task 4.4: Execution Tests
- [ ] Test create operations
- [ ] Test update operations (delete + create)
- [ ] Test delete operations
- [ ] Test CAA record space handling
- [ ] Test error scenarios (partial failures)
- [ ] Test metrics tracking
- [ ] Test rate limiting

### Phase 5: Comprehensive Testing (Week 3)

#### Task 5.1: Unit Tests
```go
// client_test.go
func TestClient_ListZones(t *testing.T) { /* ... */ }
func TestClient_RateLimitParsing(t *testing.T) { /* ... */ }
func TestClient_RetryOn429(t *testing.T) { /* ... */ }
func TestClient_Pagination(t *testing.T) { /* ... */ }

// handler_test.go
func TestHandler_GetZones(t *testing.T) { /* ... */ }
func TestHandler_ExecuteRequests_Create(t *testing.T) { /* ... */ }
func TestHandler_ExecuteRequests_Update(t *testing.T) { /* ... */ }
func TestHandler_ExecuteRequests_Delete(t *testing.T) { /* ... */ }

// factory_test.go
func TestAdapter_ValidateCredentials_Valid(t *testing.T) { /* ... */ }
func TestAdapter_ValidateCredentials_MissingToken(t *testing.T) { /* ... */ }
```

**Requirements**:
- Use `github.com/onsi/ginkgo/v2` and `github.com/onsi/gomega` (project standard)
- Use `httptest` for HTTP mocking
- Mock all external dependencies
- Achieve 85%+ code coverage
- Test all error paths

#### Task 5.2: Integration Tests
```go
// integration_test.go
var _ = Describe("Hetzner Provider Integration", func() {
    var (
        mockServer *httptest.Server
        handler    provider.DNSHandler
    )

    BeforeEach(func() {
        // Setup mock Hetzner API server
        mockServer = setupMockHetznerAPI()

        // Create handler with mock endpoint
        config := &provider.DNSHandlerConfig{
            Properties: map[string]string{
                "HETZNER_API_TOKEN": "test-token",
                "HETZNER_ENDPOINT":  mockServer.URL,
            },
            // ... other config
        }
        handler, _ = NewHandler(config)
    })

    AfterEach(func() {
        mockServer.Close()
    })

    It("should list zones successfully", func() {
        zones, err := handler.GetZones(context.Background())
        Expect(err).ToNot(HaveOccurred())
        Expect(zones).To(HaveLen(2))
    })

    It("should create DNS records", func() {
        // Test create flow
    })

    It("should update DNS records", func() {
        // Test update flow
    })

    It("should delete DNS records", func() {
        // Test delete flow
    })

    It("should handle rate limiting gracefully", func() {
        // Test 429 handling
    })
})
```

#### Task 5.3: E2E Tests (Optional, requires real Hetzner account)
- [ ] Setup test Hetzner DNS zone
- [ ] Test full lifecycle: create zone → create entry → update → delete
- [ ] Test with real rate limits
- [ ] Cleanup test resources

### Phase 6: Integration & Documentation (Week 3-4)

#### Task 6.1: Register Provider in Controller
Add to `/pkg/dnsman2/controller/controlplane/dnsprovider/add.go`:
```go
import (
    // ... existing
    "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/handler/hetzner"
)

var allTypes = map[string]provider.AddToRegistryFunc{
    alicloud.ProviderType:     alicloud.RegisterTo,
    aws.ProviderType:          aws.RegisterTo,
    azure.ProviderType:        azure.RegisterTo,
    azureprivate.ProviderType: azureprivate.RegisterTo,
    google.ProviderType:       google.RegisterTo,
    hetzner.ProviderType:      hetzner.RegisterTo,  // ADD THIS
}
```

#### Task 6.2: Update Makefile/Build
- [ ] Ensure `make generate` includes new provider
- [ ] Test `make build-local` builds successfully
- [ ] Test `make test` runs all new tests
- [ ] Verify `make check` passes linting

#### Task 6.3: Documentation
Create `docs/hetzner/README.md`:
```markdown
# Hetzner DNS Provider

## Overview
The Hetzner DNS provider manages DNS records in Hetzner DNS Console.

## Prerequisites
- Hetzner account with DNS Console access
- API token from https://dns.hetzner.com/settings/api-token

## Configuration

### Creating a DNSProvider
\`\`\`yaml
apiVersion: v1
kind: Secret
metadata:
  name: hetzner-credentials
  namespace: default
type: Opaque
stringData:
  HETZNER_API_TOKEN: "your-api-token-here"
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSProvider
metadata:
  name: hetzner
  namespace: default
spec:
  type: hetzner-dns
  secretRef:
    name: hetzner-credentials
  domains:
    include:
      - example.com
\`\`\`

## Supported Record Types
- A, AAAA
- CNAME
- MX
- TXT
- SRV
- CAA (spaces automatically removed)
- NS

## Limitations
- SOA records cannot be modified via API
- CAA records: spaces between parameters are automatically removed
- Rate limits: Approximately 3600 requests per hour
- Public zones only (no private DNS support)

## Rate Limiting
The provider automatically handles Hetzner's rate limits by:
- Tracking rate limit headers from API responses
- Implementing exponential backoff on 429 errors
- Respecting `Retry-After` headers
- Spreading requests to avoid quota exhaustion

## Troubleshooting
...
\`\`\`

#### Task 6.4: Update Root Documentation
Update `CLAUDE.md`:
```markdown
## DNS Provider Implementation

### Supported Providers (Next-Gen)
- AWS Route53
- Google Cloud DNS
- Azure DNS
- Azure Private DNS
- Alicloud DNS
- **Hetzner DNS** ← ADD THIS
```

### Phase 7: Code Quality & Review (Week 4)

#### Task 7.1: Code Quality Checks
```bash
# Format code
make format

# Run linters
make check

# Security scan
make sast

# Full verification
make verify-extended
```

**Quality Gates**:
- [ ] Zero golangci-lint errors
- [ ] Zero gosec vulnerabilities
- [ ] All tests passing
- [ ] 85%+ code coverage
- [ ] No TODOs or FIXMEs in final code

#### Task 7.2: Code Review Checklist
- [ ] Follows nextgen provider pattern exactly
- [ ] Proper error handling with wrapped errors
- [ ] Context propagation throughout
- [ ] Comprehensive logging with structured fields
- [ ] Metrics tracking for all API calls
- [ ] Rate limiting properly implemented
- [ ] No hardcoded values (use constants)
- [ ] Clear, self-documenting code
- [ ] Comprehensive test coverage
- [ ] Documentation complete and accurate

#### Task 7.3: Performance Testing
- [ ] Measure API call count for typical operations
- [ ] Verify rate limit handling under load
- [ ] Test with large zones (1000+ records)
- [ ] Profile memory usage
- [ ] Benchmark critical paths

## Testing Strategy

### Unit Tests (Required)
- **Coverage Target**: 85%+
- **Framework**: Ginkgo + Gomega
- **Scope**: All functions in isolation
- **Mocking**: HTTP responses, time, randomness

### Integration Tests (Required)
- **Mock API**: In-memory HTTP server simulating Hetzner API
- **Scenarios**: Complete CRUD lifecycle
- **Error Injection**: Network failures, rate limits, API errors

### E2E Tests (Optional)
- **Real API**: Actual Hetzner DNS account
- **CI/CD**: Only in secure environments with credentials
- **Cleanup**: Automated resource deletion

## Quality Metrics

### Code Quality
- Golangci-lint: **Zero errors**
- Gosec: **Zero vulnerabilities**
- Import restrictions: **All passing**
- Code coverage: **85%+ on all packages**

### Documentation
- Provider README with examples
- API quirks documented
- Troubleshooting guide
- Migration notes (if applicable)

### Performance
- API calls minimized (use batch operations where possible)
- Rate limit compliance verified
- Memory usage within project norms
- Response times acceptable

## Risk Management

### Known Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Strict rate limits | High | Implement smart request batching, respect headers |
| API changes | Medium | Version endpoint, comprehensive error handling |
| CAA record quirk | Low | Document clearly, handle automatically |
| Incomplete API docs | Medium | Reverse engineer from existing clients |
| Zone state staleness | Low | Nextgen queries DNS directly, not API |

### Validation Approach
1. **API Contract Testing**: Verify against real API early
2. **Gradual Rollout**: Test with non-critical zones first
3. **Monitoring**: Add comprehensive metrics and alerts
4. **Rollback Plan**: Provider can be disabled via controller flags

## Timeline

| Week | Phase | Deliverables | Validation |
|------|-------|--------------|------------|
| 1 | Research & Client | API client with tests | Unit tests passing, manual API verification |
| 1-2 | Factory & Validation | Provider factory | Credential validation tests |
| 2 | Handler | GetZones implementation | Zone discovery working |
| 2-3 | Execution | ExecuteRequests | Full CRUD cycle working |
| 3 | Testing | Comprehensive test suite | 85%+ coverage, all tests green |
| 3-4 | Integration & Docs | Registered provider, docs | Provider usable end-to-end |
| 4 | Quality & Review | Code review, fixes | All quality gates passed |

**Total Estimated Time**: 3-4 weeks for single developer

## Success Criteria

### Must Have
- ✅ Provider registered and discoverable
- ✅ Zone discovery working
- ✅ DNS record CRUD operations working
- ✅ Rate limiting implemented and tested
- ✅ 85%+ code coverage
- ✅ Zero linter errors
- ✅ Documentation complete
- ✅ Integration with nextgen controller

### Should Have
- ✅ Comprehensive error messages
- ✅ Performance optimizations
- ✅ Metrics and logging
- ✅ Edge case handling (CAA records, etc.)

### Nice to Have
- ⭕ E2E tests with real API
- ⭕ Performance benchmarks
- ⭕ Comparison with other providers
- ⭕ Load testing results

## References

### Official Documentation
- [Hetzner DNS API Docs](https://dns.hetzner.com/api-docs)
- [Hetzner DNS Console](https://docs.hetzner.com/dns-console/dns/)
- [API Token Creation](https://docs.hetzner.com/dns-console/dns/general/api-access-token/)

### Existing Implementations
- [jobstoit/hetzner-dns-go](https://github.com/jobstoit/hetzner-dns-go) - Go library reference
- [nl2go/hetzner-dns-go](https://github.com/nl2go/hetzner-dns-go) - Alternative implementation
- [Lego Hetzner Provider](https://go-acme.github.io/lego/dns/hetzner/) - ACME client implementation
- [DNSControl Hetzner](https://docs.dnscontrol.org/provider/hetzner) - DNS management tool

### Internal References
- [CLAUDE.md](CLAUDE.md) - Project development guide
- [pkg/dnsman2/dns/provider/](pkg/dnsman2/dns/provider/) - Provider architecture
- [pkg/dnsman2/dns/provider/handler/aws/](pkg/dnsman2/dns/provider/handler/aws/) - Complex provider example
- [pkg/dnsman2/dns/provider/handler/google/](pkg/dnsman2/dns/provider/handler/google/) - Simple provider example

## Next Steps

1. **Review this plan** with team/maintainers
2. **Access Hetzner DNS API** documentation and test credentials
3. **Start Phase 1** (Research & API Client)
4. **Daily standups** to track progress
5. **Code reviews** after each phase

## Notes

- This is a **nextgen-only** implementation (no classic provider)
- Focus on **code quality over speed** - this is a reference implementation
- **Test coverage is mandatory** - no exceptions
- **Documentation is part of done** - not optional
- Follow the **existing nextgen patterns exactly** - don't invent new approaches
