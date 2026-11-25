resource "tebibcloud_public_ip" "nat" {
  name = "nat"
}

resource "tebibcloud_nat_gateway" "main" {
  name         = "main"
  subnet_id    = tebibcloud_subnet.web.id
  public_ip_id = tebibcloud_public_ip.nat.id
}
