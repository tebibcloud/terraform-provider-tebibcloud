resource "tebibcloud_dns_zone" "example" {
  domain = "example.com"
}

# Delegate the domain to these at your registrar.
output "nameservers" {
  value = tebibcloud_dns_zone.example.nameservers
}
