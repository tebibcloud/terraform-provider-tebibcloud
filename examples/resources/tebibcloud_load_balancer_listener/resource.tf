resource "tebibcloud_load_balancer_listener" "api" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  port             = 8443
  target_group_id  = tebibcloud_load_balancer_target_group.api.id
}
