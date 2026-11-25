package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/mockapi"
)

func networkConfig(srv *mockapi.Server, env string) string {
	return mockProvider(srv) + fmt.Sprintf(`
resource "tebibcloud_vpc" "main" {
  name = "main"
  cidr = "10.1.0.0/16"
  tags = { env = %q }
}

resource "tebibcloud_subnet" "web" {
  name   = "web"
  vpc_id = tebibcloud_vpc.main.id
  cidr   = "10.1.1.0/24"
}

resource "tebibcloud_security_group" "web" {
  name        = "web"
  vpc_id      = tebibcloud_vpc.main.id
  description = "web servers"
}

resource "tebibcloud_security_group_rule" "ssh" {
  security_group_id = tebibcloud_security_group.web.id
  protocol          = "tcp"
  port_from         = 22
  remote_cidr       = "10.0.0.1/24"
  description       = "ssh from the office"
}

resource "tebibcloud_security_group_rule" "https6" {
  security_group_id = tebibcloud_security_group.web.id
  protocol          = "tcp"
  port_from         = 443
  port_to           = 443
  remote_cidr       = "::/0"
}

resource "tebibcloud_security_group_rule" "peers" {
  security_group_id = tebibcloud_security_group.web.id
  protocol          = "all"
  remote_group_id   = tebibcloud_security_group.web.id
  ip_version        = "ipv4"
}

resource "tebibcloud_public_ip" "nat" {
  name = "nat"
}

resource "tebibcloud_nat_gateway" "main" {
  name         = "main"
  subnet_id    = tebibcloud_subnet.web.id
  public_ip_id = tebibcloud_public_ip.nat.id
}
`, env)
}

