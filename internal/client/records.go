// Copyright (c) 2026 Alex Ackerman
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Record represents a DNS record from the Technitium API.
type Record struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	TTL          int                    `json:"ttl"`
	Disabled     bool                   `json:"disabled"`
	RData        map[string]interface{} `json:"rData"`
	LastModified string                 `json:"lastModified"`
	// Comments is the free-text note stored with the record. The API omits it
	// (or sends null) when no comment is set; both decode to "".
	Comments string `json:"comments"`
}

// RecordAddResponse is the response from adding a record.
type RecordAddResponse struct {
	Zone        json.RawMessage `json:"zone"`
	AddedRecord Record          `json:"addedRecord"`
}

// RecordGetResponse is the response from getting records.
type RecordGetResponse struct {
	Zone    json.RawMessage `json:"zone"`
	Records []Record        `json:"records"`
}

// RecordAdd adds a DNS record to a zone.
// The params map should contain type-specific parameters:
//   - A/AAAA: "ipAddress"
//   - CNAME: "cname"
//   - MX: "exchange", "preference"
//   - TXT: "text"
//   - SRV: "priority", "weight", "port", "target"
//   - PTR: "ptrName"
//   - NS: "nameServer"
//   - CAA: "flags", "tag", "value"
//   - FWD: "protocol", "forwarder", "forwarderPriority", "dnssecValidation"
//   - APP: "appName", "classPath", "recordData"
//
// 🚩 APP records have three quirks the upstream API docs do not mention, all
// measured against Technitium 15.x:
//
//  1. "appName" and "classPath" are NOT validated against the installed apps.
//     Adding appName="No Such App", classPath="No.Such" returns status "ok" and
//     creates a record that answers nothing. AppList exists so the provider can
//     catch that before writing.
//  2. Omitting "recordData" is not "no data" — it stores the empty string.
//     Harmless on add, destructive on update (see RecordUpdate).
//  3. A name holds AT MOST ONE APP record. A second add at the same name fails
//     with "Record already exists. Use overwrite option if you wish to
//     overwrite existing record." even with a different appName and classPath,
//     and even with overwrite=false. (zone, domain, APP) is the whole identity;
//     there is no per-value RRset the way A or MX records have one.
func (c *Client) RecordAdd(ctx context.Context, domain, zone, recordType string, ttl int, overwrite bool, params map[string]string) (*Record, error) {
	qp := url.Values{
		"domain": {domain},
		"zone":   {zone},
		"type":   {recordType},
	}
	if ttl > 0 {
		qp.Set("ttl", fmt.Sprintf("%d", ttl))
	}
	if overwrite {
		qp.Set("overwrite", "true")
	}
	for k, v := range params {
		qp.Set(k, v)
	}

	resp, err := c.doPost(ctx, "/api/zones/records/add", qp)
	if err != nil {
		return nil, fmt.Errorf("adding %s record for %q in zone %q: %w", recordType, domain, zone, err)
	}

	var result RecordAddResponse
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("parsing add record response: %w", err)
	}

	return &result.AddedRecord, nil
}

// RecordGet retrieves records for a domain in a zone, optionally filtered by type.
func (c *Client) RecordGet(ctx context.Context, domain, zone string) ([]Record, error) {
	qp := url.Values{
		"domain": {domain},
		"zone":   {zone},
	}

	resp, err := c.doGet(ctx, "/api/zones/records/get", qp)
	if err != nil {
		return nil, fmt.Errorf("getting records for %q in zone %q: %w", domain, zone, err)
	}

	var result RecordGetResponse
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("parsing get records response: %w", err)
	}

	return result.Records, nil
}

// RecordUpdate updates an existing DNS record.
// The params map should contain both current and new values as needed by the API.
// For A/AAAA: "ipAddress" (current), "newIpAddress" (new)
// For CNAME: "cname" (new value)
// For MX: "exchange" (current), "newExchange" (new), "preference", "newPreference"
// etc.
//
// 🚩 The current/new pairing does NOT extend to every field. FWD records have
// "forwarder"/"newForwarder", "protocol"/"newProtocol" and
// "forwarderPriority"/"newForwarderPriority", but "dnssecValidation" has no
// "new" counterpart — it is the new value, and the old one cannot be supplied
// as an identifier. Two FWD records differing only by that flag therefore
// cannot be updated independently: the update rewrites one onto the other and
// they collapse into a single record, reported as status "ok". Omitting
// "dnssecValidation" entirely is also not "leave unchanged" — it resets the
// record to false. Both verified on Technitium 15.2 and 15.4 and reported
// upstream as TechnitiumSoftware/DnsServer#2069.
//
// 🚩 APP records have no current/new pairing at all. There is no
// "newAppName", "newClassPath" or "newRecordData" — supplying them is silently
// ignored, and "appName", "classPath" and "recordData" ARE the new values. Two
// consequences, both measured against Technitium 15.x:
//
//   - "appName" is mandatory: omitting it fails with "Parameter 'appName'
//     missing."
//   - omitting "recordData" ASSIGNS THE EMPTY STRING rather than leaving the
//     stored data alone. An update meaning to change only the TTL wipes the
//     app's configuration unless it resends the data. Same trap as "comments",
//     and the reason buildUpdateParams always sends all three.
//
// Because a name holds at most one APP record (see RecordAdd), domain + zone +
// type addresses it unambiguously; nothing else is needed to disambiguate.
func (c *Client) RecordUpdate(ctx context.Context, domain, zone, recordType string, ttl int, params map[string]string) error {
	qp := url.Values{
		"domain": {domain},
		"zone":   {zone},
		"type":   {recordType},
	}
	if ttl > 0 {
		qp.Set("ttl", fmt.Sprintf("%d", ttl))
	}
	for k, v := range params {
		qp.Set(k, v)
	}

	_, err := c.doPost(ctx, "/api/zones/records/update", qp)
	if err != nil {
		return fmt.Errorf("updating %s record for %q in zone %q: %w", recordType, domain, zone, err)
	}
	return nil
}

