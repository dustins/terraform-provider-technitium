// Copyright (c) 2026 Dustin Sweigart
// SPDX-License-Identifier: MPL-2.0

package inputvalidation

import (
	"context"
	"testing"
)

// findingAttrs returns the attributes the findings point at, so a test can
// assert which fields were faulted without pinning message wording.
func findingAttrs(findings []Finding) []string {
	attrs := make([]string, 0, len(findings))
	for _, f := range findings {
		attrs = append(attrs, f.Attribute)
	}
	return attrs
}

func hasAttr(findings []Finding, attr string) bool {
	for _, f := range findings {
		if f.Attribute == attr {
			return true
		}
	}
	return false
}

func TestValidateRecordType_AcceptsAPP(t *testing.T) {
	m := NewMockAccessor(map[string]interface{}{"type": "APP"})

	findings := validateRecordType().Validate(context.Background(), m)

	if len(findings) != 0 {
		t.Fatalf("APP must be a supported record type, got findings: %v", findingAttrs(findings))
	}
}

func TestValidateAPPRecord_Valid(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]interface{}
	}{
		{
			name: "weighted round robin with JSON data",
			attrs: map[string]interface{}{
				"type":        "APP",
				"app_name":    "Weighted Round Robin",
				"value":       "WeightedRoundRobin.Address",
				"record_data": `{"ipv4Addresses":[{"address":"10.1.2.74","weight":5,"enabled":true}]}`,
			},
		},
		{
			name: "split horizon",
			attrs: map[string]interface{}{
				"type":        "APP",
				"app_name":    "Split Horizon",
				"value":       "SplitHorizon.SimpleAddress",
				"record_data": `{"private":["10.0.0.1"]}`,
			},
		},
		{
			// Some apps need no configuration at all; an add that omits
			// recordData is accepted by the server and stores "".
			name: "no record_data at all",
			attrs: map[string]interface{}{
				"type":     "APP",
				"app_name": "Drop Requests",
				"value":    "DropRequests.App",
			},
		},
		{
			// Not every app's data is JSON. A payload that does not start
			// with { or [ is never parsed as JSON, so it must not be
			// JSON-checked either.
			name: "non-JSON record_data passes through",
			attrs: map[string]interface{}{
				"type":        "APP",
				"app_name":    "Some App",
				"value":       "Some.Handler",
				"record_data": "plain text payload",
			},
		},
		{
			name: "deeply namespaced class path",
			attrs: map[string]interface{}{
				"type":     "APP",
				"app_name": "Advanced Forwarding",
				"value":    "AdvancedForwarding.Inner.Handler",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := validateAPPRecord().Validate(context.Background(), NewMockAccessor(tc.attrs))
			if len(findings) != 0 {
				t.Fatalf("expected no findings, got %v", findingAttrs(findings))
			}
		})
	}
}

func TestValidateAPPRecord_MissingAppName(t *testing.T) {
	m := NewMockAccessor(map[string]interface{}{
		"type":  "APP",
		"value": "WeightedRoundRobin.Address",
	})

	findings := validateAPPRecord().Validate(context.Background(), m)

	if !hasAttr(findings, "app_name") {
		t.Fatalf("expected an app_name finding, got %v", findingAttrs(findings))
	}
}

func TestValidateAPPRecord_EmptyAppName(t *testing.T) {
	m := NewMockAccessor(map[string]interface{}{
		"type":     "APP",
		"app_name": "",
		"value":    "WeightedRoundRobin.Address",
	})

	findings := validateAPPRecord().Validate(context.Background(), m)

	if !hasAttr(findings, "app_name") {
		t.Fatalf("an empty app_name must be rejected, got %v", findingAttrs(findings))
	}
}

func TestValidateAPPRecord_MissingClassPath(t *testing.T) {
	m := NewMockAccessor(map[string]interface{}{
		"type":     "APP",
		"app_name": "Weighted Round Robin",
	})

	findings := validateAPPRecord().Validate(context.Background(), m)

	if !hasAttr(findings, "value") {
		t.Fatalf("expected a value finding, got %v", findingAttrs(findings))
	}
}

