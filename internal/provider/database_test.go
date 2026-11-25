package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/mockapi"
)

func databaseConfig(srv *mockapi.Server, body string) string {
	return mockProvider(srv) + fmt.Sprintf(`
resource "tebibcloud_database" "app" {
  name = "app"
%s
}
`, body)
}

// TestDatabaseLifecycle follows a database from a small single instance to a high-availability
// one with backups, then opens it to the internet with an allow-list and a fallback port, and
// imports it. The owner's password is only known at create.
func TestDatabaseLifecycle(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		CheckDestroy: func(*terraform.State) error {
			if srv.Count("/projects/p-test/databases") != 0 {
				return fmt.Errorf("databases left")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: databaseConfig(srv, ``),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_database.app", "state", "active"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "plan", "b1.micro"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "mode", "single"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "version", "18"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "backup_enabled", "false"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "public_enabled", "false"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "port", "5432"),
					resource.TestCheckResourceAttrSet("tebibcloud_database.app", "password"),
					resource.TestCheckResourceAttrSet("tebibcloud_database.app", "host"),
					resource.TestCheckResourceAttrSet("tebibcloud_database.app", "ca_certificate"),
					resource.TestCheckNoResourceAttr("tebibcloud_database.app", "public_host"),
				),
			},
			{
				// Plan, mode and backups change in place; the password is kept in the state.
				Config: databaseConfig(srv, `
  plan                  = "b1.medium"
  mode                  = "ha"
  disk_gib              = 20
  backup_enabled        = true
  backup_retention_days = 14
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_database.app", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_database.app", "plan", "b1.medium"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "mode", "ha"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "disk_gib", "20"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "backup_enabled", "true"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "backup_retention_days", "14"),
					resource.TestCheckResourceAttrSet("tebibcloud_database.app", "password"),
				),
			},
			{
				// The public endpoint: an allow-list, direct TLS and a fallback port.
				Config: databaseConfig(srv, `
  plan                  = "b1.medium"
  mode                  = "ha"
  disk_gib              = 20
  backup_enabled        = true
  backup_retention_days = 14
  public_enabled        = true
  allow_list            = ["203.0.113.0/24"]
  fallback_enabled      = true
`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_database.app", plancheck.ResourceActionUpdate),
				}},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_database.app", "public_enabled", "true"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "allow_list.#", "1"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "fallback_enabled", "true"),
					resource.TestCheckResourceAttr("tebibcloud_database.app", "fallback_port", "15000"),
					resource.TestCheckResourceAttrSet("tebibcloud_database.app", "public_host"),
				),
			},
			{
				ResourceName: "tebibcloud_database.app", ImportState: true, ImportStateVerify: true,
				// An imported database has no password; skip_final_backup is only ours.
				ImportStateVerifyIgnore: []string{"timeouts", "password", "skip_final_backup", "status_message"},
			},
		},
	})
}

func TestDatabaseShrinkingTheDiskIsRefused(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: databaseConfig(srv, `  disk_gib = 20`)},
			{
				Config:      databaseConfig(srv, `  disk_gib = 10`),
				ExpectError: regexpMust(`never shrink`),
			},
		},
	})
}

func TestDatabaseOpenedWithoutAnAllowListIsRefused(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config:      databaseConfig(srv, `  public_enabled = true`),
				ExpectError: regexpMust(`allow_list`),
			},
		},
	})
}

func TestDatabaseVersionChangeReplaces(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{Config: databaseConfig(srv, `  version = 18`)},
			{
				Config: databaseConfig(srv, `  version = 17`),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_database.app", plancheck.ResourceActionDestroyBeforeCreate),
				}},
			},
		},
	})
}
