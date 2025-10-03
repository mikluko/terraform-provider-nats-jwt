package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccUserResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResourceConfig("TestUser"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "name", "TestUser"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "subject"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "issuer_seed"),
					resource.TestCheckResourceAttr("nsc_user.test", "expiry", "0s"),
					resource.TestCheckResourceAttr("nsc_user.test", "start", "0s"),
					resource.TestCheckResourceAttr("nsc_user.test", "bearer", "false"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "jwt"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "public_key"),
					testAccCheckUserPublicKeyFormat("nsc_user.test", "public_key"),
					testAccCheckUserPublicKeyFormat("nsc_user.test", "subject"),
				),
			},
			// Update and Read testing
			{
				Config: testAccUserResourceConfig("UpdatedUser"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "name", "UpdatedUser"),
				),
			},
		},
	})
}

func TestAccUserResource_withPermissions(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with permissions
			{
				Config: testAccUserResourceConfigWithPermissions(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "name", "TestUser"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_pub.#", "2"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_pub.0", "app.events.>"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_pub.1", "app.requests.>"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_sub.#", "2"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_sub.0", "app.>"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_sub.1", "metrics.>"),
					resource.TestCheckResourceAttr("nsc_user.test", "deny_pub.#", "1"),
					resource.TestCheckResourceAttr("nsc_user.test", "deny_pub.0", "app.admin.>"),
					resource.TestCheckResourceAttr("nsc_user.test", "deny_sub.#", "1"),
					resource.TestCheckResourceAttr("nsc_user.test", "deny_sub.0", "app.secrets.>"),
				),
			},
			// Update permissions
			{
				Config: testAccUserResourceConfigWithUpdatedPermissions(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "allow_pub.#", "1"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_pub.0", "public.>"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_sub.#", "1"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_sub.0", "public.>"),
				),
			},
		},
	})
}

func TestAccUserResource_withLimits(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with limits
			{
				Config: testAccUserResourceConfigWithLimits(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "name", "LimitedUser"),
					resource.TestCheckResourceAttr("nsc_user.test", "max_subscriptions", "100"),
					resource.TestCheckResourceAttr("nsc_user.test", "max_data", "1048576"), // 1MB
					resource.TestCheckResourceAttr("nsc_user.test", "max_payload", "4096"), // 4KB
					resource.TestCheckResourceAttr("nsc_user.test", "allowed_connection_types.#", "2"),
					resource.TestCheckResourceAttr("nsc_user.test", "allowed_connection_types.0", "STANDARD"),
					resource.TestCheckResourceAttr("nsc_user.test", "allowed_connection_types.1", "WEBSOCKET"),
				),
			},
			// Update limits
			{
				Config: testAccUserResourceConfigWithUpdatedLimits(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "name", "LimitedUser"),
					resource.TestCheckResourceAttr("nsc_user.test", "max_subscriptions", "200"),
					resource.TestCheckResourceAttr("nsc_user.test", "max_data", "2097152"), // 2MB
					resource.TestCheckResourceAttr("nsc_user.test", "max_payload", "8192"), // 8KB
					resource.TestCheckResourceAttr("nsc_user.test", "allowed_connection_types.#", "3"),
					resource.TestCheckResourceAttr("nsc_user.test", "allowed_connection_types.0", "STANDARD"),
					resource.TestCheckResourceAttr("nsc_user.test", "allowed_connection_types.1", "WEBSOCKET"),
					resource.TestCheckResourceAttr("nsc_user.test", "allowed_connection_types.2", "MQTT"),
				),
			},
		},
	})
}

func TestAccUserResource_withResponsePermissions(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with response permissions
			{
				Config: testAccUserResourceConfigWithResponsePermissions(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "name", "TestUser"),
					resource.TestCheckResourceAttr("nsc_user.test", "allow_pub_response", "3"),
					resource.TestCheckResourceAttr("nsc_user.test", "response_ttl", "5s"),
				),
			},
		},
	})
}

func TestAccUserResource_withBearerAndTags(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with bearer and tags
			{
				Config: testAccUserResourceConfigWithBearerAndTags(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "bearer", "true"),
					resource.TestCheckResourceAttr("nsc_user.test", "tag.#", "2"),
					resource.TestCheckResourceAttr("nsc_user.test", "tag.0", "backend"),
					resource.TestCheckResourceAttr("nsc_user.test", "tag.1", "service"),
					// When bearer = true, jwt_sensitive should be populated (jwt not checked as it's null)
					resource.TestCheckResourceAttrSet("nsc_user.test", "jwt_sensitive"),
				),
			},
		},
	})
}

