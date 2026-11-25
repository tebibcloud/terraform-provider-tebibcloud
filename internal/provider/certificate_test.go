package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// -- tebibcloud_certificate (ADR 0051) -------------------------------------------------------------

func certDestroyed(srv interface{ Count(string) int }) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if srv.Count("/projects/p-test/certificates") != 0 {
			return fmt.Errorf("certificate left")
		}
		return nil
	}
}

func TestManagedCertificateLifecycle(t *testing.T) {
	srv := newMock(t)
	cfg := func(extra string) string {
		return mockProvider(srv) + fmt.Sprintf(`
resource "tebibcloud_certificate" "site" {
  domain_names = ["www.example-site.dev", "*.example-site.dev"]
  validation   = "dns"
  %s
}

data "tebibcloud_certificate" "by_name" {
  name = tebibcloud_certificate.site.name
}
`, extra)
	}
	unitTest(t, resource.TestCase{
		CheckDestroy: certDestroyed(srv),
		Steps: []resource.TestStep{
			{
				Config: cfg(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_certificate.site", "source", "managed"),
					resource.TestCheckResourceAttr("tebibcloud_certificate.site", "status", "issued"),
					resource.TestCheckResourceAttr("tebibcloud_certificate.site", "domain_names.#", "2"),
					resource.TestCheckResourceAttr("tebibcloud_certificate.site", "validation", "dns"),
					resource.TestCheckResourceAttrSet("tebibcloud_certificate.site", "fingerprint"),
					resource.TestCheckResourceAttr("tebibcloud_certificate.site", "test_certificate", "false"),
					resource.TestCheckResourceAttrPair("data.tebibcloud_certificate.by_name", "id", "tebibcloud_certificate.site", "id"),
					resource.TestCheckResourceAttr("data.tebibcloud_certificate.by_name", "status", "issued"),
				),
			},
			{
				// Tags and the description change in place.
				Config: cfg(`description = "main site"`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("tebibcloud_certificate.site", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttr("tebibcloud_certificate.site", "description", "main site"),
			},
			{
				ResourceName: "tebibcloud_certificate.site", ImportState: true, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"timeouts", "status_message"},
			},
		},
	})
}

func TestChangingTheNamesReplacesTheCertificate(t *testing.T) {
	srv := newMock(t)
	cfg := func(name string) string {
		return mockProvider(srv) + fmt.Sprintf(`
resource "tebibcloud_certificate" "site" {
  domain_names = [%q]
  validation   = "http"
}
`, name)
	}
	unitTest(t, resource.TestCase{
		CheckDestroy: certDestroyed(srv),
		Steps: []resource.TestStep{
			{Config: cfg("a.example-site.dev")},
			{
				Config: cfg("b.example-site.dev"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("tebibcloud_certificate.site", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
		},
	})
}

func TestImportedCertificateSealsTheKeyAndKeepsItSensitive(t *testing.T) {
	srv := newMock(t)
	cfg := mockProvider(srv) + `
resource "tebibcloud_certificate" "imported" {
  certificate       = "-----BEGIN CERTIFICATE-----\nleaf\n-----END CERTIFICATE-----\n"
  certificate_chain = "-----BEGIN CERTIFICATE-----\nchain\n-----END CERTIFICATE-----\n"
  private_key       = "-----BEGIN PRIVATE KEY-----\nSECRETKEYBODY\n-----END PRIVATE KEY-----\n"
}
`
	unitTest(t, resource.TestCase{
		CheckDestroy: certDestroyed(srv),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_certificate.imported", "source", "imported"),
					resource.TestCheckResourceAttr("tebibcloud_certificate.imported", "status", "issued"),
					// The mock API refuses a request whose key isn't a sealed box, so reaching
					// "issued" means the provider sealed it. The API never saw the PEM.
					func(*terraform.State) error {
						for _, o := range srv.Dump("/projects/p-test/certificates") {
							if strings.Contains(fmt.Sprint(o), "SECRETKEYBODY") {
								return fmt.Errorf("the API object carries the key")
							}
						}
						return nil
					},
				),
			},
			{
				// A changed key replaces the certificate; the key is never read back.
				Config: strings.Replace(cfg, "SECRETKEYBODY", "OTHERBODY", 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("tebibcloud_certificate.imported", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
			},
		},
	})
}

func TestACertificateNeedsNamesOrAnImport(t *testing.T) {
	srv := newMock(t)
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config:      mockProvider(srv) + "resource \"tebibcloud_certificate\" \"x\" {}\n",
				ExpectError: regexpMust(`domain_names|Missing arguments`),
			},
			{
				Config: mockProvider(srv) + `
resource "tebibcloud_certificate" "x" {
  certificate = "pem"
}
`,
				ExpectError: regexpMust(`private_key`),
			},
		},
	})
}

func TestHTTPSListenerWithCertificatesAndARedirect(t *testing.T) {
	srv := newMock(t)
	cfg := func(certs string) string {
		return mockProvider(srv) + fmt.Sprintf(`
resource "tebibcloud_certificate" "a" {
  domain_names = ["a.example-site.dev"]
  validation   = "dns"
}

resource "tebibcloud_certificate" "b" {
  domain_names = ["b.example-site.dev"]
  validation   = "dns"
}

resource "tebibcloud_load_balancer" "web" {
  name = "web"
}

resource "tebibcloud_load_balancer_target_group" "web" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  name             = "web"
  protocol         = "http"
  port             = 8080
}

resource "tebibcloud_load_balancer_listener" "https" {
  load_balancer_id = tebibcloud_load_balancer.web.id
  protocol         = "https"
  port             = 443
  target_group_id  = tebibcloud_load_balancer_target_group.web.id
  certificates     = %s
}

resource "tebibcloud_load_balancer_listener" "redirect" {
  load_balancer_id  = tebibcloud_load_balancer.web.id
  protocol          = "http"
  port              = 80
  redirect_to_https = true
}
`, certs)
	}
	unitTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				Config: cfg("[tebibcloud_certificate.a.id]"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tebibcloud_load_balancer_listener.https", "protocol", "https"),
					resource.TestCheckResourceAttr("tebibcloud_load_balancer_listener.https", "certificates.#", "1"),
					resource.TestCheckResourceAttrPair("tebibcloud_load_balancer_listener.https", "certificates.0", "tebibcloud_certificate.a", "id"),
					resource.TestCheckResourceAttr("tebibcloud_load_balancer_listener.https", "status", "active"),
					resource.TestCheckResourceAttr("tebibcloud_load_balancer_listener.redirect", "redirect_to_https", "true"),
					resource.TestCheckResourceAttr("tebibcloud_load_balancer_listener.redirect", "redirect_port", "443"),
					resource.TestCheckNoResourceAttr("tebibcloud_load_balancer_listener.redirect", "target_group_id"),
				),
			},
			{
				// The ordered certificate list changes in place.
				Config: cfg("[tebibcloud_certificate.b.id, tebibcloud_certificate.a.id]"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("tebibcloud_load_balancer_listener.https", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.TestCheckResourceAttrPair("tebibcloud_load_balancer_listener.https", "certificates.0", "tebibcloud_certificate.b", "id"),
			},
			{
				ResourceName: "tebibcloud_load_balancer_listener.https", ImportState: true, ImportStateVerify: true,
				ImportStateIdFunc:       importID("tebibcloud_load_balancer_listener.https", "load_balancer_id", "id"),
				ImportStateVerifyIgnore: []string{"timeouts", "project_id"},
			},
		},
	})
}
