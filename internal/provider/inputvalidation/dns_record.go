// Copyright (c) 2026 Alex Ackerman
// SPDX-License-Identifier: MPL-2.0

package inputvalidation

import (
	"context"
	"fmt"
)

// validRecordTypes is the set of record types supported by the provider.
var validRecordTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "MX": true,
	"NS": true, "PTR": true, "SRV": true, "TXT": true, "CAA": true, "FWD": true,
	"APP": true,
}

// registerRecordRules adds all DNS record validation rules to the registry.
func registerRecordRules(r *Registry) {
	r.Register(validateRecordType())
	r.Register(validateARecord())
	r.Register(validateAAAARecord())
	r.Register(validateTXTRecord())
	r.Register(validateCNAMERecord())
	r.Register(validateMXRecord())
	r.Register(validateNSRecord())
	r.Register(validatePTRRecord())
	r.Register(validateSRVRecord())
	r.Register(validateCAARecord())
	r.Register(validateFWDRecord())
	r.Register(validateAPPRecord())
}

// DefaultRegistry returns a registry pre-loaded with all built-in validation rules.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	registerRecordRules(r)
	return r
}

// ---------------------------------------------------------------------------
// Record type validator
// ---------------------------------------------------------------------------

func validateRecordType() ValidationRule {
	return ValidationRule{
		Name:        "record_type",
		Description: "Validates the record type is one of the supported types",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok {
				return nil
			}
			if !validRecordTypes[rt] {
				return []Finding{{
					Attribute: "type",
					Summary:   fmt.Sprintf("Invalid record type: %q", rt),
					Detail:    "Supported record types are: A, AAAA, CNAME, MX, NS, PTR, SRV, TXT, CAA, FWD, APP (case-sensitive).",
				}}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// A record
// ---------------------------------------------------------------------------

func validateARecord() ValidationRule {
	return ValidationRule{
		Name:        "a_record_ipv4",
		Description: "A record value must be a valid IPv4 address",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "A" {
				return nil
			}
			value, ok := config.GetString("value")
			if !ok {
				return nil
			}
			if !isValidIPv4(value) {
				return []Finding{{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid A record value: %q is not a valid IPv4 address", value),
					Detail:    `A records require a valid IPv4 address (e.g., "192.0.2.1").`,
				}}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// AAAA record
// ---------------------------------------------------------------------------

func validateAAAARecord() ValidationRule {
	return ValidationRule{
		Name:        "aaaa_record_ipv6",
		Description: "AAAA record value must be a valid IPv6 address",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "AAAA" {
				return nil
			}
			value, ok := config.GetString("value")
			if !ok {
				return nil
			}
			if !isValidIPv6(value) {
				return []Finding{{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid AAAA record value: %q is not a valid IPv6 address", value),
					Detail:    `AAAA records require a valid IPv6 address (e.g., "2001:db8::1").`,
				}}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// TXT record
// ---------------------------------------------------------------------------

func validateTXTRecord() ValidationRule {
	return ValidationRule{
		Name:        "txt_record_nonempty",
		Description: "TXT record value must not be empty",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "TXT" {
				return nil
			}
			value, ok := config.GetString("value")
			if !ok {
				return nil
			}
			if value == "" {
				return []Finding{{
					Attribute: "value",
					Summary:   "Invalid TXT record value: value must not be empty",
					Detail:    `TXT records require a non-empty text value (e.g., "v=spf1 -all").`,
				}}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// CNAME record
// ---------------------------------------------------------------------------

func validateCNAMERecord() ValidationRule {
	return ValidationRule{
		Name:        "cname_record_fqdn",
		Description: "CNAME record value must be a valid FQDN",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "CNAME" {
				return nil
			}
			value, ok := config.GetString("value")
			if !ok {
				return nil
			}
			if !isValidFQDN(value) {
				return []Finding{{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid CNAME record value: %q is not a valid FQDN", value),
					Detail:    `CNAME records require a valid fully qualified domain name (e.g., "target.example.com.").`,
				}}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// MX record
// ---------------------------------------------------------------------------

func validateMXRecord() ValidationRule {
	return ValidationRule{
		Name:        "mx_record",
		Description: "MX record: value must be FQDN, priority required and in range",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "MX" {
				return nil
			}
			var findings []Finding

			priority, hasPriority := config.GetInt64("priority")
			if !hasPriority {
				findings = append(findings, Finding{
					Attribute: "priority",
					Summary:   "MX record missing required field: priority",
					Detail:    "MX records require a priority value (0-65535).",
				})
			} else if !isInRange(priority, 0, 65535) {
				findings = append(findings, Finding{
					Attribute: "priority",
					Summary:   fmt.Sprintf("Invalid MX record priority: %d is out of range", priority),
					Detail:    "MX record priority must be between 0 and 65535.",
				})
			}

			value, ok := config.GetString("value")
			if !ok {
				return findings
			}
			if !isValidFQDN(value) {
				findings = append(findings, Finding{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid MX record value: %q is not a valid FQDN", value),
					Detail:    `MX records require a valid fully qualified domain name (e.g., "mail.example.com.").`,
				})
			}

			return findings
		},
	}
}

// ---------------------------------------------------------------------------
// NS record
// ---------------------------------------------------------------------------

func validateNSRecord() ValidationRule {
	return ValidationRule{
		Name:        "ns_record_fqdn",
		Description: "NS record value must be a valid FQDN",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "NS" {
				return nil
			}
			value, ok := config.GetString("value")
			if !ok {
				return nil
			}
			if !isValidFQDN(value) {
				return []Finding{{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid NS record value: %q is not a valid FQDN", value),
					Detail:    `NS records require a valid fully qualified domain name (e.g., "ns1.example.com.").`,
				}}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// PTR record
// ---------------------------------------------------------------------------

func validatePTRRecord() ValidationRule {
	return ValidationRule{
		Name:        "ptr_record_hostname",
		Description: "PTR record value must be a valid hostname (single-label or FQDN)",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "PTR" {
				return nil
			}
			value, ok := config.GetString("value")
			if !ok {
				return nil
			}
			if !isValidHostname(value) {
				return []Finding{{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid PTR record value: %q is not a valid hostname", value),
					Detail:    `PTR records require a valid hostname — either a single label (e.g., "rancher") or FQDN (e.g., "rancher.example.com.").`,
				}}
			}
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// SRV record
// ---------------------------------------------------------------------------

func validateSRVRecord() ValidationRule {
	return ValidationRule{
		Name:        "srv_record",
		Description: "SRV record: target must be FQDN, priority/weight/port required and in range",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "SRV" {
				return nil
			}
			var findings []Finding

			type numField struct {
				name string
				attr string
			}
			for _, f := range []numField{
				{"priority", "priority"},
				{"weight", "weight"},
				{"port", "port"},
			} {
				val, has := config.GetInt64(f.attr)
				if !has {
					findings = append(findings, Finding{
						Attribute: f.attr,
						Summary:   fmt.Sprintf("SRV record missing required field: %s", f.name),
						Detail:    fmt.Sprintf("SRV records require %s (0-65535).", f.name),
					})
				} else if !isInRange(val, 0, 65535) {
					findings = append(findings, Finding{
						Attribute: f.attr,
						Summary:   fmt.Sprintf("Invalid SRV record %s: %d is out of range", f.name, val),
						Detail:    fmt.Sprintf("SRV record %s must be between 0 and 65535.", f.name),
					})
				}
			}

			value, ok := config.GetString("value")
			if !ok {
				return findings
			}
			if !isValidFQDN(value) {
				findings = append(findings, Finding{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid SRV record target: %q is not a valid FQDN", value),
					Detail:    `SRV records require a valid fully qualified domain name as target (e.g., "sip.example.com.").`,
				})
			}

			return findings
		},
	}
}

// ---------------------------------------------------------------------------
// CAA record
// ---------------------------------------------------------------------------

var validCAATags = map[string]bool{
	"issue":     true,
	"issuewild": true,
	"iodef":     true,
}

func validateCAARecord() ValidationRule {
	return ValidationRule{
		Name:        "caa_record",
		Description: "CAA record: flags must be 0/128, tag must be issue/issuewild/iodef, value non-empty",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "CAA" {
				return nil
			}
			var findings []Finding

			flags, hasFlags := config.GetInt64("caa_flags")
			if !hasFlags {
				findings = append(findings, Finding{
					Attribute: "caa_flags",
					Summary:   "CAA record missing required field: caa_flags",
					Detail:    "CAA records require caa_flags (0 = non-critical, 128 = critical).",
				})
			} else if flags != 0 && flags != 128 {
				findings = append(findings, Finding{
					Attribute: "caa_flags",
					Summary:   fmt.Sprintf("Invalid CAA record caa_flags: %d is not valid", flags),
					Detail:    "CAA record caa_flags must be 0 (non-critical) or 128 (critical).",
				})
			}

			tag, hasTag := config.GetString("caa_tag")
			if !hasTag {
				findings = append(findings, Finding{
					Attribute: "caa_tag",
					Summary:   "CAA record missing required field: caa_tag",
					Detail:    `CAA records require caa_tag: one of "issue", "issuewild", "iodef".`,
				})
			} else if !validCAATags[tag] {
				findings = append(findings, Finding{
					Attribute: "caa_tag",
					Summary:   fmt.Sprintf("Invalid CAA record caa_tag: %q is not a recognized CAA tag", tag),
					Detail:    `CAA records require one of: "issue", "issuewild", "iodef".`,
				})
			}

			value, ok := config.GetString("value")
			if ok && value == "" {
				findings = append(findings, Finding{
					Attribute: "value",
					Summary:   "Invalid CAA record value: value must not be empty",
					Detail:    `CAA records require a non-empty value (e.g., "letsencrypt.org").`,
				})
			}

			return findings
		},
	}
}

// ---------------------------------------------------------------------------
// FWD record
// ---------------------------------------------------------------------------

var validFWDProtocols = map[string]bool{
	"Udp": true, "Tcp": true, "Tls": true, "Https": true, "Quic": true,
}

func validateFWDRecord() ValidationRule {
	return ValidationRule{
		Name:        "fwd_record",
		Description: "FWD record: value must be non-empty and protocol must be valid",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "FWD" {
				return nil
			}
			var findings []Finding

			value, ok := config.GetString("value")
			if ok && value == "" {
				findings = append(findings, Finding{
					Attribute: "value",
					Summary:   "Invalid FWD record value: forwarder address must not be empty",
					Detail:    `FWD records require a forwarder address such as "1.1.1.1" or "dns.quad9.net:853 (9.9.9.9)".`,
				})
			}

			protocol, hasProtocol := config.GetString("protocol")
			if hasProtocol && protocol != "" && !validFWDProtocols[protocol] {
				findings = append(findings, Finding{
					Attribute: "protocol",
					Summary:   fmt.Sprintf("Invalid FWD record protocol: %q is not supported", protocol),
					Detail:    `FWD record protocol must be one of: "Udp", "Tcp", "Tls", "Https", "Quic".`,
				})
			}

			if priority, hasPriority := config.GetInt64("forwarder_priority"); hasPriority && priority < 0 {
				findings = append(findings, Finding{
					Attribute: "forwarder_priority",
					Summary:   fmt.Sprintf("Invalid FWD record forwarder_priority: %d is negative", priority),
					Detail:    "FWD record forwarder_priority must be zero or greater; lower values are queried first.",
				})
			}

			return findings
		},
	}
}

// ---------------------------------------------------------------------------
// APP record
// ---------------------------------------------------------------------------

func validateAPPRecord() ValidationRule {
	return ValidationRule{
		Name:        "app_record",
		Description: "APP record: app_name required, value must be a class path, record_data must be valid JSON when it looks like JSON",
		Resource:    ResourceRecord,
		Validate: func(ctx context.Context, config ConfigAccessor) []Finding {
			rt, ok := config.GetString("type")
			if !ok || rt != "APP" {
				return nil
			}
			var findings []Finding

			appName, hasAppName := config.GetString("app_name")
			if !hasAppName || appName == "" {
				findings = append(findings, Finding{
					Attribute: "app_name",
					Summary:   "APP record missing required field: app_name",
					Detail: "APP records require app_name, the name of an installed DNS app " +
						`exactly as the server reports it (e.g. "Weighted Round Robin").`,
				})
			}

			// value carries the class path. The shape check is here to catch
			// the one mistake that otherwise produces a record the server
			// accepts and never answers: putting the app NAME in value.
			// Technitium validates neither field, so without this the failure
			// shows up as a name that silently stops resolving.
			value, hasValue := config.GetString("value")
			switch {
			case !hasValue || value == "":
				findings = append(findings, Finding{
					Attribute: "value",
					Summary:   "APP record missing required field: value",
					Detail: "For an APP record, value is the DNS app class path " +
						`(e.g. "WeightedRoundRobin.Address") — the handler inside the app that ` +
						"answers the query, not the app name.",
				})
			case !isLikelyClassPath(value):
				findings = append(findings, Finding{
					Attribute: "value",
					Summary:   fmt.Sprintf("Invalid APP record value: %q does not look like a class path", value),
					Detail: `An APP record's value is a dotted class path such as ` +
						`"WeightedRoundRobin.Address" or "SplitHorizon.SimpleAddress", not the app ` +
						`name. Technitium accepts an unrecognized class path without error and ` +
						`stores a record that resolves to nothing, so it is rejected here instead.`,
				})
			}

			// Technitium parses record_data as JSON only when it starts with
			// { or [, and rejects it at apply time if it then does not parse.
			// Checking the same condition at plan time turns a failed apply
			// into a failed plan.
			if data, hasData := config.GetString("record_data"); hasData && looksLikeJSON(data) && !isValidJSON(data) {
				findings = append(findings, Finding{
					Attribute: "record_data",
					Summary:   "Invalid APP record record_data: not valid JSON",
					Detail: "record_data begins with { or [, so Technitium will parse it as JSON " +
						"and refuse the record if it does not parse. Use jsonencode() to build it " +
						"from Terraform values, or fix the literal.",
				})
			}

			return findings
		},
	}
}
