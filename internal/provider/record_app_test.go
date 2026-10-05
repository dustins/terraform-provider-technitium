// Copyright (c) 2026 Dustin Sweigart
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"strings"

	"github.com/darkhonor/terraform-provider-technitium/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

const (
	wrrApp   = "Weighted Round Robin"
	wrrClass = "WeightedRoundRobin.Address"
	wrrData  = `{"ipv4Addresses":[{"address":"10.1.2.74","weight":5,"enabled":true}]}`
)

func appModel() *RecordResourceModel {
	return &RecordResourceModel{
		Zone:       types.StringValue("example.com"),
		Name:       types.StringValue("lb.example.com"),
		Type:       types.StringValue("APP"),
		TTL:        types.Int64Value(300),
		Value:      types.StringValue(wrrClass),
		AppName:    types.StringValue(wrrApp),
		RecordData: newRecordDataValue(wrrData),
	}
}

// ---------------------------------------------------------------------------
// Parameter mapping
// ---------------------------------------------------------------------------

func TestRecordValueParam_APPIsClassPath(t *testing.T) {
	if got := client.RecordValueParam("APP"); got != "classPath" {
		t.Fatalf("RecordValueParam(APP) = %q, want %q", got, "classPath")
	}
}

// The one asymmetry in the APP surface: add and update take "recordData",
// records/get reports it back as "data".
func TestRecordAppAccessors_ReadFromRData(t *testing.T) {
	rData := map[string]interface{}{
		"appName":   wrrApp,
		"classPath": wrrClass,
		"data":      wrrData,
	}

	if got := client.RecordAppName(rData); got != wrrApp {
		t.Errorf("RecordAppName = %q, want %q", got, wrrApp)
	}
	if got := client.RecordAppData(rData); got != wrrData {
		t.Errorf("RecordAppData = %q, want %q", got, wrrData)
	}
	if got := client.RecordValueFromRData("APP", rData); got != wrrClass {
		t.Errorf("RecordValueFromRData(APP) = %q, want %q", got, wrrClass)
	}
}

func TestRecordAppAccessors_MissingKeys(t *testing.T) {
	rData := map[string]interface{}{"classPath": wrrClass}

	if got := client.RecordAppName(rData); got != "" {
		t.Errorf("RecordAppName on absent key = %q, want empty", got)
	}
	// An add that omitted recordData stores "", which the API reports as an
	// empty string rather than omitting the key.
	if got := client.RecordAppData(map[string]interface{}{"data": ""}); got != "" {
		t.Errorf("RecordAppData on empty data = %q, want empty", got)
	}
	if got := client.RecordAppData(rData); got != "" {
		t.Errorf("RecordAppData on absent key = %q, want empty", got)
	}
}

func TestBuildAddParams_APP(t *testing.T) {
	r := &RecordResource{}

	params := r.buildAddParams(appModel())

	want := map[string]string{
		"classPath":  wrrClass,
		"appName":    wrrApp,
		"recordData": wrrData,
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("params[%q] = %q, want %q", k, params[k], v)
		}
	}
}

// Omitted record_data must become the empty string rather than being left out:
// add and update have to agree on what "unset" means, and the server stores ""
// either way.
func TestBuildAddParams_APPUnsetRecordDataIsEmptyString(t *testing.T) {
	r := &RecordResource{}
	m := appModel()
	m.RecordData = newRecordDataNull()

	params := r.buildAddParams(m)

	v, present := params["recordData"]
	if !present {
		t.Fatal("recordData must be sent even when unset")
	}
	if v != "" {
		t.Fatalf("recordData = %q, want empty string", v)
	}
}

func TestBuildAddParams_APPUnknownRecordDataIsEmptyString(t *testing.T) {
	r := &RecordResource{}
	m := appModel()
	m.RecordData = recordDataValue{StringValue: basetypes.NewStringUnknown()}

	if got := r.buildAddParams(m)["recordData"]; got != "" {
		t.Fatalf("recordData = %q, want empty string", got)
	}
}

// The destructive trap: Technitium assigns recordData unconditionally on
// update, so an update that leaves it out wipes the app's configuration. Every
// APP update must carry all three fields, including one that changes only the
// TTL.
func TestBuildUpdateParams_APPAlwaysSendsAllThreeFields(t *testing.T) {
	r := &RecordResource{}
	state := appModel()
	plan := appModel()
	plan.TTL = types.Int64Value(600) // TTL-only change

	params := r.buildUpdateParams(state, plan)

	for _, k := range []string{"appName", "classPath", "recordData"} {
		if _, present := params[k]; !present {
			t.Errorf("a TTL-only APP update must still send %q", k)
		}
	}
	if params["recordData"] != wrrData {
		t.Errorf("recordData = %q, want the unchanged data %q", params["recordData"], wrrData)
	}
}

// There is no newAppName/newClassPath/newRecordData; the plain parameters are
// the new values. Nothing resembling a current/new pair may be sent.
func TestBuildUpdateParams_APPSendsNewValuesWithoutNewPrefixes(t *testing.T) {
	r := &RecordResource{}
	state := appModel()
	plan := appModel()
	plan.Value = types.StringValue("WeightedRoundRobin.CNAME")
	plan.AppName = types.StringValue(wrrApp)
	plan.RecordData = newRecordDataValue(`{"cnames":[{"domain":"a.example.com","weight":1,"enabled":true}]}`)

	params := r.buildUpdateParams(state, plan)

	if params["classPath"] != "WeightedRoundRobin.CNAME" {
		t.Errorf("classPath = %q, want the planned value", params["classPath"])
	}
	if params["recordData"] != plan.RecordData.ValueString() {
		t.Errorf("recordData = %q, want the planned value", params["recordData"])
	}
	for _, k := range []string{"newAppName", "newClassPath", "newRecordData", "newValue"} {
		if _, present := params[k]; present {
			t.Errorf("%q must not be sent: the server ignores it", k)
		}
	}
}

