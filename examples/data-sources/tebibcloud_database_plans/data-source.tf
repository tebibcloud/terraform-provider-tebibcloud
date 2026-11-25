data "tebibcloud_database_plans" "all" {}

output "database_plans" {
  value = data.tebibcloud_database_plans.all.codes
}

resource "tebibcloud_database" "app" {
  name = "app"
  plan = data.tebibcloud_database_plans.all.codes[0] # the smallest plan on offer
}
