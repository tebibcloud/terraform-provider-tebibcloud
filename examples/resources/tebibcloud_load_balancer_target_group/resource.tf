resource "tebibcloud_load_balancer_target_group" "api" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  name             = "api"
  protocol         = "tcp"
  port             = 8080
  algorithm        = "least_connections"
  proxy_protocol   = true
}
