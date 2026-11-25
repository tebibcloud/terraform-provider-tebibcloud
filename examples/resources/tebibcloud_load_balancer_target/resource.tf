resource "tebibcloud_load_balancer_target" "api" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  target_group_id  = tebibcloud_load_balancer_target_group.api.id
  vm_id            = tebibcloud_vm.api.id
  port             = 9000
}
