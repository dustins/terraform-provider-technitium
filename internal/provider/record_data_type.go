// Copyright (c) 2026 Dustin Sweigart
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// recordDataType is the type of the record_data attribute: a string whose
// equality is JSON-semantic when both sides are JSON, and byte-exact
// otherwise.
//
// Why a custom type rather than a plan modifier. The goal is for a reindented
// or key-reordered payload to produce NO plan at all. A plan modifier can copy
// the prior value over the planned one, which does suppress the diff on
// record_data itself -- but by then the framework has already decided the
// resource is changing, so id and last_modified stay "known after apply" and
// the plan is non-empty anyway. Semantic equality is consulted earlier, when
// the framework decides whether the values differ in the first place, so an
// equivalent document is a true no-op. Measured: the plan-modifier version
// left `0 to add, 1 to change, 0 to destroy` on a pure reformat.
//
// Not jsontypes.Normalized, which does the same job for strings that are
// always JSON: record_data is whatever the DNS app wants. Technitium parses it
// as JSON only when it starts with { or [, so an app taking a bare string is
// legal, and a type that insisted on JSON would reject it. Here a non-JSON
// payload simply falls back to exact comparison.
type recordDataType struct {
	basetypes.StringType
}

var _ basetypes.StringTypable = recordDataType{}

func (t recordDataType) String() string {
	return "provider.recordDataType"
}

func (t recordDataType) Equal(o attr.Type) bool {
	other, ok := o.(recordDataType)
	if !ok {
		return false
	}
	return t.StringType.Equal(other.StringType)
}

func (t recordDataType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return recordDataValue{StringValue: in}, nil
}

func (t recordDataType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}

	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T, expected basetypes.StringValue", attrValue)
	}

	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}

	return stringValuable, nil
}

func (t recordDataType) ValueType(_ context.Context) attr.Value {
	return recordDataValue{}
}

// recordDataValue is a string value with JSON-semantic equality.
type recordDataValue struct {
	basetypes.StringValue
}

var _ basetypes.StringValuableWithSemanticEquals = recordDataValue{}

func (v recordDataValue) Type(_ context.Context) attr.Type {
	return recordDataType{}
}

func (v recordDataValue) Equal(o attr.Value) bool {
	other, ok := o.(recordDataValue)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

// StringSemanticEquals reports whether two record_data values mean the same
// thing to the DNS app reading them.
//
// Object key order and whitespace are insignificant in JSON; array order is
// significant, and the weighted apps read their address lists as ordered. When
// either side is not JSON the comparison is exact, so a plain-text payload
// keeps ordinary string semantics.
func (v recordDataValue) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics

	newValue, ok := newValuable.(recordDataValue)
	if !ok {
		diags.AddError(
			"Semantic Equality Check Error",
			fmt.Sprintf("expected value type %T but got %T. Please report this to the provider developers.", v, newValuable),
		)
		return false, diags
	}

	// Null and unknown are handled by the framework before this is reached;
	// identical strings are the common case and need no parsing.
	if v.ValueString() == newValue.ValueString() {
		return true, diags
	}

	return jsonEquivalent(v.ValueString(), newValue.ValueString()), diags
}

// newRecordDataValue returns a known record_data value.
func newRecordDataValue(s string) recordDataValue {
	return recordDataValue{StringValue: basetypes.NewStringValue(s)}
}

// newRecordDataNull returns a null record_data value.
func newRecordDataNull() recordDataValue {
	return recordDataValue{StringValue: basetypes.NewStringNull()}
}

// jsonEquivalent reports whether two strings are the same JSON document.
// Returns false unless BOTH parse as JSON, so non-JSON payloads keep exact
// string semantics rather than silently comparing equal.
func jsonEquivalent(a, b string) bool {
	var av, bv interface{}
	if err := json.Unmarshal([]byte(a), &av); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(b), &bv); err != nil {
		return false
	}
	return jsonDeepEqual(av, bv)
}

// jsonDeepEqual compares two decoded JSON values. encoding/json decodes into
// map[string]interface{}, []interface{}, float64, string, bool and nil, so
// reflect.DeepEqual would also work -- but writing it out documents the one
// thing that matters here: object key order is insignificant while array
// order is significant, which is what JSON means and what the DNS apps rely
// on.
func jsonDeepEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, val := range av {
			other, present := bv[k]
			if !present || !jsonDeepEqual(val, other) {
				return false
			}
		}
		return true
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonDeepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
