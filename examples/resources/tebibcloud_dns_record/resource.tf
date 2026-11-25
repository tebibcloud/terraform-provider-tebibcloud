resource "tebibcloud_dns_record" "apex" {
  zone_id = tebibcloud_dns_zone.example.id
  type    = "A"
  values  = [tebibcloud_load_balancer.web.public_address]
}

resource "tebibcloud_dns_record" "www" {
  zone_id = tebibcloud_dns_zone.example.id
  name    = "www"
  type    = "CNAME"
  values  = ["example.com."]
}

resource "tebibcloud_dns_record" "mx" {
  zone_id = tebibcloud_dns_zone.example.id
  type    = "MX"
  ttl     = 600
  values  = ["10 mail.example.com.", "20 backup.example.com."]
}