// RecordDelete deletes a DNS record.
// The params map should contain the type-specific identifier to match the record:
//   - A/AAAA: "ipAddress"
//   - CNAME: "cname"
//   - MX: "exchange", "preference"
//   - TXT: "text"
//   - SRV: "priority", "weight", "port", "target"
//   - PTR: "ptrName"
//   - NS: "nameServer"
//   - CAA: "flags", "tag", "value"
//   - FWD: "protocol", "forwarder", "forwarderPriority", "dnssecValidation"
//
// 🚩 "dnssecValidation" appears above because the provider sends it, NOT because
// the server matches on it. Technitium 15.2 and 15.4 ignore it when selecting
// which record to delete: given two FWD records identical apart from that flag,
// the first-created one is removed whatever value is supplied, and the call
// still returns status "ok". Do not assume this call can target one of a
// colliding pair. Reported upstream as TechnitiumSoftware/DnsServer#2069.
//
// APP records need no identifier beyond domain, zone and type. "appName",
// "classPath" and "recordData" are ignored outright: a delete carrying
// deliberately wrong values for all three still removed the record (measured
// against Technitium 15.x). That is consistent rather than surprising — a name
// holds at most one APP record, so there is nothing to choose between. The
// provider therefore sends none of them, instead of sending values the server
// would ignore.
func (c *Client) RecordDelete(ctx context.Context, domain, zone, recordType string, params map[string]string) error {
	qp := url.Values{
		"domain": {domain},
		"zone":   {zone},
		"type":   {recordType},
	}
	for k, v := range params {
		qp.Set(k, v)
	}

	_, err := c.doPost(ctx, "/api/zones/records/delete", qp)
	if err != nil {
		return fmt.Errorf("deleting %s record for %q in zone %q: %w", recordType, domain, zone, err)
	}
	return nil
}

// RecordValueParam returns the API parameter name for a record type's primary value.
// Used to map the generic "value" field to the type-specific API parameter.
//
// APP maps to "classPath": the class path is an APP record's primary datum, the
// one field that says what the record actually does. Note the write/read
// asymmetry handled by RecordValueFromRData — the add and update parameter is
// "recordData" but records/get reports it back under "data".
func RecordValueParam(recordType string) string {
	switch recordType {
	case "A", "AAAA":
		return "ipAddress"
	case "CNAME":
		return "cname"
	case "MX":
		return "exchange"
	case "TXT":
		return "text"
	case "SRV":
		return "target"
	case "PTR":
		return "ptrName"
	case "NS":
		return "nameServer"
	case "CAA":
		return "value"
	case "FWD":
		return "forwarder"
	case "APP":
		return "classPath"
	default:
		return "value"
	}
}

// APP record rData keys as reported by /api/zones/records/get. The data key is
// "data", NOT the "recordData" that add and update accept — the one asymmetry
// in the APP surface.
const (
	appRDataAppName   = "appName"
	appRDataClassPath = "classPath"
	appRDataData      = "data"
)

// RecordAppName extracts an APP record's app name from an rData map.
func RecordAppName(rData map[string]interface{}) string {
	return rDataString(rData, appRDataAppName)
}

// RecordAppData extracts an APP record's app-specific configuration from an
// rData map. Returns "" when the record carries none, which is what the server
// stores for an add that omitted recordData.
func RecordAppData(rData map[string]interface{}) string {
	return rDataString(rData, appRDataData)
}

func rDataString(rData map[string]interface{}, key string) string {
	if v, ok := rData[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// RecordValueFromRData extracts the primary value from an rData map.
func RecordValueFromRData(recordType string, rData map[string]interface{}) string {
	key := RecordValueParam(recordType)
	if v, ok := rData[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
