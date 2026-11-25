data "tebibcloud_prices" "now" {}

output "b1_small_per_month" {
  value = data.tebibcloud_prices.now.monthly_by_meter["vm.b1.small"]
}
