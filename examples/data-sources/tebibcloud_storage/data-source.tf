data "tebibcloud_storage" "this" {}

output "s3_endpoint" {
  value = data.tebibcloud_storage.this.endpoint
}
