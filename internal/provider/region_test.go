package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// regionalConfig has one of each regional resource. region and zone are set only when not empty.
func regionalConfig(providerBlock, region, zone string) string {
	r, z := "", ""
	if region != "" {
		r = fmt.Sprintf("region = %q", region)
	}
	if zone != "" {
		z = fmt.Sprintf("zone = %q", zone)
	}
	return providerBlock + fmt.Sprintf(`
resource "tebibcloud_vpc" "main" {
  name = "main"
  cidr = "10.1.0.0/16"
  %[1]s
}

resource "tebibcloud_public_ip" "web" {
  name = "web"
  %[1]s
}

resource "tebibcloud_volume" "data" {
  name     = "data"
  size_gib = 10
  %[1]s
  %[2]s
}

resource "tebibcloud_bucket" "assets" {
  name = "assets-bucket"
  %[1]s
}

resource "tebibcloud_load_balancer" "web" {
  name = "web"
  %[1]s
  %[2]s
}
`, r, z)
}

// TestRegionalResourcesTakeARegion: region and zone are optional (the API picks the home region),
// are echoed back, never change on their own, and a changed one replaces the resource.
func TestRegionalResourcesTakeARegion(t *testing.T) {
	srv := newMock(t)
	p := mockProvider(srv)
	resources := []string{
		"tebibcloud_vpc.main", "tebibcloud_public_ip.web", "tebibcloud_volume.data",
		"tebibcloud_bucket.assets", "tebibcloud_load_balancer.web",
	}
	var home, chosen []resource.TestCheckFunc
	for _, r := range resources {
		home = append(home, resource.TestCheckResourceAttr(r, "region", "neo"))
		chosen = append(chosen, resource.TestCheckResourceAttr(r, "region", "neo"))
	}
	home = append(home,
		resource.TestCheckResourceAttr("tebibcloud_volume.data", "zone", "neo-a"),
		resource.TestCheckResourceAttr("tebibcloud_load_balancer.web", "zone", "neo-a"))
	chosen = append(chosen,
		resource.TestCheckResourceAttr("tebibcloud_volume.data", "zone", "neo-b"),
		resource.TestCheckResourceAttr("tebibcloud_load_balancer.web", "zone", "neo-b"))
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				// Nothing named: the platform's home region and a zone it picks.
				Config: regionalConfig(p, "", ""),
				Check:  resource.ComposeAggregateTestCheckFunc(home...),
			},
			{
				// Repeated: no change, whether the region is left out or not.
				Config:           regionalConfig(p, "", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				// Naming the region the resource already has is not a change.
				Config:           regionalConfig(p, "neo", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				// A zone is a change for zonal resources only.
				Config: regionalConfig(p, "neo", "neo-b"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_vpc.main", plancheck.ResourceActionNoop),
					plancheck.ExpectResourceAction("tebibcloud_volume.data", plancheck.ResourceActionDestroyBeforeCreate),
					plancheck.ExpectResourceAction("tebibcloud_load_balancer.web", plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(chosen...),
			},
			{
				// A new region replaces every regional resource.
				Config: regionalConfig(p, "ams", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_vpc.main", plancheck.ResourceActionDestroyBeforeCreate),
					plancheck.ExpectResourceAction("tebibcloud_public_ip.web", plancheck.ResourceActionDestroyBeforeCreate),
					plancheck.ExpectResourceAction("tebibcloud_bucket.assets", plancheck.ResourceActionDestroyBeforeCreate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_vpc.main", "region", "ams"),
					resource.TestCheckResourceAttr("tebibcloud_bucket.assets", "region", "ams"),
				),
			},
		},
	})
}

// TestNatGatewayShowsItsRegion: a NAT gateway follows its VPC, so its region is computed only.
func TestNatGatewayShowsItsRegion(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: mockProvider(srv) + `
resource "tebibcloud_vpc" "main" {
  name = "main"
  cidr = "10.1.0.0/16"
}

resource "tebibcloud_subnet" "web" {
  name   = "web"
  vpc_id = tebibcloud_vpc.main.id
  cidr   = "10.1.1.0/24"
}

resource "tebibcloud_nat_gateway" "main" {
  name      = "main"
  subnet_id = tebibcloud_subnet.web.id
}
`,
				Check: resource.TestCheckResourceAttr("tebibcloud_nat_gateway.main", "region", "neo"),
			},
			{
				Config: mockProvider(srv) + `
resource "tebibcloud_vpc" "main" {
  name = "main"
  cidr = "10.1.0.0/16"
}

resource "tebibcloud_subnet" "web" {
  name   = "web"
  vpc_id = tebibcloud_vpc.main.id
  cidr   = "10.1.1.0/24"
}

resource "tebibcloud_nat_gateway" "main" {
  name      = "main"
  subnet_id = tebibcloud_subnet.web.id
  region    = "neo"
}
`,
				PlanOnly:    true,
				ExpectError: regexpMust(`(?s)region.*(not expected|read-only|Unsupported argument)`),
			},
			{
				// the post-test destroy reuses the last config, so end on a valid one
				Config: mockProvider(srv) + `
resource "tebibcloud_vpc" "main" {
  name = "main"
  cidr = "10.1.0.0/16"
}

resource "tebibcloud_subnet" "web" {
  name   = "web"
  vpc_id = tebibcloud_vpc.main.id
  cidr   = "10.1.1.0/24"
}

resource "tebibcloud_nat_gateway" "main" {
  name      = "main"
  subnet_id = tebibcloud_subnet.web.id
}
`,
			},
		},
	})
}

