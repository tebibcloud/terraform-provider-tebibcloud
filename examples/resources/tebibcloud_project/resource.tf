resource "tebibcloud_project" "staging" {
  name        = "staging"
  description = "Staging environment"
  tags        = { env = "staging" }
}