// The mistake worth catching by shape: the app NAME in the class path field.
// Technitium accepts it, stores the record, and answers nothing.
func TestValidateAPPRecord_AppNameInValue(t *testing.T) {
	m := NewMockAccessor(map[string]interface{}{
		"type":     "APP",
		"app_name": "Weighted Round Robin",
		"value":    "Weighted Round Robin",
	})

	findings := validateAPPRecord().Validate(context.Background(), m)

	if !hasAttr(findings, "value") {
		t.Fatalf("an app name in value must be rejected, got %v", findingAttrs(findings))
	}
}

func TestValidateAPPRecord_ClassPathShapes(t *testing.T) {
	cases := []struct {
		value string
		valid bool
	}{
		{"WeightedRoundRobin.Address", true},
		{"SplitHorizon.SimpleCNAME", true},
		{"A.B", true},
		{"_Private.Handler", true},
		{"App.Handler2", true},
		{"NoDotAtAll", false},
		{"Weighted Round Robin", false},
		{"trailing.", false},
		{".leading", false},
		{"double..dot", false},
		{"2Leading.Digit", false},
		{"has space.Handler", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			m := NewMockAccessor(map[string]interface{}{
				"type":     "APP",
				"app_name": "Some App",
				"value":    tc.value,
			})
			findings := validateAPPRecord().Validate(context.Background(), m)
			got := !hasAttr(findings, "value")
			if got != tc.valid {
				t.Fatalf("value %q: accepted=%v, want accepted=%v", tc.value, got, tc.valid)
			}
		})
	}
}

func TestValidateAPPRecord_MalformedJSONRecordData(t *testing.T) {
	cases := []struct {
		name  string
		data  string
		valid bool
	}{
		{"truncated object", `{"ipv4Addresses":[`, false},
		{"truncated array", `[{"a":1}`, false},
		{"trailing comma", `{"a":1,}`, false},
		{"valid object", `{"a":1}`, true},
		{"valid array", `[1,2,3]`, true},
		{"leading whitespace then valid", "  \n{\"a\":1}", true},
		{"leading whitespace then invalid", "  \n{\"a\":", false},
		// Never parsed as JSON by the server, so never JSON-checked here.
		{"bare word", "notjson", true},
		{"empty", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMockAccessor(map[string]interface{}{
				"type":        "APP",
				"app_name":    "Weighted Round Robin",
				"value":       "WeightedRoundRobin.Address",
				"record_data": tc.data,
			})
			findings := validateAPPRecord().Validate(context.Background(), m)
			got := !hasAttr(findings, "record_data")
			if got != tc.valid {
				t.Fatalf("record_data %q: accepted=%v, want accepted=%v", tc.data, got, tc.valid)
			}
		})
	}
}

// The rule must stay out of the way of every other record type, including the
// case where an unrelated record happens to carry record_data.
func TestValidateAPPRecord_IgnoresOtherTypes(t *testing.T) {
	for _, rt := range []string{"A", "AAAA", "CNAME", "MX", "NS", "PTR", "SRV", "TXT", "CAA", "FWD"} {
		t.Run(rt, func(t *testing.T) {
			m := NewMockAccessor(map[string]interface{}{
				"type":        rt,
				"value":       "192.0.2.1",
				"record_data": `{"broken":`,
			})
			findings := validateAPPRecord().Validate(context.Background(), m)
			if len(findings) != 0 {
				t.Fatalf("APP rule must not fire for %s, got %v", rt, findingAttrs(findings))
			}
		})
	}
}

func TestLooksLikeJSON(t *testing.T) {
	cases := map[string]bool{
		`{"a":1}`:  true,
		`[1,2]`:    true,
		"  {":      true,
		"\n\t[":    true,
		"plain":    false,
		"":         false,
		`"quoted"`: false,
		"123":      false,
		"a{not":    false,
	}
	for in, want := range cases {
		if got := looksLikeJSON(in); got != want {
			t.Errorf("looksLikeJSON(%q) = %v, want %v", in, got, want)
		}
	}
}
