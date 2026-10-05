---
subcategory: ""
page_title: "technitium_record Resource - terraform-provider-technitium"
description: |-
  Manages a DNS record in a Technitium DNS zone. Supports A, AAAA, CNAME, MX, TXT, SRV,
  PTR, NS, CAA, FWD, and APP record types. Client-side validation ensures type/value
  compatibility before API calls.
---

# technitium\_record (Resource)

Manages a DNS record in a Technitium DNS zone. Supports A, AAAA, CNAME, MX, TXT, SRV, PTR, NS, CAA, FWD, and APP record types. Client-side validation ensures type/value compatibility before API calls.

-> The `overwrite` attribute controls whether this record replaces existing records of the same type at the same name. Default is `true`.

## Example Usage

### A Record

```terraform
resource "technitium_record" "web" {
  zone  = "example.com"
  name  = "www.example.com"
  type  = "A"
  value = "192.168.1.100"
  ttl   = 3600
}
```

### MX Record

```hcl
resource "technitium_record" "mail" {
  zone     = "example.com"
  name     = "example.com"
  type     = "MX"
  value    = "mail.example.com"
  priority = 10
  ttl      = 3600
}
```

### SRV Record

```hcl
resource "technitium_record" "sip" {
  zone     = "example.com"
  name     = "_sip._tcp.example.com"
  type     = "SRV"
  value    = "sip.example.com"
  priority = 10
  weight   = 60
  port     = 5060
  ttl      = 3600
}
```

### CAA Record

```hcl
resource "technitium_record" "caa" {
  zone      = "example.com"
  name      = "example.com"
  type      = "CAA"
  value     = "letsencrypt.org"
  caa_flags = 0
  caa_tag   = "issue"
  ttl       = 3600
}
```

### Additional Record Types

```hcl
resource "technitium_record" "ipv6" {
  zone  = "example.com"
  name  = "www.example.com"
  type  = "AAAA"
  value = "2001:db8::1"
}

resource "technitium_record" "alias" {
  zone  = "example.com"
  name  = "app.example.com"
  type  = "CNAME"
  value = "www.example.com"
}

resource "technitium_record" "spf" {
  zone  = "example.com"
  name  = "example.com"
  type  = "TXT"
  value = "v=spf1 mx -all"
}

resource "technitium_record" "ns" {
  zone  = "example.com"
  name  = "sub.example.com"
  type  = "NS"
  value = "ns1.example.com"
}

resource "technitium_record" "ptr" {
  zone  = "1.168.192.in-addr.arpa"
  name  = "100.1.168.192.in-addr.arpa"
  type  = "PTR"
  value = "www.example.com"
}
```

### FWD Forwarder Records

Forwarder zones are created empty; each upstream forwarder is then managed as its own
`technitium_record` resource. The simplest case is a single forwarder:

```hcl
# Simplest case: forward everything to one upstream resolver.
#
# A Forwarder zone is created empty, then the forwarder itself is a separate
# FWD record. dnssec_validation is optional and can be left out entirely — with
# a single forwarder there is nothing for it to be confused with.
resource "technitium_zone" "forwarder" {
  name = "."
  type = "Forwarder"
}

resource "technitium_record" "upstream" {
  zone      = technitium_zone.forwarder.name
  name      = "."
  type      = "FWD"
  value     = "1.1.1.1"
  protocol  = "Udp"
  overwrite = false
}
```

A fuller example, with DNSSEC validation over DNS-over-TLS and a plain fallback:

```hcl
# Full example: a primary forwarder with DNSSEC validation over DNS-over-TLS,
# plus a plain fallback.
resource "technitium_zone" "root_forwarder" {
  name = "."
  type = "Forwarder"
}

resource "technitium_record" "quad9_forwarder" {
  zone               = technitium_zone.root_forwarder.name
  name               = "."
  type               = "FWD"
  value              = "dns.quad9.net:853 (9.9.9.9)"
  protocol           = "Tls"
  forwarder_priority = 1
  dnssec_validation  = true
  overwrite          = false
}

# Lower priority is queried first, so this is the fallback. It also differs from
# the record above by value and protocol, which keeps the two independently
# addressable.
#
# When several forwarders share a zone, no two may share both value and
# protocol. Technitium identifies a forwarder record by those two alone;
# forwarder_priority and dnssec_validation do not tell records apart, and the
# provider refuses a pair that differs only by them. See "DNSSEC validation on
# forwarders" in the resource documentation.
resource "technitium_record" "cloudflare_fallback" {
  zone               = technitium_zone.root_forwarder.name
  name               = "."
  type               = "FWD"
  value              = "1.1.1.1"
  protocol           = "Udp"
  forwarder_priority = 2
  dnssec_validation  = false
  overwrite          = false
}
```

