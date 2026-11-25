package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Acceptance tests run against a real TebibCloud endpoint. They create real, billed resources
// and delete them at the end. Run them with:
//
//	TF_ACC=1 TEBIB_ENDPOINT=... TEBIB_ACCESS_KEY_ID=... TEBIB_SECRET_ACCESS_KEY=... \
//	TEBIB_ORGANIZATION=o-... TEBIB_PROJECT=p-... go test ./internal/provider -run TestAcc -timeout 60m
//
// Use a project made for testing: a failed run can leave resources behind (they are all
// named tf-acc-*).

func accPreCheck(t *testing.T, vars ...string) {
	t.Helper()
	for _, v := range append([]string{"TEBIB_ACCESS_KEY_ID", "TEBIB_SECRET_ACCESS_KEY"}, vars...) {
		if os.Getenv(v) == "" {
			t.Fatalf("%s must be set for acceptance tests", v)
		}
	}
}

func accTest(t *testing.T, c resource.TestCase) {
	t.Helper()
	c.ProtoV6ProviderFactories = protoV6Providers
	resource.Test(t, c)
}

func TestAccFullStack(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(6)
	pubkey := os.Getenv("TEBIB_ACC_SSH_PUBLIC_KEY")
	if pubkey == "" {
		// A throwaway key; nobody holds its private half.
		pubkey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIupiqSPww/kJbS2J2DgQCT/JYGFxnt4KhONzm3i992R tf-acc"
	}
	cfg := func(itype string) string {
		return fmt.Sprintf(`
provider "tebibcloud" {}

data "tebibcloud_images" "ubuntu" {
  os_family = "ubuntu"
}

data "tebibcloud_instance_types" "all" {}

resource "tebibcloud_vpc" "main" {
  name = %[1]q
  cidr = "10.77.0.0/16"
}

resource "tebibcloud_subnet" "main" {
  name   = %[1]q
  vpc_id = tebibcloud_vpc.main.id
  cidr   = "10.77.1.0/24"
}

resource "tebibcloud_security_group" "web" {
  name   = %[1]q
  vpc_id = tebibcloud_vpc.main.id
}

resource "tebibcloud_security_group_rule" "ssh" {
  security_group_id = tebibcloud_security_group.web.id
  protocol          = "tcp"
  port_from         = 22
  remote_cidr       = "0.0.0.0/0"
}

resource "tebibcloud_public_ip" "nat" {
  name = "%[1]s-nat"
}

resource "tebibcloud_nat_gateway" "main" {
  name         = %[1]q
  subnet_id    = tebibcloud_subnet.main.id
  public_ip_id = tebibcloud_public_ip.nat.id
}

resource "tebibcloud_ssh_key" "me" {
  name       = %[1]q
  public_key = %[3]q
}

resource "tebibcloud_vm" "web" {
  name               = %[1]q
  image              = data.tebibcloud_images.ubuntu.names[0]
  instance_type      = %[2]q
  subnet_id          = tebibcloud_subnet.main.id
  security_group_ids = [tebibcloud_security_group.web.id]
  ssh_key_ids        = [tebibcloud_ssh_key.me.id]
  user_data          = "#cloud-config\npackages: [nginx]\n"
}

resource "tebibcloud_volume" "data" {
  name     = %[1]q
  size_gib = 10
}

resource "tebibcloud_volume_attachment" "data" {
  volume_id = tebibcloud_volume.data.id
  vm_id     = tebibcloud_vm.web.id
}

resource "tebibcloud_public_ip" "web" {
  name = "%[1]s-web"
}

resource "tebibcloud_public_ip_association" "web" {
  public_ip_id = tebibcloud_public_ip.web.id
  vm_id        = tebibcloud_vm.web.id
  depends_on   = [tebibcloud_nat_gateway.main]
}
`, name, itype, pubkey)
	}
	accTest(t, resource.TestCase{
		PreCheck: func() { accPreCheck(t, "TEBIB_PROJECT") },
		Steps: []resource.TestStep{
			{
				Config: cfg("b1.small"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_vm.web", "state", "active"),
					resource.TestCheckResourceAttr("tebibcloud_vm.web", "current_power_state", "running"),
					resource.TestCheckResourceAttrSet("tebibcloud_vm.web", "ipv4_address"),
					resource.TestCheckResourceAttrSet("tebibcloud_public_ip_association.web", "address"),
				),
			},
			{ResourceName: "tebibcloud_vm.web", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"timeouts", "user_data", "status_message", "guest_agent", "restart_required"}},
		},
	})
}

