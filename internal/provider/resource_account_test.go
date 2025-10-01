package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/nats-io/nkeys"
)

func TestAccAccountResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccAccountResourceConfig("TestAccount"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "name", "TestAccount"),
					resource.TestCheckResourceAttrSet("nsc_account.test", "operator_seed"),
					resource.TestCheckResourceAttr("nsc_account.test", "expiry", "0s"),
					resource.TestCheckResourceAttr("nsc_account.test", "start", "0s"),
					resource.TestCheckResourceAttrSet("nsc_account.test", "jwt"),
					resource.TestCheckResourceAttrSet("nsc_account.test", "seed"),
					resource.TestCheckResourceAttrSet("nsc_account.test", "public_key"),
					testAccCheckAccountPublicKeyFormat("nsc_account.test", "public_key"),
					testAccCheckAccountSeedFormat("nsc_account.test", "seed"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "nsc_account.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccAccountImportStateIdFunc("nsc_account.test"),
				ImportStateVerifyIgnore: []string{
					"jwt",           // JWT contains timestamps
					"operator_seed", // Sensitive and not stored
					"expiry",        // Default value handling
					"start",         // Default value handling
					"allow_pub_response",
				},
			},
			// Update and Read testing
			{
				Config: testAccAccountResourceConfig("UpdatedAccount"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "name", "UpdatedAccount"),
				),
			},
		},
	})
}

func TestAccAccountResource_withLimits(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with limits
			{
				Config: testAccAccountResourceConfigWithLimits(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "name", "LimitedAccount"),
					// Account limits
					resource.TestCheckResourceAttr("nsc_account.test", "max_connections", "100"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_leaf_nodes", "10"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_data", "1073741824"), // 1GB
					resource.TestCheckResourceAttr("nsc_account.test", "max_payload", "1048576"), // 1MB
					resource.TestCheckResourceAttr("nsc_account.test", "max_subscriptions", "1000"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_imports", "50"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_exports", "50"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_wildcard_exports", "false"),
					resource.TestCheckResourceAttr("nsc_account.test", "disallow_bearer_token", "true"),
					// JetStream limits
					resource.TestCheckResourceAttr("nsc_account.test", "max_memory_storage", "536870912"), // 512MB
					resource.TestCheckResourceAttr("nsc_account.test", "max_disk_storage", "10737418240"), // 10GB
					resource.TestCheckResourceAttr("nsc_account.test", "max_streams", "10"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_consumers", "100"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_ack_pending", "1000"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_memory_stream_bytes", "134217728"), // 128MB
					resource.TestCheckResourceAttr("nsc_account.test", "max_disk_stream_bytes", "1073741824"), // 1GB
					resource.TestCheckResourceAttr("nsc_account.test", "max_bytes_required", "true"),
				),
			},
			// Update limits
			{
				Config: testAccAccountResourceConfigWithUpdatedLimits(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "name", "LimitedAccount"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_connections", "200"),
					resource.TestCheckResourceAttr("nsc_account.test", "max_streams", "20"),
				),
			},
		},
	})
}

func TestAccAccountResource_withPermissions(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with permissions
			{
				Config: testAccAccountResourceConfigWithPermissions(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "name", "TestAccount"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_pub.#", "2"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_pub.0", "app.>"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_pub.1", "events.>"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_sub.#", "2"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_sub.0", "app.>"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_sub.1", "metrics.>"),
					resource.TestCheckResourceAttr("nsc_account.test", "deny_pub.#", "1"),
					resource.TestCheckResourceAttr("nsc_account.test", "deny_pub.0", "admin.>"),
					resource.TestCheckResourceAttr("nsc_account.test", "deny_sub.#", "1"),
					resource.TestCheckResourceAttr("nsc_account.test", "deny_sub.0", "secrets.>"),
				),
			},
			// Update permissions
			{
				Config: testAccAccountResourceConfigWithUpdatedPermissions(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "allow_pub.#", "1"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_pub.0", "public.>"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_sub.#", "1"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_sub.0", "public.>"),
				),
			},
		},
	})
}

func TestAccAccountResource_withResponsePermissions(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with response permissions
			{
				Config: testAccAccountResourceConfigWithResponsePermissions(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "name", "TestAccount"),
					resource.TestCheckResourceAttr("nsc_account.test", "allow_pub_response", "5"),
					resource.TestCheckResourceAttr("nsc_account.test", "response_ttl", "10s"),
				),
			},
		},
	})
}

func TestAccAccountResource_withExpiry(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with expiry
			{
				Config: testAccAccountResourceConfigWithExpiry("720h", "24h"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account.test", "expiry", "720h"),
					resource.TestCheckResourceAttr("nsc_account.test", "start", "24h"),
				),
			},
		},
	})
}

func testAccAccountResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = %[1]q
  operator_seed = nsc_operator.test.seed
}
`, name)
}

func testAccAccountResourceConfigWithPermissions() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed

  allow_pub = ["app.>", "events.>"]
  allow_sub = ["app.>", "metrics.>"]
  deny_pub  = ["admin.>"]
  deny_sub  = ["secrets.>"]
}
`
}

func testAccAccountResourceConfigWithUpdatedPermissions() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed

  allow_pub = ["public.>"]
  allow_sub = ["public.>"]
}
`
}

func testAccAccountResourceConfigWithResponsePermissions() string {
	return `
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name               = "TestAccount"
  operator_seed      = nsc_operator.test.seed
  allow_pub_response = 5
  response_ttl       = "10s"
}
`
}

func testAccAccountResourceConfigWithExpiry(expiry, start string) string {
	return fmt.Sprintf(`
resource "nsc_operator" "test" {
  name = "TestOperator"
}

resource "nsc_account" "test" {
  name          = "TestAccount"
  operator_seed = nsc_operator.test.seed
  expiry        = %[1]q
  start         = %[2]q
}
`, expiry, start)
}

func testAccCheckAccountPublicKeyFormat(resourceName, attrName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		publicKey := rs.Primary.Attributes[attrName]
		if publicKey == "" {
			return fmt.Errorf("Public key attribute %s is empty", attrName)
		}

		// Check if it's a valid NATS account public key (starts with 'A')
		if !regexp.MustCompile(`^A[A-Z0-9]{55}$`).MatchString(publicKey) {
			return fmt.Errorf("Invalid account public key format: %s", publicKey)
		}

		return nil
	}
}

func testAccCheckAccountSeedFormat(resourceName, attrName string) resource.TestCheckFunc {
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

		// Verify it's an account seed by checking the seed prefix (SA)
		if !regexp.MustCompile(`^SA[A-Z0-9]{54,}$`).MatchString(seed) {
			return fmt.Errorf("Not an account seed: %s", seed)
		}

		// Verify the keypair can derive a valid account public key
		pubKey, err := kp.PublicKey()
		if err != nil {
			return fmt.Errorf("Failed to get public key from seed: %v", err)
		}
		if !regexp.MustCompile(`^A[A-Z0-9]{55}$`).MatchString(pubKey) {
			return fmt.Errorf("Seed does not produce a valid account public key: %s", pubKey)
		}

		return nil
	}
}

func testAccAccountResourceConfigWithLimits() string {
	return `
resource "nsc_operator" "test" {
  name                 = "TestOperator"
  generate_signing_key = true
}

resource "nsc_account" "test" {
  name          = "LimitedAccount"
  operator_seed = nsc_operator.test.seed

  # Account limits
  max_connections    = 100
  max_leaf_nodes     = 10
  max_data           = 1073741824  # 1GB
  max_payload        = 1048576     # 1MB
  max_subscriptions  = 1000
  max_imports        = 50
  max_exports        = 50
  allow_wildcard_exports = false
  disallow_bearer_token  = true

  # JetStream limits
  max_memory_storage       = 536870912   # 512MB
  max_disk_storage         = 10737418240 # 10GB
  max_streams              = 10
  max_consumers            = 100
  max_ack_pending          = 1000
  max_memory_stream_bytes  = 134217728   # 128MB
  max_disk_stream_bytes    = 1073741824  # 1GB
  max_bytes_required       = true
}
`
}

func testAccAccountResourceConfigWithUpdatedLimits() string {
	return `
resource "nsc_operator" "test" {
  name                 = "TestOperator"
  generate_signing_key = true
}

resource "nsc_account" "test" {
  name          = "LimitedAccount"
  operator_seed = nsc_operator.test.seed

  # Updated account limits
  max_connections    = 200  # Changed
  max_leaf_nodes     = 10
  max_data           = 1073741824
  max_payload        = 1048576
  max_subscriptions  = 1000
  max_imports        = 50
  max_exports        = 50
  allow_wildcard_exports = false
  disallow_bearer_token  = true

  # Updated JetStream limits
  max_memory_storage       = 536870912
  max_disk_storage         = 10737418240
  max_streams              = 20  # Changed
  max_consumers            = 100
  max_ack_pending          = 1000
  max_memory_stream_bytes  = 134217728
  max_disk_stream_bytes    = 1073741824
  max_bytes_required       = true
}
`
}

func testAccAccountImportStateIdFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("Resource not found: %s", resourceName)
		}

		name := rs.Primary.Attributes["name"]
		seed := rs.Primary.Attributes["seed"]
		operatorSeed := rs.Primary.Attributes["operator_seed"]

		if name == "" || seed == "" {
			return "", fmt.Errorf("Name or seed not found in state")
		}

		// Format: name/seed or name/seed/operator_seed
		if operatorSeed != "" {
			return fmt.Sprintf("%s/%s/%s", name, seed, operatorSeed), nil
		}
		return fmt.Sprintf("%s/%s", name, seed), nil
	}
}