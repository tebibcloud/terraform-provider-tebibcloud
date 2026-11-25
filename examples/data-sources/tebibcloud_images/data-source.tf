data "tebibcloud_images" "ubuntu" {
  os_family = "ubuntu"
}

output "ubuntu_images" {
  value = data.tebibcloud_images.ubuntu.names
}