Conditional forwarding — send a single internal namespace to the resolvers authoritative
for it, with a redundant pair of upstreams:

```hcl
# Conditional forwarding: send one internal namespace to the resolvers that are
# authoritative for it, while everything else follows the server's normal path.
#
# The Forwarder zone is named for the domain being forwarded rather than "." —
# only queries under that domain are forwarded.
resource "technitium_zone" "corp_internal" {
  name = "corp.example.net"
  type = "Forwarder"
}

# Two upstreams for redundancy. They differ by value, so they remain
# individually addressable; the priorities set which is tried first.
resource "technitium_record" "corp_dns_primary" {
  zone               = technitium_zone.corp_internal.name
  name               = technitium_zone.corp_internal.name
  type               = "FWD"
  value              = "10.10.0.53"
  protocol           = "Udp"
  forwarder_priority = 1
  overwrite          = false
}

resource "technitium_record" "corp_dns_secondary" {
  zone               = technitium_zone.corp_internal.name
  name               = technitium_zone.corp_internal.name
  type               = "FWD"
  value              = "10.20.0.53"
  protocol           = "Udp"
  forwarder_priority = 2
  overwrite          = false
}

# Internal resolvers usually serve names that do not validate publicly, so
# DNSSEC validation is left off here. Set dnssec_validation = true only for
# upstreams you expect to return signed, publicly-validatable answers.
```

#### DNSSEC validation on forwarders

**If each forwarder in a zone has its own address, or its own protocol, none of this
applies.**

~> **No two `FWD` records in a zone may share both `value` and `protocol`.** Technitium
identifies a forwarder record by its address and protocol only. `forwarder_priority` and
`dnssec_validation` do not tell two records apart, so a pair that differs only by those
cannot be destroyed or updated one at a time. The provider refuses to create such a pair
and refuses to destroy or update either record of an existing one.

**What goes wrong.** When two records share an address and protocol:

* **Destroying one destroys the wrong one.** The API deletes whichever record was created
  first, whatever priority or `dnssec_validation` the request names, and reports success.
* **Changing one merges the two.** An in-place update — even a TTL-only change — leaves a
  single record where there were two, again reported as success.

For the classic pair — a DNSSEC-validating forwarder plus a non-validating fallback to the
same upstream — the record silently lost is usually the **validating** one. On Technitium
15.5 and later, a Conditional Forwarder zone left with only non-validating forwarders is a
Negative Trust Anchor: DNSSEC validation is switched off for the whole namespace.

Verified against Technitium DNS Server 15.4 and 15.5.1. Technitium 15.5 and later also
refuse to add the second record of such a pair (`Cannot add record: record already
exists.`); 15.4 and earlier accept it, which is how existing pairs came about.

~> Earlier versions of this documentation recommended telling forwarders apart by
`forwarder_priority`. That was wrong: priority has never been part of a forwarder record's
identity. If you followed that advice, see **Recovering an existing pair** below.

**What the provider does about it.**

* **Create** with `overwrite = false` is refused when the zone already has a forwarder
  record with the same address and protocol, on every server version.
* **Destroy** and **update** are refused when the record shares its address and protocol
  with another record, and an update that would move a record onto an address and
  protocol already in use is refused too. Nothing is sent to the server.
* **Refresh** reports a warning for each managed record that is part of such a pair, so an
  existing collision surfaces on the next plan rather than on the next destroy.
* Changing `dnssec_validation` still forces replacement, so changing the setting on a
  single forwarder never goes through the update path.

**How to stay safe.** Give forwarders that should coexist a different `protocol` or a
different `value`, and use `forwarder_priority` only to set query order — lower is tried
first:

