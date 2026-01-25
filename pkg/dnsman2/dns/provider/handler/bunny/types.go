// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

import "time"

// Zone represents a DNS zone in Bunny DNS.
type Zone struct {
	ID                       int64     `json:"Id"`
	Domain                   string    `json:"Domain"`
	Records                  []Record  `json:"Records,omitempty"`
	DateModified             time.Time `json:"DateModified"`
	DateCreated              time.Time `json:"DateCreated"`
	NameserversDetected      bool      `json:"NameserversDetected"`
	CustomNameserversEnabled bool      `json:"CustomNameserversEnabled"`
	Nameserver1              string    `json:"Nameserver1,omitempty"`
	Nameserver2              string    `json:"Nameserver2,omitempty"`
	SoaEmail                 string    `json:"SoaEmail,omitempty"`
	NameserversNextCheck     time.Time `json:"NameserversNextCheck,omitempty"`
	LoggingEnabled           bool      `json:"LoggingEnabled"`
	LoggingIPAnonymizingMode int       `json:"LoggingIPAnonymizingMode"`
	LogAnonymizationType     int       `json:"LogAnonymizationType"`
}

// ZoneListResponse is the response for listing zones.
type ZoneListResponse struct {
	Items        []Zone `json:"Items"`
	CurrentPage  int    `json:"CurrentPage"`
	TotalItems   int    `json:"TotalItems"`
	HasMoreItems bool   `json:"HasMoreItems"`
}

// Record represents a DNS record in Bunny DNS.
type Record struct {
	ID                    int64    `json:"Id,omitempty"`
	Type                  int      `json:"Type"`
	TTL                   int      `json:"Ttl"`
	Value                 string   `json:"Value"`
	Name                  string   `json:"Name"`
	Weight                int      `json:"Weight,omitempty"`
	Priority              int      `json:"Priority,omitempty"`
	Port                  int      `json:"Port,omitempty"`
	Flags                 int      `json:"Flags,omitempty"`
	Tag                   string   `json:"Tag,omitempty"`
	Accelerated           bool     `json:"Accelerated,omitempty"`
	AcceleratedPullZoneID int64    `json:"AcceleratedPullZoneId,omitempty"`
	LinkName              string   `json:"LinkName,omitempty"`
	IPGeoLocationInfo     *IPGeo   `json:"IPGeoLocationInfo,omitempty"`
	MonitorStatus         int      `json:"MonitorStatus,omitempty"`
	MonitorType           int      `json:"MonitorType,omitempty"`
	GeolocationLatitude   float64  `json:"GeolocationLatitude,omitempty"`
	GeolocationLongitude  float64  `json:"GeolocationLongitude,omitempty"`
	EnviromentVariables   []EnvVar `json:"EnviromentVariables,omitempty"`
	LatencyZone           string   `json:"LatencyZone,omitempty"`
	SmartRoutingType      int      `json:"SmartRoutingType,omitempty"`
	Disabled              bool     `json:"Disabled,omitempty"`
	Comment               string   `json:"Comment,omitempty"`
}

// IPGeo contains IP geolocation information.
type IPGeo struct {
	CountryCode      string  `json:"CountryCode,omitempty"`
	Country          string  `json:"Country,omitempty"`
	ASN              int     `json:"ASN,omitempty"`
	OrganizationName string  `json:"OrganizationName,omitempty"`
	City             string  `json:"City,omitempty"`
	Latitude         float64 `json:"Latitude,omitempty"`
	Longitude        float64 `json:"Longitude,omitempty"`
}

// EnvVar represents an environment variable for Script records.
type EnvVar struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

// RecordCreateRequest is the request for creating a record.
type RecordCreateRequest struct {
	Type     int    `json:"Type"`
	TTL      int    `json:"Ttl"`
	Value    string `json:"Value"`
	Name     string `json:"Name"`
	Weight   int    `json:"Weight,omitempty"`
	Priority int    `json:"Priority,omitempty"`
	Port     int    `json:"Port,omitempty"`
	Flags    int    `json:"Flags,omitempty"`
	Tag      string `json:"Tag,omitempty"`
	Comment  string `json:"Comment,omitempty"`
}

// RecordUpdateRequest is the request for updating a record.
type RecordUpdateRequest struct {
	Type     int    `json:"Type,omitempty"`
	TTL      int    `json:"Ttl,omitempty"`
	Value    string `json:"Value,omitempty"`
	Name     string `json:"Name,omitempty"`
	Weight   int    `json:"Weight,omitempty"`
	Priority int    `json:"Priority,omitempty"`
	Port     int    `json:"Port,omitempty"`
	Flags    int    `json:"Flags,omitempty"`
	Tag      string `json:"Tag,omitempty"`
	Comment  string `json:"Comment,omitempty"`
}

// ZoneCreateRequest is the request for creating a zone.
type ZoneCreateRequest struct {
	Domain string `json:"Domain"`
}

// ErrorResponse represents an error response from the API.
type ErrorResponse struct {
	ErrorKey string `json:"ErrorKey,omitempty"`
	Field    string `json:"Field,omitempty"`
	Message  string `json:"Message"`
}

// RateLimitInfo contains rate limiting information from response headers.
type RateLimitInfo struct {
	Limit     int       // Total requests allowed in current window
	Remaining int       // Remaining requests in current window
	Reset     time.Time // When the limit resets
}

// RecordType constants for Bunny DNS record types.
const (
	RecordTypeA        = 0
	RecordTypeAAAA     = 1
	RecordTypeCNAME    = 2
	RecordTypeTXT      = 3
	RecordTypeMX       = 4
	RecordTypeRedirect = 5
	RecordTypeFlatten  = 6
	RecordTypePullZone = 7
	RecordTypeSRV      = 8
	RecordTypeCAA      = 9
	RecordTypePTR      = 10
	RecordTypeScript   = 11
	RecordTypeNS       = 12
)

// RecordTypeToString converts a Bunny record type integer to a string.
func RecordTypeToString(t int) string {
	switch t {
	case RecordTypeA:
		return "A"
	case RecordTypeAAAA:
		return "AAAA"
	case RecordTypeCNAME:
		return "CNAME"
	case RecordTypeTXT:
		return "TXT"
	case RecordTypeMX:
		return "MX"
	case RecordTypeRedirect:
		return "Redirect"
	case RecordTypeFlatten:
		return "Flatten"
	case RecordTypePullZone:
		return "PullZone"
	case RecordTypeSRV:
		return "SRV"
	case RecordTypeCAA:
		return "CAA"
	case RecordTypePTR:
		return "PTR"
	case RecordTypeScript:
		return "Script"
	case RecordTypeNS:
		return "NS"
	default:
		return "Unknown"
	}
}

// StringToRecordType converts a string record type to a Bunny record type integer.
func StringToRecordType(s string) int {
	switch s {
	case "A":
		return RecordTypeA
	case "AAAA":
		return RecordTypeAAAA
	case "CNAME":
		return RecordTypeCNAME
	case "TXT":
		return RecordTypeTXT
	case "MX":
		return RecordTypeMX
	case "Redirect":
		return RecordTypeRedirect
	case "Flatten":
		return RecordTypeFlatten
	case "PullZone":
		return RecordTypePullZone
	case "SRV":
		return RecordTypeSRV
	case "CAA":
		return RecordTypeCAA
	case "PTR":
		return RecordTypePTR
	case "Script":
		return RecordTypeScript
	case "NS":
		return RecordTypeNS
	default:
		return -1
	}
}
