// Copyright (c) 2026 Dustin Sweigart
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/darkhonor/terraform-provider-technitium/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// The app the APP-record tests use. Weighted Round Robin is a good fixture:
// it ships two app-record handler classes, so the tests can change the class
// path in place, and its record data is JSON, which exercises the semantic
// comparison.
const (
	accAppName      = "Weighted Round Robin"
	accAppClass     = "WeightedRoundRobin.Address"
	accAppClassAlt  = "WeightedRoundRobin.CNAME"
	accAppOtherName = "Split Horizon"
)

// requireDNSApp makes sure a DNS app is installed before an APP-record test
// runs, installing it from the app store if it is missing.
//
// A fresh test container has no apps at all, and an APP record naming an
// uninstalled app is refused by the provider's pre-flight check -- so without
// this the APP tests could only ever pass on a hand-prepared server.
// Installing pulls from the Technitium app store, which needs outbound network
// access from the DNS server container; where that is unavailable the test
// skips rather than failing, because an offline runner is an environment
// limitation and not a defect in the provider.
func requireDNSApp(t *testing.T, appName string) {
	t.Helper()
	skipUnlessAcceptance(t)

	ctx := context.Background()
	c, err := client.NewClient(acceptanceClientConfig())
	if err != nil {
		t.Fatalf("building acceptance client: %v", err)
	}

	installed, err := c.AppList(ctx)
	if err != nil {
		t.Fatalf("listing installed DNS apps: %v", err)
	}
	for _, app := range installed {
		if app.Name == appName {
			return
		}
	}

	storeApps, err := c.AppStoreList(ctx)
	if err != nil {
		t.Skipf("DNS app %q is not installed and the app store is unreachable (%v); "+
			"install it manually to run this test", appName, err)
	}

	for _, app := range storeApps {
		if app.Name != appName {
			continue
		}
		if err := c.AppDownloadAndInstall(ctx, appName, app.URL); err != nil {
			t.Skipf("DNS app %q could not be installed (%v); install it manually to run this test", appName, err)
		}
		return
	}

	t.Skipf("DNS app %q is neither installed nor offered by the app store", appName)
}

func TestAccRecordResource_APP(t *testing.T) {
	requireDNSApp(t, accAppName)

	zone := "rec-app-test.example.com"
	name := "lb." + zone

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create.
			{
				Config: testAccRecordAPP(zone, name, accAppName, accAppClass,
					`{"ipv4Addresses":[{"address":"192.0.2.10","weight":5,"enabled":true},{"address":"192.0.2.11","weight":3,"enabled":true}]}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record.app", "type", "APP"),
					resource.TestCheckResourceAttr("technitium_record.app", "app_name", accAppName),
					resource.TestCheckResourceAttr("technitium_record.app", "value", accAppClass),
					resource.TestCheckResourceAttr("technitium_record.app", "ttl", "300"),
					resource.TestCheckResourceAttr("technitium_record.app", "id",
						fmt.Sprintf("%s::%s::APP::%s", zone, name, accAppClass)),
				),
			},
			// Change the record data in place. This is the update that wipes
			// the app configuration if recordData is not resent.
			{
				Config: testAccRecordAPP(zone, name, accAppName, accAppClass,
					`{"ipv4Addresses":[{"address":"192.0.2.10","weight":9,"enabled":true}]}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("technitium_record.app", "record_data"),
					resource.TestMatchResourceAttr("technitium_record.app", "record_data",
						regexp.MustCompile(`"weight":9`)),
				),
			},
			// Change the class path in place. Nothing may be replaced here:
			// the record is the same record, matched on type alone.
			{
				Config: testAccRecordAPP(zone, name, accAppName, accAppClassAlt,
					`{"cnames":[{"domain":"a.example.com","weight":1,"enabled":true}]}`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record.app", "value", accAppClassAlt),
					resource.TestMatchResourceAttr("technitium_record.app", "record_data",
						regexp.MustCompile(`a\.example\.com`)),
				),
			},
			// Import. app_name and record_data are recovered by reading the
			// record, so a bare class path in the ID is enough.
			{
				ResourceName:            "technitium_record.app",
				ImportState:             true,
				ImportStateId:           fmt.Sprintf("%s::%s::APP::%s", zone, name, accAppClassAlt),
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"overwrite"},
			},
		},
	})
}

