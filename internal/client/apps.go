// Copyright (c) 2026 Dustin Sweigart
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// DNSApp is an installed DNS app as reported by /api/apps/list.
type DNSApp struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// DNSApps are the app's individual handler classes. One app ships several;
	// an APP record names exactly one of them by ClassPath.
	DNSApps []DNSAppClass `json:"dnsApps"`
}

// DNSAppClass is one handler class within an installed DNS app.
type DNSAppClass struct {
	ClassPath string `json:"classPath"`
	// IsAppRecordRequestHandler reports whether this class can answer queries
	// for an APP record. Classes with this false do something else entirely
	// (SplitHorizon.AddressTranslation is a post-processor, for instance) and
	// naming one in an APP record produces a record that resolves to nothing.
	IsAppRecordRequestHandler bool `json:"isAppRecordRequestHandler"`
}

// appListResponse is the response from listing installed DNS apps.
type appListResponse struct {
	Apps []DNSApp `json:"apps"`
}

// AppList returns the DNS apps installed on the server.
//
// Used to check an APP record's app_name and class path before writing it:
// Technitium accepts both without validation (see RecordAdd), so a typo
// otherwise creates a record that exists, reports no error, and answers
// nothing. The caller is expected to treat a failure here as "cannot verify"
// rather than "invalid", because listing apps needs a permission the API token
// may not carry.
func (c *Client) AppList(ctx context.Context) ([]DNSApp, error) {
	resp, err := c.doGet(ctx, "/api/apps/list", url.Values{})
	if err != nil {
		return nil, fmt.Errorf("listing installed DNS apps: %w", err)
	}

	var result appListResponse
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("parsing app list response: %w", err)
	}

	return result.Apps, nil
}

// StoreApp is a DNS app offered by the Technitium app store.
type StoreApp struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// URL is the download location the install call needs; it moves with each
	// app version, so it has to be read from the store rather than assumed.
	URL string `json:"url"`
}

// appStoreListResponse is the response from listing store apps.
type appStoreListResponse struct {
	StoreApps []StoreApp `json:"storeApps"`
}

// AppStoreList returns the DNS apps the server's app store offers. Reaching
// the store requires outbound network access from the DNS server itself.
//
// No resource uses this yet: it exists so the acceptance suite can provision
// the DNS app an APP record needs, without reimplementing the client's TLS and
// token handling. It is also the groundwork a `technitium_dns_app` resource
// would need.
func (c *Client) AppStoreList(ctx context.Context) ([]StoreApp, error) {
	resp, err := c.doGet(ctx, "/api/apps/listStoreApps", url.Values{})
	if err != nil {
		return nil, fmt.Errorf("listing store DNS apps: %w", err)
	}

	var result appStoreListResponse
	if err := json.Unmarshal(resp.Response, &result); err != nil {
		return nil, fmt.Errorf("parsing store app list response: %w", err)
	}

	return result.StoreApps, nil
}

// AppDownloadAndInstall installs a DNS app from downloadURL under the given
// name. Pair it with AppStoreList, which supplies the URL for a chosen app.
func (c *Client) AppDownloadAndInstall(ctx context.Context, name, downloadURL string) error {
	qp := url.Values{
		"name": {name},
		"url":  {downloadURL},
	}

	if _, err := c.doPost(ctx, "/api/apps/downloadAndInstall", qp); err != nil {
		return fmt.Errorf("installing DNS app %q: %w", name, err)
	}
	return nil
}
