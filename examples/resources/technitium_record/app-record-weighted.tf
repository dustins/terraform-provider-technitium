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
