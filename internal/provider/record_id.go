// Copyright (c) 2026 Alex Ackerman
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/darkhonor/terraform-provider-technitium/internal/client"
)

const recordIDSeparator = "::"

// buildRecordID constructs a composite record ID from the resource model.
//
// The ID format encodes type-specific fields to prevent collisions when
// multiple records share the same name and type (e.g., multiple MX records).
//
// Formats:
//   - Simple types (A, AAAA, CNAME, TXT, PTR, NS): zone::name::type::value
//   - MX: zone::name::MX::exchange:priority
//   - SRV: zone::name::SRV::target:priority:weight:port
//   - CAA: zone::name::CAA::value:flags:tag
//   - FWD: zone::name::FWD::forwarder:protocol:priority:dnssecValidation
//   - APP: zone::name::APP::classPath
//
// APP takes the simple form because a name holds at most one APP record, so
// zone + name + type is already unique; the class path rides along because the
// ID format wants four segments and it is the most useful thing to put there.
// app_name and record_data are recovered by reading the record, which is also
// why an import ID does not have to carry them.
//
// The FWD form carries dnssecValidation because forwarder/protocol/priority
// alone do not uniquely identify a record — two forwarders may differ only by
// that flag. parseImportValueSegment still accepts the legacy 3-field form, so
// IDs generated before this existed continue to import.
func buildRecordID(model *RecordResourceModel) string {
	zone := model.Zone.ValueString()
	name := model.Name.ValueString()
	recordType := model.Type.ValueString()
	value := model.Value.ValueString()

	var valueSegment string
	switch recordType {
	case "MX":
		valueSegment = fmt.Sprintf("%s:%d", value, model.Priority.ValueInt64())
	case "SRV":
		valueSegment = fmt.Sprintf("%s:%d:%d:%d",
			value,
			model.Priority.ValueInt64(),
			model.Weight.ValueInt64(),
			model.Port.ValueInt64(),
		)
	case "CAA":
		valueSegment = fmt.Sprintf("%s:%d:%s",
			value,
			model.CAAFlags.ValueInt64(),
			model.CAATag.ValueString(),
		)
	case "FWD":
		valueSegment = fmt.Sprintf("%s:%s:%d:%t", value, model.Protocol.ValueString(), model.ForwarderPriority.ValueInt64(), model.DNSSECValidation.ValueBool())
	default:
		valueSegment = value
	}

	return strings.Join([]string{zone, name, recordType, valueSegment}, recordIDSeparator)
}