// TestListenerTLSPolicy: the policy defaults to the only one that exists; others are refused.
func TestListenerTLSPolicy(t *testing.T) {
	srv := newMock(t)
	cfg := func(policy string) string {
		return mockProvider(srv) + fmt.Sprintf(`
resource "tebibcloud_load_balancer" "web" {
  name = "web"
}

resource "tebibcloud_load_balancer_target_group" "web" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  name             = "web"
  port             = 80
}

resource "tebibcloud_load_balancer_listener" "tcp" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  port             = 8080
  target_group_id  = tebibcloud_load_balancer_target_group.web.id
  %s
}
`, policy)
	}
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: cfg(""),
				Check:  resource.TestCheckResourceAttr("tebibcloud_load_balancer_listener.tcp", "tls_policy", "tls12-intermediate"),
			},
			{
				Config:           cfg(`tls_policy = "tls12-intermediate"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}},
			},
			{
				Config:      cfg(`tls_policy = "tls10"`),
				PlanOnly:    true,
				ExpectError: regexpMust(`tls12-intermediate`),
			},
			{
				// the post-test destroy reuses the last config, so end on a valid one
				Config: cfg(""),
			},
		},
	})
}

// TestCatalogueDataSourcesNewFields: the fields the API added to images and instance types.
func TestCatalogueDataSourcesNewFields(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{{
			Config: mockProvider(srv) + `
data "tebibcloud_images" "all" {}

data "tebibcloud_instance_types" "all" {}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.tebibcloud_images.all", "images.0.distro", "ubuntu"),
				resource.TestCheckResourceAttr("data.tebibcloud_images.all", "images.0.distro_name", "Ubuntu"),
				resource.TestCheckResourceAttr("data.tebibcloud_images.all", "images.0.version", "24.04"),
				resource.TestCheckResourceAttr("data.tebibcloud_images.all", "images.0.is_default", "true"),
				resource.TestCheckResourceAttr("data.tebibcloud_images.all", "images.0.sort_order", "10"),
				resource.TestCheckResourceAttr("data.tebibcloud_instance_types.all", "instance_types.0.schedulable", "true"),
				resource.TestCheckResourceAttr("data.tebibcloud_instance_types.all", "instance_types.0.visibility", "active"),
				resource.TestCheckNoResourceAttr("data.tebibcloud_instance_types.all", "instance_types.0.retire_after"),
				resource.TestCheckResourceAttr("data.tebibcloud_instance_types.all", "instance_types.2.visibility", "retiring"),
				resource.TestCheckResourceAttr("data.tebibcloud_instance_types.all", "instance_types.2.retire_after", "2027-01-01T00:00:00Z"),
				resource.TestCheckResourceAttr("data.tebibcloud_instance_types.all", "instance_types.2.schedulable", "false"),
			),
		}},
	})
}
