package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/mockapi"
)

func projectConfig(srv *mockapi.Server, description string) string {
	return mockProvider(srv) + fmt.Sprintf(`
resource "tebibcloud_project" "staging" {
  name        = "staging"
  description = %q
  tags        = { env = "staging" }
}

data "tebibcloud_project" "by_name" {
  name       = tebibcloud_project.staging.name
  depends_on = [tebibcloud_project.staging]
}

data "tebibcloud_project" "default" {}

data "tebibcloud_vpc" "staging" {
  project_id = tebibcloud_project.staging.id
  default    = true
}

resource "tebibcloud_vpc" "staging" {
  project_id = tebibcloud_project.staging.id
  name       = "app"
  cidr       = "10.20.0.0/16"
}
`, description)
}

func TestProjectLifecycle(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		CheckDestroy: func(*terraform.State) error {
			if p := srv.Find("/projects", "name", "staging"); p != "" {
				return fmt.Errorf("%s still exists", p)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: projectConfig(srv, "first"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_project.staging", "organization_id", mockapi.Org),
					resource.TestCheckResourceAttr("tebibcloud_project.staging", "state", "active"),
					resource.TestCheckResourceAttrPair("data.tebibcloud_project.by_name", "id", "tebibcloud_project.staging", "id"),
					resource.TestCheckResourceAttr("data.tebibcloud_project.default", "id", mockapi.Project),
					resource.TestCheckResourceAttr("data.tebibcloud_vpc.staging", "default", "true"),
					resource.TestCheckResourceAttrPair("data.tebibcloud_vpc.staging", "project_id", "tebibcloud_project.staging", "id"),
					resource.TestCheckResourceAttrPair("tebibcloud_vpc.staging", "project_id", "tebibcloud_project.staging", "id"),
				),
			},
			{
				Config: projectConfig(srv, "second"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_project.staging", plancheck.ResourceActionUpdate),
				}},
				Check: resource.TestCheckResourceAttr("tebibcloud_project.staging", "description", "second"),
			},
			{ResourceName: "tebibcloud_project.staging", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
			{
				ResourceName: "tebibcloud_vpc.staging", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"timeouts"},
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					r := s.RootModule().Resources["tebibcloud_vpc.staging"].Primary
					return r.Attributes["project_id"] + "/" + r.ID, nil
				},
			},
		},
	})
}

func TestPricesDataSourceETB(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: mockProvider(srv) + `
data "tebibcloud_prices" "etb" {
  currency = "ETB"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.tebibcloud_prices.etb", "currency", "ETB"),
					resource.TestCheckResourceAttr("data.tebibcloud_prices.etb", "prices.0.currency", "ETB"),
					resource.TestCheckResourceAttr("data.tebibcloud_prices.etb", "prices.0.unit_price", "1.0000"),
					resource.TestCheckResourceAttr("data.tebibcloud_prices.etb", "monthly_by_meter.vm.b1.small", "730.00"),
					// Meters without a monthly price stay out of the map.
					resource.TestCheckNoResourceAttr("data.tebibcloud_prices.etb", "monthly_by_meter.volume.gib"),
				),
			},
			{
				// A currency the API does not offer is refused, not silently defaulted.
				Config: mockProvider(srv) + `
data "tebibcloud_prices" "bad" {
  currency = "EUR"
}
`,
				ExpectError: regexpMust(`currency must be ETB or USD`),
			},
		},
	})
}

func TestCatalogueDataSources(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{{
			Config: mockProvider(srv) + `
data "tebibcloud_images" "all" {}

data "tebibcloud_images" "ubuntu" {
  os_family = "ubuntu"
}

data "tebibcloud_instance_types" "all" {}

data "tebibcloud_instance_types" "big" {
  min_vcpus = 4
}

data "tebibcloud_prices" "now" {}

data "tebibcloud_vpc" "default" {
  default = true
}

data "tebibcloud_subnet" "default" {
  default = true
  vpc_id  = data.tebibcloud_vpc.default.id
}

data "tebibcloud_security_group" "default" {
  default = true
  vpc_id  = data.tebibcloud_vpc.default.id
}

data "tebibcloud_vpc" "by_name" {
  name = "default-vpc"
}
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.tebibcloud_images.all", "images.#", "2"),
				resource.TestCheckResourceAttr("data.tebibcloud_images.ubuntu", "names.#", "1"),
				resource.TestCheckResourceAttr("data.tebibcloud_images.ubuntu", "names.0", "ubuntu-24.04"),
				resource.TestCheckResourceAttr("data.tebibcloud_images.ubuntu", "images.0.default_user", "ubuntu"),
				resource.TestCheckResourceAttr("data.tebibcloud_instance_types.all", "instance_types.#", "3"),
				resource.TestCheckResourceAttr("data.tebibcloud_instance_types.big", "names.0", "g1.large"),
				resource.TestCheckResourceAttr("data.tebibcloud_prices.now", "currency", "USD"),
				resource.TestCheckResourceAttr("data.tebibcloud_prices.now", "region", "neo"),
				resource.TestCheckResourceAttr("data.tebibcloud_prices.now", "monthly_by_meter.vm.b1.small", "7.30"),
				resource.TestCheckResourceAttr("data.tebibcloud_prices.now", "egress_allowances.0.gb_per_month", "100"),
				resource.TestCheckResourceAttr("data.tebibcloud_vpc.default", "cidr", "10.0.0.0/16"),
				resource.TestCheckResourceAttr("data.tebibcloud_subnet.default", "default", "true"),
				resource.TestCheckResourceAttrPair("data.tebibcloud_subnet.default", "vpc_id", "data.tebibcloud_vpc.default", "id"),
				resource.TestCheckResourceAttr("data.tebibcloud_security_group.default", "name", "default-security-group"),
				resource.TestCheckResourceAttrPair("data.tebibcloud_vpc.by_name", "id", "data.tebibcloud_vpc.default", "id"),
			),
		}},
	})
}

func TestDataSourceLookupNeedsAKey(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{{
			Config:      mockProvider(srv) + `data "tebibcloud_vpc" "x" {}`,
			ExpectError: regexpMust(`(?s)At least one.*id.*name.*default`),
		}},
	})
}

func TestProviderNeedsCredentials(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "tebibcloud" {
  endpoint = %q
  project  = "p-test"
}
data "tebibcloud_images" "all" {}
`, srv.URL),
				ExpectError: regexpMust(`no credentials`),
			},
			{
				Config: fmt.Sprintf(`
provider "tebibcloud" {
  endpoint          = %q
  access_key_id     = "TBKTEST"
  secret_access_key = "wrong"
  project           = "p-test"
}
data "tebibcloud_images" "all" {}
`, srv.URL),
				ExpectError: regexpMust(`Unauthenticated \(HTTP 401\)`),
			},
			{
				Config: fmt.Sprintf(`
provider "tebibcloud" {
  endpoint          = %q
  access_key_id     = "AKIAEXAMPLE"
  secret_access_key = "x"
}
data "tebibcloud_prices" "p" {}
`, srv.URL),
				ExpectError: regexpMust(`not a TebibCloud access key ID`),
			},
		},
	})
}
