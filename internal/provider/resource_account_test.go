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
					resource.TestCheckResourceAttr("natsjwt_account.test", "name", "TestAccount"),
					resource.TestCheckResourceAttrSet("natsjwt_account.test", "operator_seed"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "expiry", "0s"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "start", "0s"),
					resource.TestCheckResourceAttrSet("natsjwt_account.test", "jwt"),
					resource.TestCheckResourceAttrSet("natsjwt_account.test", "seed"),
					resource.TestCheckResourceAttrSet("natsjwt_account.test", "public_key"),
					testAccCheckAccountPublicKeyFormat("natsjwt_account.test", "public_key"),
					testAccCheckAccountSeedFormat("natsjwt_account.test", "seed"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "natsjwt_account.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccAccountImportStateIdFunc("natsjwt_account.test"),
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
					resource.TestCheckResourceAttr("natsjwt_account.test", "name", "UpdatedAccount"),
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
					resource.TestCheckResourceAttr("natsjwt_account.test", "name", "TestAccount"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_pub.#", "2"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_pub.0", "app.>"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_pub.1", "events.>"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_sub.#", "2"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_sub.0", "app.>"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_sub.1", "metrics.>"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "deny_pub.#", "1"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "deny_pub.0", "admin.>"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "deny_sub.#", "1"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "deny_sub.0", "secrets.>"),
				),
			},
			// Update permissions
			{
				Config: testAccAccountResourceConfigWithUpdatedPermissions(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_pub.#", "1"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_pub.0", "public.>"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_sub.#", "1"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_sub.0", "public.>"),
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
					resource.TestCheckResourceAttr("natsjwt_account.test", "name", "TestAccount"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "allow_pub_response", "5"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "response_ttl", "10s"),
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
					resource.TestCheckResourceAttr("natsjwt_account.test", "expiry", "720h"),
					resource.TestCheckResourceAttr("natsjwt_account.test", "start", "24h"),
				),
			},
		},
	})
}

func testAccAccountResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "natsjwt_operator" "test" {
  name = "TestOperator"
}

resource "natsjwt_account" "test" {
  name          = %[1]q
  operator_seed = natsjwt_operator.test.seed
}
`, name)
}

func testAccAccountResourceConfigWithPermissions() string {
	return `
resource "natsjwt_operator" "test" {
  name = "TestOperator"
}

resource "natsjwt_account" "test" {
  name          = "TestAccount"
  operator_seed = natsjwt_operator.test.seed

  allow_pub = ["app.>", "events.>"]
  allow_sub = ["app.>", "metrics.>"]
  deny_pub  = ["admin.>"]
  deny_sub  = ["secrets.>"]
}
`
}

func testAccAccountResourceConfigWithUpdatedPermissions() string {
	return `
resource "natsjwt_operator" "test" {
  name = "TestOperator"
}

resource "natsjwt_account" "test" {
  name          = "TestAccount"
  operator_seed = natsjwt_operator.test.seed

  allow_pub = ["public.>"]
  allow_sub = ["public.>"]
}
`
}

func testAccAccountResourceConfigWithResponsePermissions() string {
	return `
resource "natsjwt_operator" "test" {
  name = "TestOperator"
}

resource "natsjwt_account" "test" {
  name               = "TestAccount"
  operator_seed      = natsjwt_operator.test.seed
  allow_pub_response = 5
  response_ttl       = "10s"
}
`
}

func testAccAccountResourceConfigWithExpiry(expiry, start string) string {
	return fmt.Sprintf(`
resource "natsjwt_operator" "test" {
  name = "TestOperator"
}

resource "natsjwt_account" "test" {
  name          = "TestAccount"
  operator_seed = natsjwt_operator.test.seed
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