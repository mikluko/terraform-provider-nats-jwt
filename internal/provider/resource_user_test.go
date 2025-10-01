package provider

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/nats-io/nkeys"
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
					resource.TestCheckResourceAttrSet("nsc_user.test", "account_seed"),
					resource.TestCheckResourceAttr("nsc_user.test", "expiry", "0s"),
					resource.TestCheckResourceAttr("nsc_user.test", "start", "0s"),
					resource.TestCheckResourceAttr("nsc_user.test", "bearer", "false"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "jwt"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "seed"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "public_key"),
					resource.TestCheckResourceAttrSet("nsc_user.test", "creds"),
					testAccCheckUserPublicKeyFormat("nsc_user.test", "public_key"),
					testAccCheckUserSeedFormat("nsc_user.test", "seed"),
					testAccCheckUserCredsFormat("nsc_user.test", "creds"),
				),
			},
			// ImportState testing
			{
				ResourceName:            "nsc_user.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateIdFunc:       testAccUserImportStateIdFunc("nsc_user.test"),
				ImportStateVerifyIgnore: []string{"jwt", "creds", "account_seed", "expiry", "start", "bearer"}, // JWT/creds contain timestamps, account_seed is sensitive, defaults handling
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
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name         = %[1]q
  account_seed = nsc_account.test.seed
}
`, name)
}

func testAccUserResourceConfigWithPermissions() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name         = "TestUser"
  account_seed = nsc_account.test.seed

  allow_pub = ["app.events.>", "app.requests.>"]
  allow_sub = ["app.>", "metrics.>"]
  deny_pub  = ["app.admin.>"]
  deny_sub  = ["app.secrets.>"]
}
`
}

func testAccUserResourceConfigWithUpdatedPermissions() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name         = "TestUser"
  account_seed = nsc_account.test.seed

  allow_pub = ["public.>"]
  allow_sub = ["public.>"]
}
`
}

func testAccUserResourceConfigWithResponsePermissions() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name               = "TestUser"
  account_seed       = nsc_account.test.seed
  allow_pub_response = 3
  response_ttl       = "5s"
}
`
}

func testAccUserResourceConfigWithBearerAndTags() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name         = "TestUser"
  account_seed = nsc_account.test.seed
  bearer       = true
  tag          = ["backend", "service"]
}
`
}

func testAccUserResourceConfigWithSourceNetwork() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name           = "TestUser"
  account_seed   = nsc_account.test.seed
  source_network = ["192.168.1.0/24", "10.0.0.0/8"]
}
`
}

func testAccUserResourceConfigWithExpiry(expiry, start string) string {
	return fmt.Sprintf(`
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name         = "TestUser"
  account_seed = nsc_account.test.seed
  expiry       = %[1]q
  start        = %[2]q
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

func testAccCheckUserSeedFormat(resourceName, attrName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		seed := rs.Primary.Attributes[attrName]
		if seed == "" {
			return fmt.Errorf("Seed attribute %s is empty", attrName)
		}

		// Try to create keypair from seed to validate it
		kp, err := nkeys.FromSeed([]byte(seed))
		if err != nil {
			return fmt.Errorf("Invalid seed format: %v", err)
		}

		// Verify it's a user seed by checking the seed prefix (SU)
		if !regexp.MustCompile(`^SU[A-Z0-9]{54,}$`).MatchString(seed) {
			return fmt.Errorf("Not a user seed: %s", seed)
		}

		// Verify the keypair can derive a valid user public key
		pubKey, err := kp.PublicKey()
		if err != nil {
			return fmt.Errorf("Failed to get public key from seed: %v", err)
		}
		if !regexp.MustCompile(`^U[A-Z0-9]{55}$`).MatchString(pubKey) {
			return fmt.Errorf("Seed does not produce a valid user public key: %s", pubKey)
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

		// Check if creds contains the expected format
		if !strings.Contains(creds, "-----BEGIN NATS USER JWT-----") {
			return fmt.Errorf("Creds missing JWT header")
		}
		if !strings.Contains(creds, "------END NATS USER JWT------") {
			return fmt.Errorf("Creds missing JWT footer")
		}
		if !strings.Contains(creds, "-----BEGIN USER NKEY SEED-----") {
			return fmt.Errorf("Creds missing seed header")
		}
		if !strings.Contains(creds, "------END USER NKEY SEED------") {
			return fmt.Errorf("Creds missing seed footer")
		}

		return nil
	}
}

func testAccUserResourceConfigWithLimits() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name         = "LimitedUser"
  account_seed = nsc_account.test.seed

  # User limits
  max_subscriptions = 100
  max_data          = 1048576  # 1MB
  max_payload       = 4096     # 4KB

  # Connection type restrictions
  allowed_connection_types = ["STANDARD", "WEBSOCKET"]
}
`
}

func testAccUserResourceConfigWithUpdatedLimits() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
}

resource "nsc_user" "test" {
  name         = "LimitedUser"
  account_seed = nsc_account.test.seed

  # Updated user limits
  max_subscriptions = 200       # Changed
  max_data          = 2097152   # 2MB - Changed
  max_payload       = 8192      # 8KB - Changed

  # Updated connection type restrictions
  allowed_connection_types = ["STANDARD", "WEBSOCKET", "MQTT"]  # Added MQTT
}
`
}

func testAccUserImportStateIdFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("Resource not found: %s", resourceName)
		}

		name := rs.Primary.Attributes["name"]
		seed := rs.Primary.Attributes["seed"]
		accountSeed := rs.Primary.Attributes["account_seed"]

		if name == "" || seed == "" {
			return "", fmt.Errorf("Name or seed not found in state")
		}

		// Format: name/seed or name/seed/account_seed
		if accountSeed != "" {
			return fmt.Sprintf("%s/%s/%s", name, seed, accountSeed), nil
		}
		return fmt.Sprintf("%s/%s", name, seed), nil
	}
}
