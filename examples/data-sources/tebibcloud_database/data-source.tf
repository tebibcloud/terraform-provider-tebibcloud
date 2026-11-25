# A database made elsewhere (the console, the CLI, another configuration).
data "tebibcloud_database" "app" {
  name = "app"
}

output "database_host" {
  value = data.tebibcloud_database.app.host
}

output "database_ca_certificate" {
  value = data.tebibcloud_database.app.ca_certificate
}