// APP records are addressed by name alone. Sending identifiers the server
// ignores would state an intent it does not act on.
func TestBuildDeleteParams_APPSendsNoIdentifiers(t *testing.T) {
	r := &RecordResource{}

	params := r.buildDeleteParams(appModel())

	if len(params) != 0 {
		t.Fatalf("expected no type-specific delete params for APP, got %v", params)
	}
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

func TestBuildRecordID_APP(t *testing.T) {
	got := buildRecordID(appModel())
	want := "example.com::lb.example.com::APP::" + wrrClass

	if got != want {
		t.Fatalf("buildRecordID = %q, want %q", got, want)
	}
}

func TestParseImportValueSegment_APPIsTheWholeClassPath(t *testing.T) {
	fields, err := parseImportValueSegment("APP", wrrClass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields.Value != wrrClass {
		t.Fatalf("Value = %q, want %q", fields.Value, wrrClass)
	}
}

// A name holds at most one APP record, so type alone identifies it. This is
// what makes a class path change show up as drift instead of as a vanished
// record that cannot be recreated.
func TestRecordMatchesState_APPMatchesOnTypeAlone(t *testing.T) {
	state := appModel()

	rec := client.Record{
		Type: "APP",
		RData: map[string]interface{}{
			"appName":   "Split Horizon",
			"classPath": "SplitHorizon.SimpleAddress",
			"data":      `{"private":["10.0.0.1"]}`,
		},
	}

	if !recordMatchesState(rec, state) {
		t.Fatal("an APP record whose every field drifted must still match: it is the same record")
	}
}

func TestRecordMatchesState_APPDoesNotMatchOtherTypes(t *testing.T) {
	state := appModel()

	for _, rt := range []string{"A", "CNAME", "TXT", "FWD"} {
		rec := client.Record{Type: rt, RData: map[string]interface{}{"ipAddress": "192.0.2.1"}}
		if recordMatchesState(rec, state) {
			t.Errorf("APP state must not match a %s record", rt)
		}
	}
}

// The reverse guard: the APP shortcut must not leak into other types, where
// the primary value still has to match.
func TestRecordMatchesState_NonAPPStillComparesValue(t *testing.T) {
	state := &RecordResourceModel{
		Type:  types.StringValue("A"),
		Value: types.StringValue("192.0.2.1"),
	}
	rec := client.Record{Type: "A", RData: map[string]interface{}{"ipAddress": "192.0.2.99"}}

	if recordMatchesState(rec, state) {
		t.Fatal("an A record with a different address must not match")
	}
}

// ---------------------------------------------------------------------------
// record_data semantic equality
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

func TestAppCreateConflictDetail_OffersImportNotOverwrite(t *testing.T) {
	detail := appCreateConflictDetail(appModel())

	if !strings.Contains(detail, "terraform import") {
		t.Error("the conflict message must point at import")
	}
	if !strings.Contains(detail, buildRecordID(appModel())) {
		t.Error("the conflict message must include the import ID")
	}
	if !strings.Contains(detail, "at most one") {
		t.Error("the conflict message must explain the one-per-name rule")
	}
}

func TestAppRecordClassPaths_OnlyListsRecordHandlers(t *testing.T) {
	app := &client.DNSApp{
		Name: "Split Horizon",
		DNSApps: []client.DNSAppClass{
			{ClassPath: "SplitHorizon.AddressTranslation", IsAppRecordRequestHandler: false},
			{ClassPath: "SplitHorizon.SimpleAddress", IsAppRecordRequestHandler: true},
			{ClassPath: "SplitHorizon.SimpleCNAME", IsAppRecordRequestHandler: true},
		},
	}

	got := appRecordClassPaths(app)

	if strings.Contains(got, "AddressTranslation") {
		t.Errorf("a non-record handler must not be suggested: %s", got)
	}
	for _, want := range []string{"SimpleAddress", "SimpleCNAME"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in the suggestions, got %s", want, got)
		}
	}
}

func TestAppRecordClassPaths_AppWithNoRecordHandlers(t *testing.T) {
	app := &client.DNSApp{
		Name:    "Log Exporter",
		DNSApps: []client.DNSAppClass{{ClassPath: "LogExporter.App", IsAppRecordRequestHandler: false}},
	}

	if got := appRecordClassPaths(app); !strings.Contains(got, "none") {
		t.Fatalf("expected a 'none' explanation, got %s", got)
	}
}

func TestInstalledAppNames(t *testing.T) {
	if got := installedAppNames(nil); !strings.Contains(got, "none installed") {
		t.Errorf("empty app list = %q, want a 'none installed' explanation", got)
	}

	apps := []client.DNSApp{{Name: "Split Horizon"}, {Name: "Weighted Round Robin"}}
	got := installedAppNames(apps)
	for _, want := range []string{"Split Horizon", "Weighted Round Robin"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
}
