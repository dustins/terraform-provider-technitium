// Copyright (c) 2026 Dustin Sweigart
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/darkhonor/terraform-provider-technitium/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ---------------------------------------------------------------------------
// Pre-flight app verification
// ---------------------------------------------------------------------------

// guardAPPRecord checks an APP record's app_name and class path against the
// apps actually installed on the server, before the record is written.
//
// This exists because Technitium validates neither. Measured against 15.x, an
// add with appName="No Such App" and classPath="No.Such" returns status "ok"
// and leaves a record in the zone that resolves to nothing — no error at apply
// time, no drift on the next plan, just a name that silently stops answering.
// A class path naming a handler that is installed but is not an app-record
// handler (SplitHorizon.AddressTranslation, for one) fails the same way.
//
// Returns false when the record must not be written. A failure to LIST the
// apps is deliberately not such a case: /api/apps/list needs a permission the
// API token may not carry, and a token scoped to zones alone should still be
// able to manage records. In that case the check is skipped and logged at
// debug level rather than surfaced as a warning, because the alternative is a
// warning on every plan for the whole life of such a configuration. The
// resource documentation states that the check is best-effort for this reason.
func (r *RecordResource) guardAPPRecord(ctx context.Context, model *RecordResourceModel, diags *diag.Diagnostics) bool {
	appName := model.AppName.ValueString()
	classPath := model.Value.ValueString()

	apps, err := r.client.AppList(ctx)
	if err != nil {
		tflog.Debug(ctx, "skipping APP record pre-flight check: could not list installed DNS apps", map[string]interface{}{
			"error": err.Error(),
		})
		return true
	}

	var app *client.DNSApp
	for i := range apps {
		if apps[i].Name == appName {
			app = &apps[i]
			break
		}
	}
	if app == nil {
		diags.AddAttributeError(
			path.Root("app_name"),
			"DNS app is not installed",
			fmt.Sprintf(
				"app_name %q does not match any DNS app installed on this server.\n\n"+
					"Technitium accepts an unknown app name without complaint and stores a "+
					"record that answers nothing, so the provider refuses it here instead.\n\n"+
					"Installed apps: %s\n\n"+
					"Install the app first (DNS apps are installed from the server's Apps tab, "+
					"or via /api/apps/downloadAndInstall); the provider does not install apps.",
				appName, installedAppNames(apps),
			),
		)
		return false
	}

	for _, cls := range app.DNSApps {
		if cls.ClassPath != classPath {
			continue
		}
		if !cls.IsAppRecordRequestHandler {
			diags.AddAttributeError(
				path.Root("value"),
				"DNS app class cannot serve APP records",
				fmt.Sprintf(
					"class path %q belongs to app %q but is not an app-record request handler, "+
						"so an APP record naming it will resolve to nothing.\n\n"+
						"Class paths in %q that can serve APP records: %s",
					classPath, appName, appName, appRecordClassPaths(app),
				),
			)
			return false
		}
		return true
	}

	diags.AddAttributeError(
		path.Root("value"),
		"DNS app class path not found",
		fmt.Sprintf(
			"class path %q is not provided by the installed app %q.\n\n"+
				"For an APP record, value is the class path — the handler inside the app "+
				"that answers the query — not the app name.\n\n"+
				"Class paths in %q that can serve APP records: %s",
			classPath, appName, appName, appRecordClassPaths(app),
		),
	)
	return false
}

func installedAppNames(apps []client.DNSApp) string {
	if len(apps) == 0 {
		return "(none installed)"
	}
	names := make([]string, 0, len(apps))
	for _, a := range apps {
		names = append(names, fmt.Sprintf("%q", a.Name))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func appRecordClassPaths(app *client.DNSApp) string {
	paths := make([]string, 0, len(app.DNSApps))
	for _, cls := range app.DNSApps {
		if cls.IsAppRecordRequestHandler {
			paths = append(paths, fmt.Sprintf("%q", cls.ClassPath))
		}
	}
	if len(paths) == 0 {
		return "(none — this app does not handle APP records at all)"
	}
	sort.Strings(paths)
	return strings.Join(paths, ", ")
}

// appMatchCount reports how many APP records already exist at m's name. The
// server permits one, so the only interesting answers are 0 and 1.
func (r *RecordResource) appMatchCount(ctx context.Context, m *RecordResourceModel) (int, error) {
	records, err := r.client.RecordGet(ctx, m.Name.ValueString(), m.Zone.ValueString())
	if err != nil {
		if isRecordAlreadyGone(err) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, rec := range records {
		if rec.Type == "APP" {
			n++
		}
	}
	return n, nil
}

// appCreateConflictDetail explains a create that collided with an APP record
// already present at the same name.
//
// Worth spelling out because the server's own message ("Record already
// exists. Use overwrite option...") invites exactly the wrong fix: setting
// overwrite = true silently discards whatever the existing record held, which
// for an APP record is the app's entire configuration.
func appCreateConflictDetail(model *RecordResourceModel) string {
	return fmt.Sprintf(
		"%s already has an APP record, and a name holds at most one.\n\n"+
			"Unlike A or MX records, APP records do not form an RRset: the server refuses a "+
			"second one even with a different app_name or class path.\n\n"+
			"To take over the existing record:\n"+
			"    terraform import <resource address> '%s'\n\n"+
			"Setting overwrite = true would replace it instead, discarding the app "+
			"configuration it currently holds.",
		model.Name.ValueString(),
		buildRecordID(model),
	)
}
