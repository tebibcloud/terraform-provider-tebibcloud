package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// TestDatabasePlansDataSource reads the plans on offer, the versions and the windows.
func TestDatabasePlansDataSource(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{{
			Config: mockProvider(srv) + `data "tebibcloud_database_plans" "all" {}`,
			Check: resource.ComposeAggregateTestCheckFunc(
				// b1.large is not on offer, so it is not listed.
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "codes.#", "4"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "codes.0", "b1.nano"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "codes.1", "b1.micro"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "plans.0.cpu_millicores", "500"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "plans.0.memory_mib", "512"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "plans.0.schedulable", "true"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "plans.1.name", "db.b1.micro"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "default_version", "18"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "versions.#", "2"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "maintenance_windows.#", "7"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "default_maintenance_window", "sun-02"),
				resource.TestCheckResourceAttr("data.tebibcloud_database_plans.all", "ha_enabled", "true"),
			),
		}},
	})
}

// TestDatabaseDataSource finds a database by name and by ID. It has no password, and a
// lookup that names nothing, or both, is refused.
func TestDatabaseDataSource(t *testing.T) {
	srv := newMock(t)
	cfg := databaseConfig(srv, `  plan = "b1.nano"`) + `
data "tebibcloud_database" "by_name" {
  name = tebibcloud_database.app.name
}

data "tebibcloud_database" "by_id" {
  id = tebibcloud_database.app.id
}
`
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair("data.tebibcloud_database.by_name", "id", "tebibcloud_database.app", "id"),
					resource.TestCheckResourceAttrPair("data.tebibcloud_database.by_id", "name", "tebibcloud_database.app", "name"),
					resource.TestCheckResourceAttr("data.tebibcloud_database.by_name", "plan", "b1.nano"),
					resource.TestCheckResourceAttr("data.tebibcloud_database.by_name", "cpu_millicores", "500"),
					resource.TestCheckResourceAttr("data.tebibcloud_database.by_name", "memory_mib", "512"),
					resource.TestCheckResourceAttr("data.tebibcloud_database.by_name", "region", "neo"),
					resource.TestCheckResourceAttr("data.tebibcloud_database.by_name", "port", "5432"),
					resource.TestCheckResourceAttr("data.tebibcloud_database.by_name", "backup_enabled", "false"),
					resource.TestCheckResourceAttr("data.tebibcloud_database.by_name", "public_enabled", "false"),
					resource.TestCheckResourceAttrPair("data.tebibcloud_database.by_name", "host", "tebibcloud_database.app", "host"),
					resource.TestCheckResourceAttrSet("data.tebibcloud_database.by_name", "ca_certificate"),
					resource.TestCheckNoResourceAttr("data.tebibcloud_database.by_name", "password"),
				),
			},
			{
				// Repeated: the same config again reads the same database and plans no change.
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectEmptyPlan(),
				}},
			},
			{
				Config:      mockProvider(srv) + `data "tebibcloud_database" "none" {}`,
				ExpectError: regexpMust(`(?s)(No attribute specified|one of).*`),
			},
			{
				Config: mockProvider(srv) + `
data "tebibcloud_database" "both" {
  id   = "pgdb-x"
  name = "app"
}
`,
				ExpectError: regexpMust(`(?s)(Invalid Attribute Combination|only one of).*`),
			},
			{
				Config:      mockProvider(srv) + `data "tebibcloud_database" "gone" { name = "nope" }`,
				ExpectError: regexpMust(`no database matches`),
			},
		},
	})
}

// TestDatabasePlanIsCheckedAgainstTheAPI: an unknown plan, an empty one and a plan that is
// not on offer fail at plan time, and a refused change leaves the database as it was.
func TestDatabasePlanIsCheckedAgainstTheAPI(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config:      databaseConfig(srv, `  plan = "b1.huge"`),
				PlanOnly:    true,
				ExpectError: regexpMust(`Unknown database plan[\s\S]*b1\.huge[\s\S]*b1\.micro`),
			},
			{
				Config:      databaseConfig(srv, `  plan = "b1.large"`), // exists, but not on offer
				PlanOnly:    true,
				ExpectError: regexpMust(`Unknown database plan`),
			},
			{
				Config:      databaseConfig(srv, `  plan = ""`),
				PlanOnly:    true,
				ExpectError: regexpMust(`(?s)at least 1`),
			},
			{
				Config: databaseConfig(srv, `  plan = "b1.nano"`),
				Check:  resource.TestCheckResourceAttr("tebibcloud_database.app", "plan", "b1.nano"),
			},
			{
				// A refused change plans nothing and the database keeps its plan.
				Config:      databaseConfig(srv, `  plan = "b1.huge"`),
				PlanOnly:    true,
				ExpectError: regexpMust(`Unknown database plan`),
			},
			{
				Config: databaseConfig(srv, `  plan = "b1.nano"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectEmptyPlan(),
				}},
			},
			{
				// No plan in the config keeps the one the database has.
				Config: databaseConfig(srv, ``),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectEmptyPlan(),
				}},
				Check: resource.TestCheckResourceAttr("tebibcloud_database.app", "plan", "b1.nano"),
			},
			{
				// A valid change goes through.
				Config: databaseConfig(srv, `  plan = "b1.small"`),
				Check:  resource.TestCheckResourceAttr("tebibcloud_database.app", "plan", "b1.small"),
			},
		},
	})
}

func TestDatabaseDiskBelowTheFloorIsRefused(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{{
			Config:      databaseConfig(srv, `  disk_gib = 4`),
			PlanOnly:    true,
			ExpectError: regexpMust(`(?s)at least 5`),
		}},
	})
}

// TestDatabaseRegionAndComputedSize: region and zone are chosen at create, size fields come back.
func TestDatabaseRegionAndComputedSize(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: databaseConfig(srv, "  region = \"neo\"\n  zone   = \"neo-b\"\n  plan   = \"b1.medium\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_database.app", "region", "neo"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "zone", "neo-b"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "vcpus", "2"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "cpu_millicores", "2000"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "memory_mib", "4096"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "power_state", "running"),
					resource.TestCheckNoResourceAttr("tebibcloud_database.app", "disk_used_bytes"),
				),
			},
			{
				// A resize changes the computed size with it.
				Config: databaseConfig(srv, "  region = \"neo\"\n  zone   = \"neo-b\"\n  plan   = \"b1.nano\""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_database.app", "cpu_millicores", "500"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "memory_mib", "512"),
				),
			},
			{
				// A new zone replaces the database.
				Config: databaseConfig(srv, "  region = \"neo\"\n  zone   = \"neo-a\"\n  plan   = \"b1.nano\""),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_database.app", plancheck.ResourceActionDestroyBeforeCreate),
				}},
			},
		},
	})
}
