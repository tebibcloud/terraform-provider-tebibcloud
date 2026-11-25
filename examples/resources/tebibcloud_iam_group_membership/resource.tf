resource "tebibcloud_iam_group_membership" "abel" {
  group_id = tebibcloud_iam_group.ops.id
  user_id  = "u-abc123"
}
