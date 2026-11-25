resource "tebibcloud_iam_policy_attachment" "ci_vm_operator" {
  policy     = tebibcloud_iam_policy.vm_operator.id
  principal  = tebibcloud_iam_service_account.ci.trn
  project_id = "p-def456" # leave out for the whole organization
}
