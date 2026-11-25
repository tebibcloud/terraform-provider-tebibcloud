data "tebibcloud_security_group" "default" {
  default = true
  vpc_id  = data.tebibcloud_vpc.default.id
}
