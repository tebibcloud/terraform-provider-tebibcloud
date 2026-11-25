resource "tebibcloud_volume_attachment" "data" {
  volume_id = tebibcloud_volume.data.id
  vm_id     = tebibcloud_vm.web.id
}
