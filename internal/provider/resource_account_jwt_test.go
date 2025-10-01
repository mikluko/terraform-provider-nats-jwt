package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAccountJWTResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccAccountJWTResourceConfig("test-account"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nsc_account_jwt.test", "id"),
					resource.TestCheckResourceAttrSet("nsc_account_jwt.test", "public_key"),
					resource.TestCheckResourceAttrSet("nsc_account_jwt.test", "jwt"),
					resource.TestCheckResourceAttr("nsc_account_jwt.test", "name", "test-account"),
				),
			},
			// Update and Read testing
			{
				Config: testAccAccountJWTResourceConfig("test-account-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account_jwt.test", "name", "test-account-updated"),
				),
			},
		},
	})
}

func TestAccAccountJWTResource_withExports(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAccountJWTResourceConfigWithExports(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account_jwt.test", "export.#", "2"),
					resource.TestCheckResourceAttr("nsc_account_jwt.test", "export.0.type", "stream"),
					resource.TestCheckResourceAttr("nsc_account_jwt.test", "export.1.type", "service"),
				),
			},
		},
	})
}

func TestAccAccountJWTResource_circularDependency(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccAccountJWTResourceConfigCircular(),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Service A
					resource.TestCheckResourceAttrSet("nsc_account_jwt.service_a", "id"),
					resource.TestCheckResourceAttrSet("nsc_account_jwt.service_a", "jwt"),
					resource.TestCheckResourceAttr("nsc_account_jwt.service_a", "export.#", "1"),
					resource.TestCheckResourceAttr("nsc_account_jwt.service_a", "import.#", "1"),

					// Service B
					resource.TestCheckResourceAttrSet("nsc_account_jwt.service_b", "id"),
					resource.TestCheckResourceAttrSet("nsc_account_jwt.service_b", "jwt"),
					resource.TestCheckResourceAttr("nsc_account_jwt.service_b", "export.#", "1"),
					resource.TestCheckResourceAttr("nsc_account_jwt.service_b", "import.#", "1"),
				),
			},
		},
	})
}

func testAccAccountJWTResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account_key" "test" {
  name = %[1]q
}

resource "nsc_account_jwt" "test" {
  name          = %[1]q
  account_seed  = nsc_account_key.test.seed
  operator_seed = nsc_operator.test.seed
}
`, name)
}

func testAccAccountJWTResourceConfigWithExports() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account_key" "test" {
  name = "ExportAccount"
}

resource "nsc_account_jwt" "test" {
  name          = "ExportAccount"
  account_seed  = nsc_account_key.test.seed
  operator_seed = nsc_operator.test.seed

  export {
    subject     = "events.>"
    type        = "stream"
    description = "Event stream export"
    advertise   = true
  }

  export {
    subject            = "api.requests"
    type               = "service"
    response_type      = "Singleton"
    response_threshold = "5s"
    token_required     = true
  }
}
`
}

func testAccAccountJWTResourceConfigCircular() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

# Phase 1: Generate keys for both accounts
resource "nsc_account_key" "service_a" {
  name = "service-a"
}

resource "nsc_account_key" "service_b" {
  name = "service-b"
}

# Phase 2: Create JWTs with cross-account imports
resource "nsc_account_jwt" "service_a" {
  name          = "service-a"
  account_seed  = nsc_account_key.service_a.seed
  operator_seed = nsc_operator.test.seed

  export {
    subject = "requests.>"
    type    = "stream"
  }

  import {
    account = nsc_account_key.service_b.public_key
    subject = "responses.>"
    type    = "stream"
  }
}

resource "nsc_account_jwt" "service_b" {
  name          = "service-b"
  account_seed  = nsc_account_key.service_b.seed
  operator_seed = nsc_operator.test.seed

  export {
    subject = "responses.>"
    type    = "stream"
  }

  import {
    account = nsc_account_key.service_a.public_key
    subject = "requests.>"
    type    = "stream"
  }
}
`
}
