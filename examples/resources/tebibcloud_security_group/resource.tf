resource "tebibcloud_security_group" "web" {
  name        = "web"
  vpc_id      = tebibcloud_vpc.main.id
  description = "Web servers"
}
