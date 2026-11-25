resource "tebibcloud_database" "app" {
  name = "app"
  plan = "b1.small" # see the tebibcloud_database_plans data source
  mode = "ha"

  backup_enabled        = true
  backup_retention_days = 14
}

# The owner role is "app"; connect from a VM of the same VPC.
output "database_host" {
  value = tebibcloud_database.app.host
}

output "database_password" {
  value     = tebibcloud_database.app.password
  sensitive = true
}