func TestAccIAM(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(6)
	accTest(t, resource.TestCase{
		PreCheck: func() { accPreCheck(t, "TEBIB_ORGANIZATION") },
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
provider "tebibcloud" {}

resource "tebibcloud_iam_service_account" "ci" {
  name = %[1]q
}

resource "tebibcloud_iam_access_key" "ci" {
  service_account_id = tebibcloud_iam_service_account.ci.id
}

resource "tebibcloud_iam_policy" "read" {
  name = %[1]q
  document = jsonencode({
    Version   = "2026-10-08"
    Statement = [{ Effect = "Allow", Action = ["compute:Describe*"], Resource = "*" }]
  })
}

resource "tebibcloud_iam_policy_attachment" "ci" {
  policy    = tebibcloud_iam_policy.read.id
  principal = tebibcloud_iam_service_account.ci.trn
}
`, name),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("tebibcloud_iam_access_key.ci", "secret"),
				resource.TestCheckResourceAttr("tebibcloud_iam_policy_attachment.ci", "principal_type", "service-account"),
			),
		}},
	})
}

// TestAccStorageAndDNS needs no compute: a bucket, a storage key, and a DNS zone for a domain
// nobody delegates to us (it stays pending_verification) with a few record sets.
func TestAccStorageAndDNS(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(6)
	cfg := func(visibility string, mxTTL int) string {
		return fmt.Sprintf(`
provider "tebibcloud" {}

data "tebibcloud_storage" "this" {
  depends_on = [tebibcloud_bucket.data]
}

resource "tebibcloud_bucket" "data" {
  name          = %[1]q
  visibility    = %[2]q
  max_size_gib  = 1
  force_destroy = true
}

resource "tebibcloud_storage_key" "ci" {
  name = %[1]q
}

resource "tebibcloud_dns_zone" "acc" {
  domain = "%[1]s.com"
}

resource "tebibcloud_dns_record" "www" {
  zone_id = tebibcloud_dns_zone.acc.id
  name    = "www"
  type    = "CNAME"
  values  = ["%[1]s.com"]
}

resource "tebibcloud_dns_record" "mx" {
  zone_id = tebibcloud_dns_zone.acc.id
  type    = "MX"
  ttl     = %[3]d
  values  = ["10 mail.%[1]s.com"]
}

resource "tebibcloud_dns_record" "spf" {
  zone_id = tebibcloud_dns_zone.acc.id
  type    = "TXT"
  values  = ["v=spf1 -all"]
}
`, name, visibility, mxTTL)
	}
	accTest(t, resource.TestCase{
		PreCheck: func() { accPreCheck(t, "TEBIB_PROJECT") },
		Steps: []resource.TestStep{
			{
				Config: cfg("private", 3600),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_bucket.data", "state", "active"),
					resource.TestCheckResourceAttrSet("tebibcloud_bucket.data", "url"),
					resource.TestCheckResourceAttrSet("tebibcloud_storage_key.ci", "secret_access_key"),
					resource.TestCheckResourceAttr("tebibcloud_storage_key.ci", "effective_status", "active"),
					resource.TestCheckResourceAttrSet("data.tebibcloud_storage.this", "region"),
					resource.TestCheckResourceAttr("tebibcloud_dns_zone.acc", "status", "pending_verification"),
					resource.TestCheckResourceAttr("tebibcloud_dns_zone.acc", "nameservers.#", "2"),
					resource.TestCheckResourceAttr("tebibcloud_dns_record.www", "canonical_values.0", name+".com."),
				),
			},
			{
				// In place: the MX TTL. (A public bucket needs a verified account, which the
				// test organization may not be, so the visibility stays.)
				Config: cfg("private", 600),
				Check:  resource.TestCheckResourceAttr("tebibcloud_dns_record.mx", "ttl", "600"),
			},
			{ResourceName: "tebibcloud_bucket.data", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"timeouts", "force_destroy", "status_message", "stored_bytes", "object_count"}},
			{ResourceName: "tebibcloud_dns_zone.acc", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"timeouts", "status_message"}},
		},
	})
}

// TestAccLoadBalancer puts a VM behind a public load balancer, with a snapshot schedule on
// the VM. It checks the wiring, not traffic (TestAccFullStack covers a VM on the internet).
// It needs the capacity for a VM and the load balancer's nodes, and two public IPv4 addresses
// (the NAT gateway's and the load balancer's).
func TestAccLoadBalancer(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(6)
	cfg := func(tgPort int) string {
		return fmt.Sprintf(`
provider "tebibcloud" {}

data "tebibcloud_images" "ubuntu" {
  os_family = "ubuntu"
}

data "tebibcloud_vpc" "default" {
  default = true
}

data "tebibcloud_subnet" "default" {
  vpc_id  = data.tebibcloud_vpc.default.id
  default = true
}

# Public load balancers are reached through their VPC's NAT gateway.
resource "tebibcloud_public_ip" "nat" {
  name = "%[1]s-nat"
}

resource "tebibcloud_nat_gateway" "main" {
  name         = %[1]q
  subnet_id    = data.tebibcloud_subnet.default.id
  public_ip_id = tebibcloud_public_ip.nat.id
}

resource "tebibcloud_vm" "web" {
  name          = %[1]q
  image         = data.tebibcloud_images.ubuntu.names[0]
  instance_type = "b1.small"
  subnet_id     = data.tebibcloud_subnet.default.id
}

resource "tebibcloud_snapshot_schedule" "nightly" {
  name            = %[1]q
  vm_id           = tebibcloud_vm.web.id
  frequency       = "daily"
  hour            = 3
  retention_count = 2
  enabled         = false
}

resource "tebibcloud_load_balancer" "web" {
  name       = %[1]q
  depends_on = [tebibcloud_nat_gateway.main]
}

resource "tebibcloud_load_balancer_target_group" "web" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  name             = "web"
  port             = %[2]d
}

resource "tebibcloud_load_balancer_listener" "http" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  port             = 80
  target_group_id  = tebibcloud_load_balancer_target_group.web.id
}

resource "tebibcloud_load_balancer_target" "web" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  target_group_id  = tebibcloud_load_balancer_target_group.web.id
  vm_id            = tebibcloud_vm.web.id
}
`, name, tgPort)
	}
	accTest(t, resource.TestCase{
		PreCheck: func() { accPreCheck(t, "TEBIB_PROJECT") },
		Steps: []resource.TestStep{
			{
				Config: cfg(80),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_load_balancer.web", "state", "active"),
					resource.TestCheckResourceAttrSet("tebibcloud_load_balancer.web", "public_address"),
					resource.TestCheckResourceAttr("tebibcloud_load_balancer_target.web", "effective_port", "80"),
					resource.TestCheckResourceAttr("tebibcloud_snapshot_schedule.nightly", "effective_cron", "0 3 * * *"),
				),
			},
			{
				Config: cfg(8080),
				Check:  resource.TestCheckResourceAttr("tebibcloud_load_balancer_target_group.web", "port", "8080"),
			},
			{
				// The target follows its group's port; Terraform sees it on the next refresh.
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr("tebibcloud_load_balancer_target.web", "effective_port", "8080"),
			},
			{ResourceName: "tebibcloud_load_balancer_target_group.web", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: importID("tebibcloud_load_balancer_target_group.web", "load_balancer_id", "id"), ImportStateVerifyIgnore: []string{"timeouts"}},
			{ResourceName: "tebibcloud_snapshot_schedule.nightly", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"timeouts", "status_message", "next_run_at"}},
		},
	})
}
