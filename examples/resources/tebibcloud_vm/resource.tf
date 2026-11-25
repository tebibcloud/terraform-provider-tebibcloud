data "tebibcloud_images" "ubuntu" {
  os_family = "ubuntu"
}

resource "tebibcloud_vm" "web" {
  name               = "web-1"
  image              = data.tebibcloud_images.ubuntu.names[0]
  instance_type      = "b1.small"
  subnet_id          = tebibcloud_subnet.web.id
  security_group_ids = [tebibcloud_security_group.web.id]
  ssh_key_ids        = [tebibcloud_ssh_key.me.id]
  root_disk_gib      = 20

  user_data = <<-EOT
    #cloud-config
    packages: [nginx]
  EOT

  timeouts {
    create = "20m"
  }
}
