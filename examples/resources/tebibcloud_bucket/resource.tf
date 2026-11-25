resource "tebibcloud_bucket" "assets" {
  name         = "acme-assets"
  visibility   = "public-read"
  max_size_gib = 50
}
