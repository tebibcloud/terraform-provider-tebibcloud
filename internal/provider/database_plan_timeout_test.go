package provider

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// TestDatabasePlanCheckDoesNotHoldUpThePlan: when the plan catalogue never answers, the check
// gives up after its short timeout, makes one call per try (no retries), and the plan and apply
// still succeed (the API checks the plan again on create).
func TestDatabasePlanCheckDoesNotHoldUpThePlan(t *testing.T) {
	old := planCatalogueTimeout
	planCatalogueTimeout = 200 * time.Millisecond
	t.Cleanup(func() { planCatalogueTimeout = old })

	srv := newMock(t)
	var start time.Time
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{{
			PreConfig: func() { srv.HangPlans(true); start = time.Now() },
			Config:    databaseConfig(srv, `  plan = "b1.nano"`),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("tebibcloud_database.app", "plan", "b1.nano"),
				func(*terraform.State) error {
					srv.HangPlans(false)
					if d := time.Since(start); d > 10*time.Second {
						return fmt.Errorf("plan and apply took %s", d)
					}
					// A try per plan, never the client's four retries each.
					n := 0
					for _, c := range srv.Calls() {
						if strings.HasSuffix(c, "/database-plans") {
							n++
						}
					}
					if n == 0 || n > 6 {
						return fmt.Errorf("%d catalogue calls", n)
					}
					return nil
				},
			),
		}},
	})
}
