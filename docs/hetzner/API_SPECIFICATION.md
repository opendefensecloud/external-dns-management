# Hetzner DNS API Specification

This document provides a comprehensive specification of the Hetzner DNS API based on research of the [jobstoit/hetzner-dns-go](https://github.com/jobstoit/hetzner-dns-go) library and other sources.

## Base Information

- **Base URL**: `https://dns.hetzner.com/api/v1`
- **Protocol**: HTTPS only
- **Authentication**: Bearer token via `Auth-API-Token` HTTP header
- **Content-Type**: `application/json`
- **User-Agent**: Recommended format: `{application}/{version} hetzner-dns/{sdk-version}`

## Authentication

All API requests require authentication using an API token in the request header:

```http
Auth-API-Token: {your-api-token}
```

**Token characteristics:**
- Length: Typically 32+ characters
- Format: Alphanumeric (a-zA-Z0-9)
- Obtained from: https://dns.hetzner.com/settings/api-token
- Security: Tokens should be treated as sensitive credentials

## Rate Limiting

Based on external sources, Hetzner DNS implements rate limiting:

- **Headers** (expected in responses):
  - `Ratelimit-Limit`: Total requests allowed in the current time window
  - `Ratelimit-Remaining`: Remaining requests in current window
  - `Ratelimit-Reset`: Unix timestamp when the limit resets

- **Behavior on limit exceeded**:
  - HTTP Status: `429 Too Many Requests`
  - Header: `Retry-After` - seconds to wait before retrying

- **Best practices**:
  - Burst through first half of quota
  - Spread remaining requests evenly across time window
  - Respect `Retry-After` header on 429 responses
  - Implement exponential backoff for retries

## Pagination

List endpoints support pagination via query parameters:

- `page`: Page number (1-indexed)
- `per_page`: Number of items per page

**Response metadata:**
```json
{
  "meta": {
    "pagination": {
      "page": 1,
      "per_page": 100,
      "last_page": 3,
      "total_entries": 250
    }
  }
}
```

## Endpoints

### Zones

#### List Zones
```http
GET /zones
GET /zones?page=1&per_page=100
GET /zones?name=example.com
GET /zones?search_name=example
```

**Query Parameters:**
- `page` (int): Page number
- `per_page` (int): Items per page
- `name` (string): Filter by exact zone name
- `search_name` (string): Search zones by name substring

**Response:**
```json
{
  "zones": [
    {
      "id": "zone-id-123",
      "created": "2023-01-15 10:30:00.123 +0000 UTC",
      "modified": "2023-01-16 14:20:15.456 +0000 UTC",
      "legacy_dns_host": "",
      "legacy_ns": [],
      "name": "example.com",
      "ns": [
        "hydrogen.ns.hetzner.com",
        "helium.ns.hetzner.de",
        "oxygen.ns.hetzner.com"
      ],
      "owner": "owner-id",
      "paused": false,
      "permission": "owner",
      "project": "",
      "registrar": "",
      "status": "verified",
      "ttl": 86400,
      "verified": "2023-01-15 11:00:00.000 +0000 UTC",
      "records_count": 42,
      "is_secondary_dns": false,
      "txt_verification": {
        "name": "_validation.example.com",
        "token": "verification-token-here"
      }
    }
  ],
  "meta": {
    "pagination": {
      "page": 1,
      "per_page": 100,
      "last_page": 1,
      "total_entries": 1
    }
  }
}
```

#### Get Zone
```http
GET /zones/{zone_id}
```

**Response:**
```json
{
  "zone": {
    "id": "zone-id-123",
    "name": "example.com",
    ...
  }
}
```

#### Create Zone
```http
POST /zones
Content-Type: application/json

{
  "name": "example.com",
  "ttl": 86400
}
```

**Request Body:**
- `name` (string, required): Zone domain name
- `ttl` (int, optional): Default TTL for records

**Response:** Same as Get Zone

#### Update Zone
```http
PUT /zones/{zone_id}
Content-Type: application/json

{
  "name": "example.com",
  "ttl": 3600
}
```

**Request Body:** Same as Create Zone

**Response:** Same as Get Zone

#### Delete Zone
```http
DELETE /zones/{zone_id}
```

**Response:** HTTP 200 (empty body)

### Records

#### List Records
```http
GET /records
GET /records?zone_id={zone_id}
GET /records?zone_id={zone_id}&page=1&per_page=100
```

**Query Parameters:**
- `zone_id` (string): Filter by zone ID
- `page` (int): Page number
- `per_page` (int): Items per page

**Response:**
```json
{
  "records": [
    {
      "id": "record-id-456",
      "type": "A",
      "created": "2023-01-15 10:35:00.123 +0000 UTC",
      "modified": "2023-01-15 10:35:00.123 +0000 UTC",
      "zone_id": "zone-id-123",
      "name": "www",
      "value": "192.0.2.1",
      "ttl": 3600
    },
    {
      "id": "record-id-789",
      "type": "CNAME",
      "created": "2023-01-15 10:36:00.123 +0000 UTC",
      "modified": "2023-01-15 10:36:00.123 +0000 UTC",
      "zone_id": "zone-id-123",
      "name": "blog",
      "value": "www.example.com",
      "ttl": 3600
    }
  ],
  "meta": {
    "pagination": {
      "page": 1,
      "per_page": 100,
      "last_page": 1,
      "total_entries": 2
    }
  }
}
```

#### Get Record
```http
GET /records/{record_id}
```

**Response:**
```json
{
  "record": {
    "id": "record-id-456",
    "type": "A",
    ...
  }
}
```

#### Create Record
```http
POST /records
Content-Type: application/json

{
  "zone_id": "zone-id-123",
  "type": "A",
  "name": "www",
  "value": "192.0.2.1",
  "ttl": 3600
}
```

**Request Body:**
- `zone_id` (string, required): Zone ID
- `type` (string, required): Record type (see Supported Record Types)
- `name` (string, required): Record name (without zone suffix)
- `value` (string, required): Record value
- `ttl` (int, optional): TTL in seconds (inherits from zone if not specified)

**Response:** Same as Get Record

#### Update Record
```http
PUT /records/{record_id}
Content-Type: application/json

{
  "zone_id": "zone-id-123",
  "type": "A",
  "name": "www",
  "value": "192.0.2.2",
  "ttl": 3600
}
```

**Request Body:** Same as Create Record

**Response:** Same as Get Record

#### Delete Record
```http
DELETE /records/{record_id}
```

**Response:** HTTP 200 (empty body)

#### Bulk Create Records
```http
POST /records/bulk
Content-Type: application/json

{
  "records": [
    {
      "zone_id": "zone-id-123",
      "type": "A",
      "name": "server1",
      "value": "192.0.2.1",
      "ttl": 3600
    },
    {
      "zone_id": "zone-id-123",
      "type": "A",
      "name": "server2",
      "value": "192.0.2.2",
      "ttl": 3600
    }
  ]
}
```

**Response:**
```json
{
  "records": [...],
  "valid_records": [...],
  "invalid_records": [...]
}
```

#### Bulk Update Records
```http
PUT /records/bulk
Content-Type: application/json

{
  "records": [
    {
      "id": "record-id-1",
      "zone_id": "zone-id-123",
      "type": "A",
      "name": "server1",
      "value": "192.0.2.10",
      "ttl": 3600
    }
  ]
}
```

**Response:**
```json
{
  "records": [...],
  "failed_records": [...]
}
```

## Supported Record Types

Based on the library code, the following record types are supported:

- `A` - IPv4 address
- `AAAA` - IPv6 address
- `CNAME` - Canonical name
- `MX` - Mail exchange
- `TXT` - Text record
- `SRV` - Service record
- `CAA` - Certification Authority Authorization
- `NS` - Name server
- `PTR` - Pointer record
- `RP` - Responsible person
- `SOA` - Start of authority (read-only via API)
- `HINFO` - Host information
- `DANE` - DNS-based Authentication of Named Entities
- `TLSA` - TLS Authentication
- `DS` - Delegation Signer

**Note:** SOA records cannot be modified via the API according to external documentation.

## Zone Status Values

- `verified` - Zone is verified and active
- `pending` - Zone verification pending
- `failed` - Zone verification failed

## Error Responses

### HTTP Status Codes

- `200 OK` - Successful request
- `400 Bad Request` - Invalid request (missing required fields, validation errors)
- `401 Unauthorized` - Invalid or missing API token
- `403 Forbidden` - Insufficient permissions for the resource
- `404 Not Found` - Resource not found
- `409 Conflict` - Resource conflict (e.g., duplicate zone)
- `422 Unprocessable Entity` - Validation error
- `429 Too Many Requests` - Rate limit exceeded
- `500 Internal Server Error` - Server error
- `503 Service Unavailable` - Service temporarily unavailable

### Error Response Format

```json
{
  "error": {
    "message": "Error description",
    "code": "ERROR_CODE"
  }
}
```

## Known API Quirks and Limitations

Based on research from [DNSControl Hetzner provider](https://docs.dnscontrol.org/provider/hetzner):

1. **CAA Records**: As of June 2022, CAA record values must not contain spaces between parameters. Spaces must be removed for the API to accept them.
   - Incorrect: `0 issue "letsencrypt.org"`
   - Correct: `0issue"letsencrypt.org"`

2. **SOA Records**: Cannot be modified via the API despite zone import functionality existing.

3. **Rate Limiting**: Heavily rate-limited. The API advertises limits via response headers. Best practice is to burst through half the quota, then spread requests evenly across the remaining time window.

4. **Public Zones Only**: Hetzner DNS only supports public DNS zones. No private DNS zones are available.

5. **Token Validation**: Tokens must match the pattern `[a-zA-Z0-9]{32}` (32 alphanumeric characters minimum).

## Implementation Notes for external-dns-management

### Required Operations

The nextgen provider needs to implement:

1. **GetZones()**: List all accessible zones
   - Use pagination to handle large zone lists
   - Track rate limits from response headers

2. **ListRecords(zoneID)**: Get all records for a zone
   - Required to build record lookup map for updates/deletes
   - Use pagination for zones with many records

3. **CreateRecord(zoneID, record)**: Create a single record
   - May want to batch using bulk create endpoint for efficiency

4. **DeleteRecord(recordID)**: Delete a record by ID
   - Requires fetching records first to get IDs

5. **UpdateRecord**: Hetzner has a native update endpoint
   - Can use delete + create pattern (common in external-dns-management)
   - Or use native PUT /records/{id} endpoint

### Rate Limiting Strategy

1. Parse rate limit headers from every response:
   ```go
   limit, _ := strconv.Atoi(resp.Header.Get("Ratelimit-Limit"))
   remaining, _ := strconv.Atoi(resp.Header.Get("Ratelimit-Remaining"))
   reset, _ := strconv.ParseInt(resp.Header.Get("Ratelimit-Reset"), 10, 64)
   ```

2. Implement exponential backoff on 429 responses:
   ```go
   if resp.StatusCode == 429 {
       retryAfter := resp.Header.Get("Retry-After")
       // Wait and retry
   }
   ```

3. Use the framework's rate limiter (passed in DNSHandlerConfig) in addition to response-based rate limiting

### CAA Record Handling

Automatically remove spaces from CAA record values before sending to API:

```go
if recordType == "CAA" {
    value = strings.ReplaceAll(value, " ", "")
}
```

### Record Name Handling

- API expects record names WITHOUT the zone suffix
- `www.example.com` for zone `example.com` should be sent as `www`
- `@` or empty string for apex records

### Bulk Operations

Consider using bulk endpoints for efficiency:
- POST /records/bulk for creating multiple records
- PUT /records/bulk for updating multiple records
- Reduces API calls and rate limit pressure

## Testing Considerations

1. **Mock Server**: Create comprehensive mock responses including:
   - Pagination scenarios
   - Rate limit headers
   - Error responses (401, 404, 429, 500)

2. **Rate Limit Testing**: Simulate 429 responses with Retry-After

3. **CAA Record Testing**: Verify space removal in CAA records

4. **Pagination Testing**: Test with large zone/record lists

5. **Error Handling**: Test all error status codes

## References

- [jobstoit/hetzner-dns-go](https://github.com/jobstoit/hetzner-dns-go) - Primary reference
- [Hetzner DNS API Docs](https://dns.hetzner.com/api-docs) - Official documentation
- [DNSControl Hetzner Provider](https://docs.dnscontrol.org/provider/hetzner) - Known quirks and limitations
- [Lego Hetzner DNS Provider](https://go-acme.github.io/lego/dns/hetzner/) - Configuration reference
