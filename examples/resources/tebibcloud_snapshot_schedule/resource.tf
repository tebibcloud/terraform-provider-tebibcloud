resource "tebibcloud_snapshot_schedule" "nightly" {
  name            = "nightly"
  vm_id           = tebibcloud_vm.db.id
  frequency       = "daily"
  hour            = 2
  minute          = 30
  timezone        = "Africa/Nairobi"
  retention_count = 7
}