// parseRecordID splits a composite ID into its four segments.
//
// Uses SplitN with limit 4 so that values containing "::" (e.g., IPv6
// addresses in AAAA records or TXT content) are preserved intact in the
// value segment.
func parseRecordID(id string) (zone, name, recordType, valueSegment string, err error) {
	parts := strings.SplitN(id, recordIDSeparator, 4)
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return "", "", "", "", fmt.Errorf(
			"invalid record ID %q: expected format zone::name::type::value", id)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// importValueFields holds the type-specific fields decoded from the value
// segment of a composite import ID.
//
// This is a named struct rather than a list of positional returns on purpose.
// The previous signature returned a single shared string slot that CAA used for
// its tag and FWD reused for its protocol, so ImportState read a variable named
// caaTag and wrote it straight into the protocol attribute. That worked only by
// coincidence of the two types sharing a slot, and it is exactly the kind of
// thing that regresses silently when a new record type is added.
type importValueFields struct {
	Value    string
	Priority int64
	Weight   int64
	Port     int64
	CAAFlags int64
	// CAATag is set for CAA records only.
	CAATag string
	// Protocol is set for FWD records only.
	Protocol string
	// DNSSECValidation is set for FWD records only, and is nil when the legacy
	// 3-field form (forwarder:protocol:priority) was used. nil therefore means
	// "the import ID did not state it", which is distinct from an explicit
	// false — the caller must not set the attribute in that case.
	DNSSECValidation *bool
}

// parseImportValueSegment extracts value and type-specific fields from the
// value segment of an import ID.
//
// Formats:
//   - MX: exchange:priority (parsed from right via LastIndex)
//   - SRV: target:priority:weight:port (last 3 fields numeric, rest is target)
//   - CAA: value:flags:tag (last 2 fields are flags+tag, rest is value)
//   - FWD: forwarder:protocol:priority[:dnssecValidation] (parsed from the
//     right; the trailing dnssec field is optional for backward compatibility
//     with IDs generated before it was part of the identity)
//   - Simple types: entire segment is the value
//
// Parsing from the right matters for FWD: a forwarder may itself contain
// colons (an IPv6 address, or a DoH URL such as
// https://cloudflare-dns.com/dns-query), so the leading fields are rejoined.
func parseImportValueSegment(recordType, valueSegment string) (importValueFields, error) {
	fail := func(format string, args ...any) (importValueFields, error) {
		return importValueFields{}, fmt.Errorf(format, args...)
	}

	switch recordType {
	case "MX":
		idx := strings.LastIndex(valueSegment, ":")
		if idx < 0 {
			return fail("invalid MX value segment %q: expected format exchange:priority", valueSegment)
		}
		p, parseErr := strconv.ParseInt(valueSegment[idx+1:], 10, 64)
		if parseErr != nil {
			return fail("invalid MX priority in %q: %w", valueSegment, parseErr)
		}
		return importValueFields{Value: valueSegment[:idx], Priority: p}, nil

	case "SRV":
		// Format: target:priority:weight:port
		// Parse from the right: last 3 colon-separated fields are numeric.
		parts := strings.Split(valueSegment, ":")
		if len(parts) < 4 {
			return fail("invalid SRV value segment %q: expected format target:priority:weight:port", valueSegment)
		}
		p, parseErr := strconv.ParseInt(parts[len(parts)-3], 10, 64)
		if parseErr != nil {
			return fail("invalid SRV priority in %q: %w", valueSegment, parseErr)
		}
		w, parseErr := strconv.ParseInt(parts[len(parts)-2], 10, 64)
		if parseErr != nil {
			return fail("invalid SRV weight in %q: %w", valueSegment, parseErr)
		}
		pt, parseErr := strconv.ParseInt(parts[len(parts)-1], 10, 64)
		if parseErr != nil {
			return fail("invalid SRV port in %q: %w", valueSegment, parseErr)
		}
		return importValueFields{
			Value:    strings.Join(parts[:len(parts)-3], ":"),
			Priority: p,
			Weight:   w,
			Port:     pt,
		}, nil

	case "FWD":
		parts := strings.Split(valueSegment, ":")
		if len(parts) < 3 {
			return fail("invalid FWD value segment %q: expected format forwarder:protocol:priority[:dnssecValidation]", valueSegment)
		}

		// The trailing dnssec field is optional. Detect it by value rather than
		// by field count: the forwarder may contain colons, so a count-based
		// test cannot tell "4 fields because dnssec is present" from "4 fields
		// because the forwarder had a colon in it".
		var dnssec *bool
		priorityIdx := len(parts) - 1
		if last := parts[len(parts)-1]; last == "true" || last == "false" {
			v := last == "true"
			dnssec = &v
			priorityIdx = len(parts) - 2
		}
		if priorityIdx < 2 {
			return fail("invalid FWD value segment %q: expected format forwarder:protocol:priority[:dnssecValidation]", valueSegment)
		}

		p, parseErr := strconv.ParseInt(parts[priorityIdx], 10, 64)
		if parseErr != nil {
			return fail("invalid FWD priority in %q: %w", valueSegment, parseErr)
		}
		return importValueFields{
			Value:            strings.Join(parts[:priorityIdx-1], ":"),
			Priority:         p,
			Protocol:         parts[priorityIdx-1],
			DNSSECValidation: dnssec,
		}, nil

	case "CAA":
		// Format: value:flags:tag
		// Parse from the right: last field is tag, second-to-last is flags.
		parts := strings.Split(valueSegment, ":")
		if len(parts) < 3 {
			return fail("invalid CAA value segment %q: expected format value:flags:tag", valueSegment)
		}
		f, parseErr := strconv.ParseInt(parts[len(parts)-2], 10, 64)
		if parseErr != nil {
			return fail("invalid CAA flags in %q: %w", valueSegment, parseErr)
		}
		return importValueFields{
			Value:    strings.Join(parts[:len(parts)-2], ":"),
			CAAFlags: f,
			CAATag:   parts[len(parts)-1],
		}, nil

	default:
		return importValueFields{Value: valueSegment}, nil
	}
}

// recordMatchesState returns true if an API record matches the Terraform state
// model. This is used to find the specific record when multiple records share
// the same name and type.
//
// Matching criteria by type:
//   - All types: match on type AND primary value (via client.RecordValueFromRData)
//   - MX: also match on preference (rData "preference") vs state.Priority
//   - SRV: also match on priority, weight, port rData fields
//   - CAA: also match on flags, tag rData fields
//   - FWD: also match on protocol and priority/forwarderPriority rData fields
func recordMatchesState(rec client.Record, state *RecordResourceModel) bool {
	recordType := state.Type.ValueString()

	// Type must match.
	if rec.Type != recordType {
		return false
	}

	// APP is a singleton: a name holds at most one APP record, so type alone
	// identifies it and the primary-value comparison below must be skipped.
	//
	// That skip is the point, not a shortcut. If the class path had to match,
	// then changing it on the server -- or in the configuration -- would make
	// Read find no match, call RemoveResource, and plan a create. The create
	// would then fail against the record that is still sitting there
	// ("Record already exists"), leaving a configuration that cannot converge.
	// Matching on type lets a class path, app name or record data change
	// surface as ordinary drift and be fixed in place by Update.
	if recordType == "APP" {
		return true
	}

	// Primary value must match.
	apiValue := client.RecordValueFromRData(recordType, rec.RData)
	if apiValue != state.Value.ValueString() {
		return false
	}

	// Type-specific field matching.
	switch recordType {
	case "MX":
		if pref, ok := rec.RData["preference"]; ok {
			if int64(toFloat64(pref)) != state.Priority.ValueInt64() {
				return false
			}
		}
	case "SRV":
		if p, ok := rec.RData["priority"]; ok {
			if int64(toFloat64(p)) != state.Priority.ValueInt64() {
				return false
			}
		}
		if w, ok := rec.RData["weight"]; ok {
			if int64(toFloat64(w)) != state.Weight.ValueInt64() {
				return false
			}
		}
		if pt, ok := rec.RData["port"]; ok {
			if int64(toFloat64(pt)) != state.Port.ValueInt64() {
				return false
			}
		}
	case "CAA":
		if f, ok := rec.RData["flags"]; ok {
			if int64(toFloat64(f)) != state.CAAFlags.ValueInt64() {
				return false
			}
		}
		if tag, ok := rec.RData["tag"]; ok {
			if fmt.Sprintf("%v", tag) != state.CAATag.ValueString() {
				return false
			}
		}
	case "FWD":
		if protocol, ok := rec.RData["protocol"]; ok && !state.Protocol.IsNull() {
			if fmt.Sprintf("%v", protocol) != state.Protocol.ValueString() {
				return false
			}
		}
		if priority, ok := rec.RData["forwarderPriority"]; ok && !state.ForwarderPriority.IsNull() {
			if int64(toFloat64(priority)) != state.ForwarderPriority.ValueInt64() {
				return false
			}
		} else if priority, ok := rec.RData["priority"]; ok && !state.ForwarderPriority.IsNull() {
			if int64(toFloat64(priority)) != state.ForwarderPriority.ValueInt64() {
				return false
			}
		}
		if dnssec, ok := rec.RData["dnssecValidation"]; ok && !state.DNSSECValidation.IsNull() {
			var apiDNSSEC bool
			switch v := dnssec.(type) {
			case bool:
				apiDNSSEC = v
			default:
				apiDNSSEC = toFloat64(v) != 0
			}
			if apiDNSSEC != state.DNSSECValidation.ValueBool() {
				return false
			}
		}
	}

	return true
}
