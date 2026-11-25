# Changelog

## 0.1.0 (2026-11-25)

First release as `tebibcloud/tebibcloud`, a new provider in the `tebibcloud` namespace. It carries
everything from `tebibhost/tebib` 0.1.0 plus the never-published 0.2.0 below, and:

- **Breaking: the provider is renamed `tebibcloud`.** The registry address is
  `tebibcloud/tebibcloud` (was `tebibhost/tebib`), every resource and data source is
  `tebibcloud_*` (was `tebib_*`), the provider block is `provider "tebibcloud"` and the Go module
  is `github.com/tebibcloud/terraform-provider-tebibcloud`. `tebibhost/tebib` 0.1.0 stays
  where it is and gets no more releases (0.2.0 was never published). Settings are unchanged: the `TEBIB_*` environment
  variables and the `tebib` CLI profile files work as before.

  To move a configuration:

  1. In `required_providers` use `tebibcloud = { source = "tebibcloud/tebibcloud" }`, and
     `provider "tebibcloud" {}`.
  2. Rename the names in your `.tf` files:
     `sed -i -E 's/(^|[^A-Za-z0-9_])tebib_([a-z])/\1tebibcloud_\2/g; s/provider "tebib"/provider "tebibcloud"/' *.tf`
  3. Rewrite the state (back it up first; `moved` blocks can't change a resource's type, and
     `terraform state replace-provider` changes only the provider address, not the type names).
     `jq` changes only each resource's `type`, provider address and `dependencies`, never your
     values; do not push if the check prints `false`:

     ```sh
     tofu state pull > state.json && cp state.json state.backup.json
     jq '.serial += 1 | .resources |= map(.type |= sub("^tebib_"; "tebibcloud_") | .provider |= sub("/tebibhost/tebib\"\\]$"; "/tebibcloud/tebibcloud\"]") | .instances |= map(if .dependencies then .dependencies |= map(sub("^(?<d>data\\.)?tebib_"; "\(.d // "")tebibcloud_")) else . end))' state.json > state.new.json
     jq -e '[.resources[] | select((.type | startswith("tebib_")) or (.provider | test("/tebibhost/tebib\"\\]$")))] | length == 0' state.new.json
     tofu init -upgrade && tofu state push state.new.json
     tofu plan   # must show no replacement
     ```

- New data sources `tebibcloud_database` (by `id` or `name`; everything but the password) and
  `tebibcloud_database_plans` (plan codes and sizes, Postgres versions, maintenance windows,
  whether HA is on).
- `tebibcloud_database.plan` is checked against the plans the API offers when you plan, and the
  error lists them. It no longer defaults to `b1.small` in the provider: left out, the platform's
  default plan (`b1.micro`) is used and kept. An empty `plan` is refused. `disk_gib` must be at
  least 5. New attributes: `region`, `zone`, `vcpus`, `cpu_millicores`, `memory_mib`,
  `power_state`, `disk_used_bytes`.
- Fields the API has that the provider lacked: `region` on `tebibcloud_vpc`,
  `tebibcloud_public_ip`, `tebibcloud_volume`, `tebibcloud_bucket`, `tebibcloud_load_balancer` and
  `tebibcloud_database`, `zone` on `tebibcloud_volume`, `tebibcloud_load_balancer` and
  `tebibcloud_database`; `region` (computed) on `tebibcloud_nat_gateway`; `tls_policy` on
  `tebibcloud_load_balancer_listener`; `last_used_ip` on `tebibcloud_iam_access_key`. All are
  optional, default to what the API picks, and a changed one replaces the resource. Data sources:
  `tebibcloud_images` gains `distro`, `distro_name`, `version`, `is_default`, `sort_order`;
  `tebibcloud_instance_types` gains `schedulable`, `visibility`, `retire_after`.

## History: `tebibhost/tebib`

The provider's earlier name. Its versions below stay on the registry as `tebibhost/tebib` and get no
more releases.

### 0.2.0 (2026-11-19, never published)

This release never reached the registry: its public repository was to live in a `tebibcloud` GitHub
organization that did not exist yet. Its changes ship in `tebibcloud/tebibcloud` 0.1.0.

- The provider moves to the `tebibcloud` namespace: `source = "tebibcloud/tebib"` and the Go module
  `github.com/tebibcloud/terraform-provider-tebib`. The `tebibhost/tebib` 0.1.0 stays where it is; it is not
  updated.

- Load balancers: target group protocol `https` (TLS from the load balancer to the targets) with
  `verify_backend` (off by default), `ca_bundle` and `server_name`; health check protocol `https`.
  A target group's `name` and `protocol` now change in place (a `protocol` only to one its
  listeners can forward to). Listener: `managed_certificate` requests a managed certificate with
  the listener (`status` `certificate_pending` until it is issued and attached automatically),
  `pending_certificate_id`; `certificates` is now also computed.

- New resource and data source `tebib_certificate` (managed from Let's Encrypt, or imported with a
  sealed private key; `private_key` is sensitive, kept in state and never read back). Listener
  resource: protocol `https`, `certificates`, `redirect_to_https`, `redirect_port`, `status`;
  `target_group_id` is optional (a redirect listener has none).

- New resource `tebib_database` (managed Postgres, ADR 0046): plan, single or HA, disk (grow
  only), backups (off by default, 1-30 days), maintenance window. The owner's password is a
  sensitive attribute known only at create. Private path only: the public endpoint is not in the
  provider yet.
- Breaking: `tebib_prices` follows the two fixed price lists (ETB for Ethiopia, USD for every
  other country). New optional argument `currency` (`ETB` or `USD`, default `USD`). Renamed:
  `unit_price_usd` is `unit_price`, `monthly_usd` is `monthly`, `monthly_usd_by_meter` is
  `monthly_by_meter`; each price also has `currency`.

### 0.1.0 (2026-11-07)

First version.

- Resources: `tebib_project`, `tebib_vpc`, `tebib_subnet`, `tebib_security_group`,
  `tebib_security_group_rule`, `tebib_nat_gateway`, `tebib_public_ip`,
  `tebib_public_ip_association`, `tebib_ssh_key`, `tebib_vm`, `tebib_volume`,
  `tebib_volume_attachment`, `tebib_snapshot`, `tebib_iam_service_account`,
  `tebib_iam_access_key`, `tebib_iam_role`, `tebib_iam_policy`,
  `tebib_iam_policy_attachment`, `tebib_iam_group`, `tebib_iam_group_membership`,
  `tebib_bucket`, `tebib_storage_key`, `tebib_load_balancer`,
  `tebib_load_balancer_target_group`, `tebib_load_balancer_listener`,
  `tebib_load_balancer_target`, `tebib_dns_zone`, `tebib_dns_record`,
  `tebib_snapshot_schedule`.
- Data sources: `tebib_project`, `tebib_images`, `tebib_instance_types`, `tebib_prices`,
  `tebib_vpc`, `tebib_subnet`, `tebib_security_group`, `tebib_storage`.
- Preview: breaking changes are allowed before 1.0.0 and are listed here.