func TestAccUserResource_jwtSensitiveWithoutBearer(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create without bearer (default bearer = false)
			{
				Config: testAccUserResourceConfig("TestUser"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "bearer", "false"),
					// When bearer = false, both jwt and jwt_sensitive should be populated
					resource.TestCheckResourceAttrSet("nsc_user.test", "jwt"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "jwt_sensitive"),
				),
			},
		},
	})
}

func TestAccUserResource_withSourceNetwork(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with source network restrictions
			{
				Config: testAccUserResourceConfigWithSourceNetwork(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "source_network.#", "2"),
					resource.TestCheckResourceAttr("nsc_user.test", "source_network.0", "192.168.1.0/24"),
					resource.TestCheckResourceAttr("nsc_user.test", "source_network.1", "10.0.0.0/8"),
				),
			},
		},
	})
}

func TestAccUserResource_withExpiry(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with expiry
			{
				Config: testAccUserResourceConfigWithExpiry("720h", "24h"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_user.test", "expiry", "720h"),
					resource.TestCheckResourceAttr("nsc_user.test", "start", "24h"),
				),
			},
		},
	})
}

func testAccUserResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name        = %[1]q
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed
}
`, name)
}

func testAccUserResourceConfigWithPermissions() string {
	return `
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name        = "TestUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  allow_pub = ["app.events.>", "app.requests.>"]
  allow_sub = ["app.>", "metrics.>"]
  deny_pub  = ["app.admin.>"]
  deny_sub  = ["app.secrets.>"]
}
`
}

func testAccUserResourceConfigWithUpdatedPermissions() string {
	return `
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name        = "TestUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  allow_pub = ["public.>"]
  allow_sub = ["public.>"]
}
`
}

func testAccUserResourceConfigWithLimits() string {
	return `
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name        = "LimitedUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  max_subscriptions       = 100
  max_data                = 1048576  # 1MB
  max_payload             = 4096     # 4KB
  allowed_connection_types = ["STANDARD", "WEBSOCKET"]
}
`
}

func testAccUserResourceConfigWithUpdatedLimits() string {
	return `
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name        = "LimitedUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  max_subscriptions       = 200      # Changed
  max_data                = 2097152  # 2MB Changed
  max_payload             = 8192     # 8KB Changed
  allowed_connection_types = ["STANDARD", "WEBSOCKET", "MQTT"]  # Changed
}
`
}

func testAccUserResourceConfigWithResponsePermissions() string {
	return `
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name               = "TestUser"
  subject            = nsc_nkey.user.public_key
  issuer_seed        = nsc_nkey.account.seed
  allow_pub_response = 3
  response_ttl       = "5s"
}
`
}

func testAccUserResourceConfigWithBearerAndTags() string {
	return `
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name        = "TestUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed
  bearer      = true
  tag         = ["backend", "service"]
}
`
}

func testAccUserResourceConfigWithSourceNetwork() string {
	return `
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name           = "TestUser"
  subject        = nsc_nkey.user.public_key
  issuer_seed    = nsc_nkey.account.seed
  source_network = ["192.168.1.0/24", "10.0.0.0/8"]
}
`
}

func testAccUserResourceConfigWithExpiry(expiry, start string) string {
	return fmt.Sprintf(`
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

resource "nsc_operator" "test" {
  name        = "TestOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "test" {
  name        = "TestAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "test" {
  name        = "TestUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed
  expiry      = %[1]q
  start       = %[2]q
}
`, expiry, start)
}

func testAccCheckUserPublicKeyFormat(resourceName, attrName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		publicKey := rs.Primary.Attributes[attrName]
		if publicKey == "" {
			return fmt.Errorf("Public key attribute %s is empty", attrName)
		}

		// Check if it's a valid NATS user public key (starts with 'U')
		if !regexp.MustCompile(`^U[A-Z0-9]{55}$`).MatchString(publicKey) {
			return fmt.Errorf("Invalid user public key format: %s", publicKey)
		}

		return nil
	}
}

func testAccCheckUserCredsFormat(resourceName, attrName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		creds := rs.Primary.Attributes[attrName]
		if creds == "" {
			return fmt.Errorf("Creds attribute %s is empty", attrName)
		}

		// Check if it contains the expected JWT and seed delimiters
		if !strings.Contains(creds, "-----BEGIN NATS USER JWT-----") {
			return fmt.Errorf("Creds file missing JWT header")
		}
		if !strings.Contains(creds, "-----BEGIN USER NKEY SEED-----") {
			return fmt.Errorf("Creds file missing seed header")
		}

		return nil
	}
}