```hcl
resource "technitium_record" "validating" {
  zone               = technitium_zone.fwd.name
  name               = technitium_zone.fwd.name
  type               = "FWD"
  value              = "1.1.1.1"
  protocol           = "Udp"
  forwarder_priority = 1
  dnssec_validation  = true
  overwrite          = false
}

resource "technitium_record" "non_validating" {
  zone               = technitium_zone.fwd.name
  name               = technitium_zone.fwd.name
  type               = "FWD"
  value              = "1.1.1.1"
  protocol           = "Tcp" # a distinct protocol keeps the two records addressable
  forwarder_priority = 2
  dnssec_validation  = false
  overwrite          = false
}
```

Before keeping a non-validating fallback at all, consider what it is for: a fallback that
does not validate is a path around DNSSEC validation whenever the validating forwarder is
unreachable.

**Recovering an existing pair.** The API cannot remove one record of a colliding pair
without risking the other, so rebuild them together, in a maintenance window:

1. `terraform state rm` every `technitium_record` resource for that address and protocol in
   the zone.
2. Delete those records on the server, from the web console or the API. Each delete removes
   one record; repeat until none remain for that address and protocol.
3. Change the configuration so no two `FWD` records in the zone share both `value` and
   `protocol`.
4. `terraform apply` to recreate them.

**Record comments are public on Negative Trust Anchors.** On Technitium 15.5 and later, a
Conditional Forwarder zone whose forwarder record has DNSSEC validation disabled answers
with an Extended DNS Error (`NegativeTrustAnchor`) whose text is the record's comment,
visible to any client that queries the zone. Do not put anything in such a comment that
should not be public.

### APP Records (DNS Apps)

