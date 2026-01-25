// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bunny

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Execution helpers", func() {
	Describe("makeRecordKey", func() {
		It("should create a consistent key", func() {
			key := makeRecordKey("www.example.com", "A")
			Expect(key).To(Equal("www.example.com:A"))
		})

		It("should remove trailing dots", func() {
			key := makeRecordKey("www.example.com.", "A")
			Expect(key).To(Equal("www.example.com:A"))
		})
	})

	Describe("parseMXValue", func() {
		It("should parse standard MX format", func() {
			priority, value := parseMXValue("10 mail.example.com")
			Expect(priority).To(Equal(10))
			Expect(value).To(Equal("mail.example.com"))
		})

		It("should use default priority for invalid format", func() {
			priority, value := parseMXValue("mail.example.com")
			Expect(priority).To(Equal(10)) // Default priority
			Expect(value).To(Equal("mail.example.com"))
		})

		It("should handle high priority values", func() {
			priority, value := parseMXValue("100 backup.mail.example.com")
			Expect(priority).To(Equal(100))
			Expect(value).To(Equal("backup.mail.example.com"))
		})
	})

	Describe("parseSRVValue", func() {
		It("should parse standard SRV format", func() {
			priority, weight, port, value := parseSRVValue("10 5 443 server.example.com")
			Expect(priority).To(Equal(10))
			Expect(weight).To(Equal(5))
			Expect(port).To(Equal(443))
			Expect(value).To(Equal("server.example.com"))
		})

		It("should handle invalid format", func() {
			priority, weight, port, value := parseSRVValue("server.example.com")
			Expect(priority).To(Equal(0))
			Expect(weight).To(Equal(0))
			Expect(port).To(Equal(0))
			Expect(value).To(Equal("server.example.com"))
		})
	})

	Describe("parseCAAValue", func() {
		It("should parse standard CAA format", func() {
			flags, tag, value := parseCAAValue("0 issue \"letsencrypt.org\"")
			Expect(flags).To(Equal(0))
			Expect(tag).To(Equal("issue"))
			Expect(value).To(Equal("letsencrypt.org"))
		})

		It("should handle issuewild tag", func() {
			flags, tag, value := parseCAAValue("0 issuewild \"letsencrypt.org\"")
			Expect(flags).To(Equal(0))
			Expect(tag).To(Equal("issuewild"))
			Expect(value).To(Equal("letsencrypt.org"))
		})

		It("should handle iodef tag", func() {
			flags, tag, value := parseCAAValue("0 iodef \"mailto:admin@example.com\"")
			Expect(flags).To(Equal(0))
			Expect(tag).To(Equal("iodef"))
			Expect(value).To(Equal("mailto:admin@example.com"))
		})

		It("should handle critical flag", func() {
			flags, tag, value := parseCAAValue("128 issue \"ca.example.com\"")
			Expect(flags).To(Equal(128))
			Expect(tag).To(Equal("issue"))
			Expect(value).To(Equal("ca.example.com"))
		})

		It("should handle invalid format", func() {
			flags, tag, value := parseCAAValue("invalid")
			Expect(flags).To(Equal(0))
			Expect(tag).To(Equal("issue")) // Default tag
			Expect(value).To(Equal("invalid"))
		})
	})

	Describe("RecordTypeToString", func() {
		It("should convert known types", func() {
			Expect(RecordTypeToString(RecordTypeA)).To(Equal("A"))
			Expect(RecordTypeToString(RecordTypeAAAA)).To(Equal("AAAA"))
			Expect(RecordTypeToString(RecordTypeCNAME)).To(Equal("CNAME"))
			Expect(RecordTypeToString(RecordTypeTXT)).To(Equal("TXT"))
			Expect(RecordTypeToString(RecordTypeMX)).To(Equal("MX"))
			Expect(RecordTypeToString(RecordTypeSRV)).To(Equal("SRV"))
			Expect(RecordTypeToString(RecordTypeCAA)).To(Equal("CAA"))
			Expect(RecordTypeToString(RecordTypeNS)).To(Equal("NS"))
		})

		It("should return Unknown for invalid types", func() {
			Expect(RecordTypeToString(999)).To(Equal("Unknown"))
		})
	})

	Describe("StringToRecordType", func() {
		It("should convert known types", func() {
			Expect(StringToRecordType("A")).To(Equal(RecordTypeA))
			Expect(StringToRecordType("AAAA")).To(Equal(RecordTypeAAAA))
			Expect(StringToRecordType("CNAME")).To(Equal(RecordTypeCNAME))
			Expect(StringToRecordType("TXT")).To(Equal(RecordTypeTXT))
			Expect(StringToRecordType("MX")).To(Equal(RecordTypeMX))
			Expect(StringToRecordType("SRV")).To(Equal(RecordTypeSRV))
			Expect(StringToRecordType("CAA")).To(Equal(RecordTypeCAA))
			Expect(StringToRecordType("NS")).To(Equal(RecordTypeNS))
		})

		It("should return -1 for unknown types", func() {
			Expect(StringToRecordType("UNKNOWN")).To(Equal(-1))
			Expect(StringToRecordType("")).To(Equal(-1))
		})
	})
})
