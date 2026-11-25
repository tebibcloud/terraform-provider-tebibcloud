resource "tebibcloud_public_ip_association" "web" {
  public_ip_id = tebibcloud_public_ip.web.id
  vm_id        = tebibcloud_vm.web.id

  # A VM's public IP reaches it through the VPC's NAT gateway.
  depends_on = [tebibcloud_nat_gateway.main]
}
