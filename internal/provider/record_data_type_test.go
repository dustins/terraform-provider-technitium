// Copyright (c) 2026 Dustin Sweigart
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

func TestJSONEquivalent(t *testing.T) {
	cases := []struct {
		name  string
		a, b  string
		equal bool
	}{
		{
			name:  "whitespace and indentation differ",
			a:     `{"ipv4Addresses":[{"address":"10.0.0.1","weight":5}]}`,
			b:     "{\n  \"ipv4Addresses\": [\n    {\n      \"address\": \"10.0.0.1\",\n      \"weight\": 5\n    }\n  ]\n}",
			equal: true,
		},
		{
			name:  "object key order differs",
			a:     `{"address":"10.0.0.1","weight":5}`,
			b:     `{"weight":5,"address":"10.0.0.1"}`,
			equal: true,
		},
		{
			name:  "nested key order differs",
			a:     `{"o":{"a":1,"b":2},"l":[1,2]}`,
			b:     `{"l":[1,2],"o":{"b":2,"a":1}}`,
			equal: true,
		},
		{
			name:  "trailing newline",
			a:     `{"a":1}`,
			b:     "{\"a\":1}\n",
			equal: true,
		},
		{
			name:  "number formatting",
			a:     `{"weight":5}`,
			b:     `{"weight":5.0}`,
			equal: true,
		},
		{
			// Array order is significant in JSON, and the weighted apps read
			// their address lists as ordered.
			name:  "array order differs",
			a:     `[1,2]`,
			b:     `[2,1]`,
			equal: false,
		},
		{
			name:  "a weight changed",
			a:     `{"address":"10.0.0.1","weight":5}`,
			b:     `{"address":"10.0.0.1","weight":3}`,
			equal: false,
		},
		{
			name:  "extra key",
			a:     `{"a":1}`,
			b:     `{"a":1,"b":2}`,
			equal: false,
		},
		{
			name:  "missing key",
			a:     `{"a":1,"b":2}`,
			b:     `{"a":1}`,
			equal: false,
		},
		{
			name:  "number vs string",
			a:     `{"weight":5}`,
			b:     `{"weight":"5"}`,
			equal: false,
		},
		{
			name:  "bool vs null",
			a:     `{"enabled":false}`,
			b:     `{"enabled":null}`,
			equal: false,
		},
		{
			name:  "object vs array",
			a:     `{"a":1}`,
			b:     `[{"a":1}]`,
			equal: false,
		},
		{
			// Non-JSON keeps exact string semantics. Two unparseable payloads
			// must not be declared equivalent just because neither is JSON --
			// StringSemanticEquals short-circuits identical strings before
			// this is ever consulted, so reporting false here is correct and
			// keeps a non-JSON change visible.
			name:  "identical non-JSON is not reported equivalent",
			a:     "plain text",
			b:     "plain text",
			equal: false,
		},
		{
			name:  "empty strings",
			a:     "",
			b:     "",
			equal: false,
		},
		{
			name:  "one side unparseable",
			a:     `{"a":1}`,
			b:     `{"a":`,
			equal: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := jsonEquivalent(tc.a, tc.b); got != tc.equal {
				t.Fatalf("jsonEquivalent(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.equal)
			}
		})
	}
}

func TestRecordDataValue_SemanticEqualsIgnoresFormatting(t *testing.T) {
	stored := newRecordDataValue(`{"ipv4Addresses":[{"address":"10.0.0.1","weight":5}]}`)
	reformatted := newRecordDataValue("{\n  \"ipv4Addresses\": [\n    { \"weight\": 5, \"address\": \"10.0.0.1\" }\n  ]\n}")

	equal, diags := stored.StringSemanticEquals(context.Background(), reformatted)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !equal {
		t.Fatal("a reindented, key-reordered payload must compare equal: the app reads the same document")
	}
}

func TestRecordDataValue_SemanticEqualsKeepsRealChange(t *testing.T) {
	stored := newRecordDataValue(`{"ipv4Addresses":[{"address":"10.0.0.1","weight":5}]}`)
	changed := newRecordDataValue(`{"ipv4Addresses":[{"address":"10.0.0.1","weight":9}]}`)

	equal, diags := stored.StringSemanticEquals(context.Background(), changed)

	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if equal {
		t.Fatal("a changed weight must not compare equal")
	}
}

// Non-JSON payloads fall back to exact comparison, which the identical-string
// short circuit handles. An app whose data is a bare string must still work.
func TestRecordDataValue_SemanticEqualsNonJSON(t *testing.T) {
	cases := []struct {
		name  string
		a, b  string
		equal bool
	}{
		{"identical plain text", "some payload", "some payload", true},
		{"different plain text", "some payload", "other payload", false},
		{"both empty", "", "", true},
		{"empty vs set", "", "x", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equal, diags := newRecordDataValue(tc.a).StringSemanticEquals(context.Background(), newRecordDataValue(tc.b))
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if equal != tc.equal {
				t.Fatalf("%q vs %q: equal=%v, want %v", tc.a, tc.b, equal, tc.equal)
			}
		})
	}
}

func TestRecordDataValue_SemanticEqualsRejectsForeignType(t *testing.T) {
	_, diags := newRecordDataValue(`{"a":1}`).StringSemanticEquals(
		context.Background(), basetypes.NewStringValue(`{"a":1}`))

	if !diags.HasError() {
		t.Fatal("a value of another type must produce an error diagnostic, not a silent false")
	}
}

func TestRecordDataType_RoundTripsThroughString(t *testing.T) {
	ctx := context.Background()

	valuable, diags := (recordDataType{}).ValueFromString(ctx, basetypes.NewStringValue("payload"))
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	v, ok := valuable.(recordDataValue)
	if !ok {
		t.Fatalf("ValueFromString returned %T, want recordDataValue", valuable)
	}
	if v.ValueString() != "payload" {
		t.Fatalf("value = %q, want %q", v.ValueString(), "payload")
	}
}

func TestRecordDataType_NullAndUnknownSurvive(t *testing.T) {
	if !newRecordDataNull().IsNull() {
		t.Error("newRecordDataNull must produce a null value")
	}
	unknown := recordDataValue{StringValue: basetypes.NewStringUnknown()}
	if !unknown.IsUnknown() {
		t.Error("an unknown StringValue must stay unknown through the wrapper")
	}
}

func TestRecordDataType_Equality(t *testing.T) {
	if !(recordDataType{}).Equal(recordDataType{}) {
		t.Error("recordDataType must equal itself")
	}
	if (recordDataType{}).Equal(basetypes.StringType{}) {
		t.Error("recordDataType must not equal a plain StringType: that is what gives it its own semantics")
	}
	if !newRecordDataValue("x").Equal(newRecordDataValue("x")) {
		t.Error("identical recordDataValues must be Equal")
	}
	if newRecordDataValue("x").Equal(newRecordDataValue("y")) {
		t.Error("differing recordDataValues must not be Equal")
	}
	// Equal is exact; only StringSemanticEquals is JSON-aware. Terraform calls
	// the two at different points and conflating them would hide real changes.
	if newRecordDataValue(`{"a":1}`).Equal(newRecordDataValue(`{ "a": 1 }`)) {
		t.Error("Equal must stay byte-exact; semantic comparison belongs to StringSemanticEquals")
	}
}

func TestRecordDataType_ValueTypeIsRecordDataValue(t *testing.T) {
	if _, ok := (recordDataType{}).ValueType(context.Background()).(recordDataValue); !ok {
		t.Fatal("ValueType must report recordDataValue")
	}
}