// A TTL-only change must not disturb record_data. Technitium assigns
// recordData on every update, so an update that does not resend it stores the
// empty string and the app stops answering -- silently, with the API
// reporting success.
func TestAccRecordResource_APPTTLOnlyChangeKeepsRecordData(t *testing.T) {
	requireDNSApp(t, accAppName)

	zone := "rec-app-ttl-test.example.com"
	name := "lb." + zone
	data := `{"ipv4Addresses":[{"address":"192.0.2.20","weight":4,"enabled":true}]}`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRecordAPPWithTTL(zone, name, accAppName, accAppClass, data, 300),
				Check: resource.TestMatchResourceAttr("technitium_record.app", "record_data",
					regexp.MustCompile(`192\.0\.2\.20`)),
			},
			{
				Config: testAccRecordAPPWithTTL(zone, name, accAppName, accAppClass, data, 900),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("technitium_record.app", "ttl", "900"),
					resource.TestMatchResourceAttr("technitium_record.app", "record_data",
						regexp.MustCompile(`192\.0\.2\.20`)),
					resource.TestMatchResourceAttr("technitium_record.app", "record_data",
						regexp.MustCompile(`"weight":4`)),
				),
			},
		},
	})
}

// Reindenting the JSON must not produce a diff: the app reads the same
// document either way, and applying the change would rewrite the record for
// nothing.
func TestAccRecordResource_APPRecordDataReformatIsNotADiff(t *testing.T) {
	requireDNSApp(t, accAppName)

	zone := "rec-app-fmt-test.example.com"
	name := "lb." + zone

	compact := `{"ipv4Addresses":[{"address":"192.0.2.30","weight":2,"enabled":true}]}`
	pretty := `{
  "ipv4Addresses": [
    {
      "enabled": true,
      "weight": 2,
      "address": "192.0.2.30"
    }
  ]
}`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRecordAPP(zone, name, accAppName, accAppClass, compact),
			},
			{
				Config:   testAccRecordAPP(zone, name, accAppName, accAppClass, pretty),
				PlanOnly: true,
			},
		},
	})
}

// An app that is not installed must be refused before anything is written.
// Technitium would accept it and store a record that answers nothing.
func TestAccRecordResource_APPUninstalledAppIsRefused(t *testing.T) {
	requireDNSApp(t, accAppName)

	zone := "rec-app-bad-test.example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRecordAPP(zone, "nope."+zone,
					"No Such App Is Installed", "NoSuch.Handler", `{}`),
				ExpectError: regexp.MustCompile(`DNS app is not installed`),
			},
		},
	})
}

// A class path the installed app does not expose must be refused too.
func TestAccRecordResource_APPUnknownClassPathIsRefused(t *testing.T) {
	requireDNSApp(t, accAppName)

	zone := "rec-app-badclass-test.example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRecordAPP(zone, "nope."+zone,
					accAppName, "WeightedRoundRobin.NoSuchHandler", `{}`),
				ExpectError: regexp.MustCompile(`class path not found`),
			},
		},
	})
}

// A handler class that exists but cannot serve APP records must be refused:
// it would leave a record that resolves to nothing.
func TestAccRecordResource_APPNonRecordHandlerClassIsRefused(t *testing.T) {
	requireDNSApp(t, accAppOtherName)

	zone := "rec-app-nonhandler-test.example.com"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccRecordAPP(zone, "nope."+zone,
					accAppOtherName, "SplitHorizon.AddressTranslation", `{}`),
				ExpectError: regexp.MustCompile(`cannot serve APP records`),
			},
		},
	})
}

func testAccRecordAPP(zone, name, appName, classPath, recordData string) string {
	return testAccRecordAPPWithTTL(zone, name, appName, classPath, recordData, 300)
}

func testAccRecordAPPWithTTL(zone, name, appName, classPath, recordData string, ttl int) string {
	return testAccProviderHCL() + fmt.Sprintf(`

resource "technitium_zone" "app" {
  name = %[1]q
  type = "Primary"
}

resource "technitium_record" "app" {
  zone        = technitium_zone.app.name
  name        = %[2]q
  type        = "APP"
  ttl         = %[6]d
  value       = %[4]q
  app_name    = %[3]q
  record_data = %[5]q
}
`, zone, name, appName, classPath, recordData, ttl)
}
