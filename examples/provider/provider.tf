# Settings come from the first of: this block, TEBIB_* environment variables, the
# tebib CLI's profile files (~/.config/tebib), the defaults.
provider "tebibcloud" {
  # endpoint          = "https://console.tebibcloud.com"  # TEBIB_ENDPOINT
  # access_key_id     = "TBK..."                       # TEBIB_ACCESS_KEY_ID
  # secret_access_key = var.tebibcloud_secret               # TEBIB_SECRET_ACCESS_KEY
  # profile           = "lab"                          # TEBIB_PROFILE
  organization = "o-abc123" # TEBIB_ORGANIZATION
  project      = "p-def456" # TEBIB_PROJECT
}