An `APP` record hands a name to an installed [DNS app](https://blog.technitium.com/2021/03/creating-and-running-dns-apps-on.html),
which computes the answer at query time — weighted load balancing, split-horizon
answers, failover, and so on.

Three attributes describe one:

* `app_name` — the installed app, named exactly as the server reports it.
* `value` — the **class path**: the handler inside that app which answers the query.
  This is not the app name. One app ships several handlers, and they behave differently.
* `record_data` — the handler's configuration, usually JSON. Optional; a few handlers
  take none.

```hcl
# Weighted round-robin load balancing via the "Weighted Round Robin" DNS app.
#
# The app must already be installed on the server (Apps tab in the web console,
# or /api/apps/downloadAndInstall); this provider does not install DNS apps.
#
# `value` is the class path -- the handler inside the app that answers the
# query. `app_name` is the app it belongs to. `record_data` is whatever that
# handler expects, which for this one is a weighted address list.
resource "technitium_record" "ntp_weighted" {
  zone = technitium_zone.internal.name
  name = "ntp.core.example.com"
  type = "APP"
  ttl  = 300

  app_name = "Weighted Round Robin"
  value    = "WeightedRoundRobin.Address"

  record_data = jsonencode({
    ipv4Addresses = [
      { address = "10.1.2.74", weight = 5, enabled = true },
      { address = "10.1.2.67", weight = 3, enabled = true },
      { address = "10.1.2.73", weight = 1, enabled = true },
    ]
  })

  # An enabled A or AAAA record at this name would win outright and the app
  # would never answer, with nothing reported as wrong -- so retire the A
  # records being replaced rather than leaving them declared alongside this.
  # (A record disabled in the web console does not mask the app, but the
  # provider cannot express "disabled", so anything it creates is enabled.)
  overwrite = false
}
```

```hcl
# Split-horizon answers via the "Split Horizon" DNS app: one name resolving
# differently depending on where the client is.
#
# A second example because the class path, not just the record data, is what
# selects behaviour: the same app also ships SplitHorizon.SimpleCNAME, and
# SplitHorizon.AddressTranslation, which is not an app-record handler at all.
# Naming that last one would store a record that resolves to nothing, so the
# provider checks the class against the installed app and refuses it.
resource "technitium_record" "api_split" {
  zone = technitium_zone.internal.name
  name = "api.example.com"
  type = "APP"
  ttl  = 60

  app_name = "Split Horizon"
  value    = "SplitHorizon.SimpleAddress"

  record_data = jsonencode({
    public            = ["203.0.113.10"]
    private           = ["10.1.2.10"]
    "10.0.0.0/8"      = ["10.1.2.11"]
  })
}

# Some apps need no configuration at all, in which case record_data is simply
# omitted.
resource "technitium_record" "whatismydns" {
  zone = technitium_zone.internal.name
  name = "whoami.example.com"
  type = "APP"

  app_name = "What Is My Dns"
  value    = "WhatIsMyDns.App"
}
```

#### A name holds at most one APP record

APP records do not form an RRset. Technitium refuses a second APP record at the same
name even with a different `app_name` or class path, so `(zone, name, APP)` identifies
the record completely — which is why `terraform import` needs nothing more than the
class path, and why changing `app_name`, `value` or `record_data` is an in-place
update rather than a replacement.

~> **An *enabled* address record at the same name silences the app.** Technitium
permits an `A` or `AAAA` record to sit beside an `APP` record, and while that record
is enabled it wins outright: the app never answers and nothing reports a problem.
Measured on Technitium 15.4 — a weighted record returned weighted answers until one
`A` record was added at that name, after which every answer came from the `A`
record. This holds for both `WeightedRoundRobin.Address` and
`WeightedRoundRobin.CNAME`; returning a different record type does not avoid it.

A **disabled** address record does not mask the app — the APP record answers
normally — which is how a name can be migrated by hand, by disabling the old
records in the web console rather than deleting them.

That matters here because this provider does not manage a record's disabled flag.
A disabled record is still a record: it appears in `records/get`, so Terraform
matches and keeps managing it, but nothing in the configuration says it is
disabled, and a record Terraform creates is always enabled. So a name taken over
by hand this way has address records that look managed and inert at the same time.
Removing them from the configuration deletes them, which is usually what you want;
re-adding one re-enables it and silences the app again.

#### The app must already be installed

This provider does not install DNS apps — install them from the web console's Apps tab
or via `/api/apps/downloadAndInstall` first.

Technitium accepts an `app_name` and class path that match nothing installed, returning
success and storing a record that resolves to nothing. To keep that from reaching the
zone, the provider checks both against `/api/apps/list` before writing, and refuses:

* an `app_name` no installed app matches;
* a class path the named app does not provide;
* a class path that exists but is not an app-record handler — `SplitHorizon.AddressTranslation`,
  for instance, is a post-processor, and an APP record naming it answers nothing.

-> This check is best-effort: listing apps needs a permission the API token may not
carry, and a token scoped to zones alone should still be able to manage records. When
the list cannot be read the check is skipped (logged at `DEBUG`) and the record is
written as configured.

#### record_data and formatting

`record_data` is stored byte for byte, and equivalent JSON does not register as a
change: reindenting it, or reordering keys within an object, produces no plan. Array
order *is* significant — the weighted apps read their address lists as ordered — and a
payload that is not JSON is compared exactly.

A value that begins with `{` or `[` is validated as JSON at plan time, because
Technitium parses it on exactly that condition and rejects the record if it does not
parse.

~> Technitium assigns `recordData` on every update, so an update that omits it stores
the empty string rather than leaving the stored configuration alone. The provider
always sends all three fields, which is what keeps a TTL-only change from silently
emptying the app's configuration. Nothing to configure — but it is worth knowing
before driving this API by hand.

### Multiple Records at Same Name (Round-Robin)

```hcl
resource "technitium_record" "web1" {
  zone      = "example.com"
  name      = "www.example.com"
  type      = "A"
  value     = "192.168.1.100"
  overwrite = false
}

resource "technitium_record" "web2" {
  zone      = "example.com"
  name      = "www.example.com"
  type      = "A"
  value     = "192.168.1.101"
  overwrite = false
}
```

-> When creating multiple records at the same name and type, set `overwrite = false` on each resource to prevent them from replacing each other.

## Argument Reference

* `zone` - (Required, String) Parent zone name. (Forces replacement.)

* `name` - (Required, String) FQDN for the record. (Forces replacement.)

* `type` - (Required, String) Record type. Valid values: `A`, `AAAA`, `CNAME`, `MX`, `TXT`, `SRV`, `PTR`, `NS`, `CAA`, `FWD`, `APP`. (Forces replacement.)

* `value` - (Required, String) Record data. For `FWD`, this is the forwarder address using Technitium name-server address syntax, e.g. `1.1.1.1`, `dns.quad9.net:853 (9.9.9.9)`, or a DoH URL.
  For `APP`, this is the DNS app **class path** — the handler inside the app that answers
  the query, e.g. `WeightedRoundRobin.Address` — not the app name. See
  [APP Records (DNS Apps)](#app-records-dns-apps).

* `ttl` - (Optional, Integer) TTL in seconds. Default: `3600`.

* `priority` - (Optional, Integer) Priority for MX and SRV records.

* `weight` - (Optional, Integer) Weight for SRV records.

* `port` - (Optional, Integer) Port for SRV records.

* `caa_flags` - (Optional, Integer) CAA flags. `0` = non-critical, `128` = critical.

* `caa_tag` - (Optional, String) CAA tag. Valid values: `issue`, `issuewild`, `iodef`.

* `protocol` - (Optional, String) Protocol for `FWD` records. Valid values: `Udp`, `Tcp`, `Tls`, `Https`, `Quic`.

* `forwarder_priority` - (Optional, Integer) Priority for `FWD` records. Lower values are queried first.
  Priority sets query order only — it does **not** make two forwarder records with the same
  `value` and `protocol` distinct. See [DNSSEC validation on forwarders](#dnssec-validation-on-forwarders).

* `dnssec_validation` - (Optional, Boolean) Enable DNSSEC validation for `FWD` records.
  Changing this value forces the record to be **replaced** (destroyed and recreated) rather
  than updated in place. It does not make two forwarder records distinct. See
  [DNSSEC validation on forwarders](#dnssec-validation-on-forwarders) before defining more
  than one forwarder to the same address.

* `proxy_type`, `proxy_address`, `proxy_port`, `proxy_username`, `proxy_password` - (Optional) Proxy settings for `FWD` records. `proxy_password` is sensitive.

* `app_name` - (Optional, String) Name of the installed DNS app serving this record, exactly
  as the server reports it, e.g. `Weighted Round Robin`. Required for `APP` records. The app
  must already be installed; this provider does not install DNS apps. See
  [APP Records (DNS Apps)](#app-records-dns-apps).

* `record_data` - (Optional, String) App-specific configuration for an `APP` record, as the
  app requires it — usually a JSON document, so `jsonencode(...)` or a heredoc. Some apps
  need none. Equivalent JSON that differs only in object key order or whitespace does not
  register as a change; array order is significant. A value beginning with `{` or `[` is
  validated as JSON at plan time.

* `overwrite` - (Optional, Boolean) Replace existing record set. Default: `true`.

* `comments` - (Optional, String) Free-text comment stored with the record, as shown in the
  Technitium web console. When omitted, the provider does not manage the comment: it adopts
  whatever the server holds and carries it through in-place updates unchanged. Set it to `""`
  to clear the comment. Removing the attribute from the configuration stops managing the
  comment but leaves it in place on the server.
  Do not store secrets or PII in comments: the value is not marked sensitive and appears in
  plan output and state in plain text.

## Attributes Reference

In addition to the arguments above, the following computed attributes are exported:

* `id` - Record identifier (`zone::name::type::value` composite key). For MX records: `zone::name::MX::exchange:priority`. For SRV records: `zone::name::SRV::target:priority:weight:port`. For CAA records: `zone::name::CAA::value:flags:tag`. For FWD records: `zone::name::FWD::forwarder:protocol:priority:dnssecValidation`. For APP records: `zone::name::APP::classPath`. The `dnssecValidation` field distinguishes otherwise-identical forwarders; the legacy 3-field form `forwarder:protocol:priority` is still accepted on import for backward compatibility.

* `last_modified` - Timestamp of last modification.

## Import

DNS records can be imported using the `::` separator with the format `zone::name::type::value`.

```shell
# A record
terraform import technitium_record.web "example.com::www.example.com::A::192.168.1.100"

# MX record (exchange:priority)
terraform import technitium_record.mail "example.com::example.com::MX::mail.example.com:10"

# SRV record (target:priority:weight:port)
terraform import technitium_record.sip "example.com::_sip._tcp.example.com::SRV::sip.example.com:10:60:5060"

# CAA record (value:flags:tag)
terraform import technitium_record.caa "example.com::example.com::CAA::letsencrypt.org:0:issue"

# FWD record (forwarder:protocol:priority:dnssecValidation)
terraform import technitium_record.forwarder ".::.::FWD::1.1.1.1:Udp:2:true"

# FWD record, legacy 3-field form (still accepted; dnssec_validation is left unset)
terraform import technitium_record.forwarder_legacy ".::.::FWD::1.1.1.1:Udp:2"

# APP record (classPath). app_name and record_data are read from the record,
# so the class path is all the ID has to carry.
terraform import technitium_record.app "example.com::lb.example.com::APP::WeightedRoundRobin.Address"
```
