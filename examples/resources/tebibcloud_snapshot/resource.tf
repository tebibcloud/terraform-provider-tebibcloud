resource "tebibcloud_snapshot" "before_upgrade" {
  name  = "before-upgrade"
  vm_id = tebibcloud_vm.web.id
}
