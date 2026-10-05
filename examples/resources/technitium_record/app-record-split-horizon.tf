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