func TestNetworkLifecycle(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		CheckDestroy: func(*terraform.State) error {
			for _, coll := range []string{"vpcs", "subnets", "security-groups", "nat-gateways", "public-ips"} {
				if p := srv.Find("/projects/p-test/"+coll, "name", "main"); p != "" {
					return fmt.Errorf("%s still exists", p)
				}
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: networkConfig(srv, "test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_vpc.main", "state", "active"),
					resource.TestCheckResourceAttr("tebibcloud_vpc.main", "project_id", mockapi.Project),
					resource.TestCheckResourceAttr("tebibcloud_vpc.main", "is_default", "false"),
					resource.TestCheckResourceAttrSet("tebibcloud_vpc.main", "ipv6_cidr"),
					resource.TestCheckResourceAttr("tebibcloud_vpc.main", "tags.env", "test"),
					resource.TestCheckResourceAttr("tebibcloud_subnet.web", "gateway", "10.1.1.1"),
					resource.TestCheckResourceAttrPair("tebibcloud_subnet.web", "vpc_id", "tebibcloud_vpc.main", "id"),
					resource.TestCheckResourceAttr("tebibcloud_security_group.web", "description", "web servers"),
					resource.TestCheckResourceAttr("tebibcloud_security_group_rule.ssh", "port_to", "22"),
					resource.TestCheckResourceAttr("tebibcloud_security_group_rule.ssh", "ip_version", "ipv4"),
					resource.TestCheckResourceAttr("tebibcloud_security_group_rule.ssh", "direction", "ingress"),
					// The API spells it 10.0.0.0/24; the user's spelling is kept.
					resource.TestCheckResourceAttr("tebibcloud_security_group_rule.ssh", "remote_cidr", "10.0.0.1/24"),
					resource.TestCheckResourceAttr("tebibcloud_security_group_rule.https6", "ip_version", "ipv6"),
					resource.TestCheckResourceAttrPair("tebibcloud_security_group_rule.peers", "remote_group_id", "tebibcloud_security_group.web", "id"),
					resource.TestCheckResourceAttrPair("tebibcloud_nat_gateway.main", "public_address", "tebibcloud_public_ip.nat", "address"),
					resource.TestCheckResourceAttrPair("tebibcloud_nat_gateway.main", "vpc_id", "tebibcloud_vpc.main", "id"),
				),
			},
			{
				// Tags change in place.
				Config: networkConfig(srv, "prod"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_vpc.main", plancheck.ResourceActionUpdate),
					plancheck.ExpectResourceAction("tebibcloud_subnet.web", plancheck.ResourceActionNoop),
				}},
				Check: resource.TestCheckResourceAttr("tebibcloud_vpc.main", "tags.env", "prod"),
			},
			{ResourceName: "tebibcloud_vpc.main", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts", "status_message", "has_nat_gateway"}},
			{ResourceName: "tebibcloud_subnet.web", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
			{ResourceName: "tebibcloud_security_group.web", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
			{ResourceName: "tebibcloud_nat_gateway.main", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
			{ResourceName: "tebibcloud_public_ip.nat", ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"timeouts"}},
			{
				ResourceName: "tebibcloud_security_group_rule.ssh", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					r := s.RootModule().Resources["tebibcloud_security_group_rule.ssh"].Primary
					return mockapi.Project + "/" + r.Attributes["security_group_id"] + "/" + r.ID, nil
				},
				// The import sees the API's spelling.
				ImportStateVerifyIgnore: []string{"timeouts", "remote_cidr"},
			},
			{
				// Drift: the rule was removed in the dashboard. Terraform adds it back.
				PreConfig: func() {
					sg := srv.Find("/projects/p-test/security-groups", "name", "web")
					srv.Mutate(sg, func(o map[string]any) {
						var keep []any
						for _, r := range o["rules"].([]any) {
							if r.(map[string]any)["description"] != "ssh from the office" {
								keep = append(keep, r)
							}
						}
						o["rules"] = keep
					})
				},
				Config: networkConfig(srv, "prod"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_security_group_rule.ssh", plancheck.ResourceActionCreate),
				}},
			},
			{
				// Drift: tags changed in the dashboard. Terraform puts them back.
				PreConfig: func() {
					srv.Mutate(srv.Find("/projects/p-test/vpcs", "name", "main"), func(o map[string]any) {
						o["tags"] = map[string]any{"env": "changed"}
					})
				},
				Config: networkConfig(srv, "prod"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_vpc.main", plancheck.ResourceActionUpdate),
				}},
				Check: resource.TestCheckResourceAttr("tebibcloud_vpc.main", "tags.env", "prod"),
			},
			{
				// Drift: the public IP was released outside Terraform. It is made again, and the
				// NAT gateway that depends on it is replaced.
				PreConfig: func() { srv.Remove(srv.Find("/projects/p-test/public-ips", "name", "nat")) },
				Config:    networkConfig(srv, "prod"),
				ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{
					plancheck.ExpectResourceAction("tebibcloud_public_ip.nat", plancheck.ResourceActionCreate),
					plancheck.ExpectResourceAction("tebibcloud_nat_gateway.main", plancheck.ResourceActionDestroyBeforeCreate),
				}},
			},
		},
	})
	assertCalled(t, srv, "POST /api/v1/projects/p-test/security-groups/", "DELETE /api/v1/projects/p-test/vpcs/")
}

// assertCalled checks the fake API saw calls starting with each prefix.
func assertCalled(t *testing.T, srv *mockapi.Server, prefixes ...string) {
	t.Helper()
	calls := srv.Calls()
	for _, p := range prefixes {
		found := false
		for _, c := range calls {
			if strings.HasPrefix(c, p) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no call starting with %q among %d calls", p, len(calls))
		}
	}
}

func TestSecurityGroupRuleValidation(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: mockProvider(srv) + `
resource "tebibcloud_security_group_rule" "bad" {
  security_group_id = "sg-1"
  protocol          = "icmp"
  port_from         = 22
  remote_cidr       = "0.0.0.0/0"
}`,
				ExpectError: regexpMust(`Ports need tcp or udp`),
			},
			{
				Config: mockProvider(srv) + `
resource "tebibcloud_security_group_rule" "bad" {
  security_group_id = "sg-1"
  protocol          = "tcp"
}`,
				ExpectError: regexpMust(`(?s)remote_cidr.*remote_group_id`),
			},
		},
	})
}
