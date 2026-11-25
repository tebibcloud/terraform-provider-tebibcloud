# A web server in its own VPC, reachable over SSH and HTTP(S) on a public IP.
#
#   export TEBIB_ACCESS_KEY_ID=TBK... TEBIB_SECRET_ACCESS_KEY=... TEBIB_PROJECT=p-...
#   tofu init && tofu apply      # or: terraform init && terraform apply
#   ssh ubuntu@$(tofu output -raw public_ip)

terraform {
  required_providers {
    tebibcloud = {
      source  = "tebibcloud/tebibcloud"
      version = "~> 0.1"
    }
  }
}

# Settings come from the environment (TEBIB_*) or your `tebib configure` profile.
provider "tebibcloud" {}

variable "ssh_public_key" {
  description = "Your SSH public key. Default: ~/.ssh/id_ed25519.pub."
  type        = string
  default     = null
}

variable "admin_cidr" {
  description = "Where SSH is allowed from. Narrow it to your own address."
  type        = string
  default     = "0.0.0.0/0"
}

data "tebibcloud_images" "ubuntu" {
  os_family = "ubuntu"
}

# -- network ------------------------------------------------------------------------------

resource "tebibcloud_vpc" "web" {
  name = "web"
  cidr = "10.10.0.0/16"
}

resource "tebibcloud_subnet" "web" {
  name   = "web"
  vpc_id = tebibcloud_vpc.web.id
  cidr   = "10.10.1.0/24"
}

# Public IPs reach VMs through their VPC's NAT gateway, so the VPC needs one.
resource "tebibcloud_public_ip" "nat" {
  name = "web-nat"
}

resource "tebibcloud_nat_gateway" "web" {
  name         = "web"
  subnet_id    = tebibcloud_subnet.web.id
  public_ip_id = tebibcloud_public_ip.nat.id
}

resource "tebibcloud_security_group" "web" {
  name        = "web"
  vpc_id      = tebibcloud_vpc.web.id
  description = "SSH from admins, HTTP and HTTPS from anywhere"
}

resource "tebibcloud_security_group_rule" "ssh" {
  security_group_id = tebibcloud_security_group.web.id
  protocol          = "tcp"
  port_from         = 22
  remote_cidr       = var.admin_cidr
}

resource "tebibcloud_security_group_rule" "http" {
  security_group_id = tebibcloud_security_group.web.id
  protocol          = "tcp"
  port_from         = 80
  remote_cidr       = "0.0.0.0/0"
}

resource "tebibcloud_security_group_rule" "https" {
  security_group_id = tebibcloud_security_group.web.id
  protocol          = "tcp"
  port_from         = 443
  remote_cidr       = "0.0.0.0/0"
}

# -- server -------------------------------------------------------------------------------

resource "tebibcloud_ssh_key" "me" {
  name       = "me"
  public_key = var.ssh_public_key != null ? var.ssh_public_key : file(pathexpand("~/.ssh/id_ed25519.pub"))
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

  tags = {
    role = "web"
  }
}

resource "tebibcloud_volume" "data" {
  name     = "web-1-data"
  size_gib = 50
}

resource "tebibcloud_volume_attachment" "data" {
  volume_id = tebibcloud_volume.data.id
  vm_id     = tebibcloud_vm.web.id
}

resource "tebibcloud_public_ip" "web" {
  name = "web-1"
}

resource "tebibcloud_public_ip_association" "web" {
  public_ip_id = tebibcloud_public_ip.web.id
  vm_id        = tebibcloud_vm.web.id

  depends_on = [tebibcloud_nat_gateway.web]
}

output "public_ip" {
  value = tebibcloud_public_ip.web.address
}

output "private_ip" {
  value = tebibcloud_vm.web.ipv4_address
}

output "ipv6" {
  value = tebibcloud_vm.web.ipv6_address
}
