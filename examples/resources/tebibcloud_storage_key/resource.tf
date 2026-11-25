resource "tebibcloud_iam_service_account" "app" {
  name = "app"
}

resource "tebibcloud_storage_key" "app" {
  name               = "app"
  service_account_id = tebibcloud_iam_service_account.app.id
}

data "tebibcloud_storage" "this" {}

# For an S3 client (rclone, the AWS SDKs...): path-style URLs.
output "s3" {
  value = {
    endpoint          = data.tebibcloud_storage.this.endpoint
    region            = data.tebibcloud_storage.this.region
    access_key_id     = tebibcloud_storage_key.app.access_key_id
    secret_access_key = tebibcloud_storage_key.app.secret_access_key
  }
  sensitive = true
}
