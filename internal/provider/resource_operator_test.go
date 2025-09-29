package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/nats-io/nkeys"
)

func TestAccOperatorResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccOperatorResourceConfig("TestOperator"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("natsjwt_operator.test", "name", "TestOperator"),
					resource.TestCheckResourceAttr("natsjwt_operator.test", "generate_signing_key", "false"),
					resource.TestCheckResourceAttr("natsjwt_operator.test", "expiry", "0s"),
					resource.TestCheckResourceAttr("natsjwt_operator.test", "start", "0s"),
					resource.TestCheckResourceAttrSet("natsjwt_operator.test", "jwt"),
					resource.TestCheckResourceAttrSet("natsjwt_operator.test", "seed"),
					resource.TestCheckResourceAttrSet("natsjwt_operator.test", "public_key"),
					resource.TestCheckNoResourceAttr("natsjwt_operator.test", "signing_key"),
					resource.TestCheckNoResourceAttr("natsjwt_operator.test", "signing_key_seed"),
					testAccCheckOperatorPublicKeyFormat("natsjwt_operator.test", "public_key"),
					testAccCheckOperatorSeedFormat("natsjwt_operator.test", "seed"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "natsjwt_operator.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccOperatorImportStateIdFunc("natsjwt_operator.test"),
				ImportStateVerifyIgnore: []string{
					"jwt",    // JWT contains timestamps
					"expiry", // Default value handling
					"start",  // Default value handling
					"generate_signing_key",
				},
			},
			// Update and Read testing
			{
				Config: testAccOperatorResourceConfig("UpdatedOperator"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("natsjwt_operator.test", "name", "UpdatedOperator"),
				),
			},
		},
	})
}

func TestAccOperatorResource_withSigningKey(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with signing key
			{
				Config: testAccOperatorResourceConfigWithSigningKey("TestOperator"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("natsjwt_operator.test", "name", "TestOperator"),
					resource.TestCheckResourceAttr("natsjwt_operator.test", "generate_signing_key", "true"),
					resource.TestCheckResourceAttrSet("natsjwt_operator.test", "signing_key"),
					resource.TestCheckResourceAttrSet("natsjwt_operator.test", "signing_key_seed"),
					testAccCheckOperatorPublicKeyFormat("natsjwt_operator.test", "signing_key"),
					testAccCheckOperatorSeedFormat("natsjwt_operator.test", "signing_key_seed"),
				),
			},
		},
	})
}

func TestAccOperatorResource_withExpiry(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with expiry
			{
				Config: testAccOperatorResourceConfigWithExpiry("TestOperator", "720h", "24h"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("natsjwt_operator.test", "name", "TestOperator"),
					resource.TestCheckResourceAttr("natsjwt_operator.test", "expiry", "720h"),
					resource.TestCheckResourceAttr("natsjwt_operator.test", "start", "24h"),
				),
			},
			// Update expiry
			{
				Config: testAccOperatorResourceConfigWithExpiry("TestOperator", "1440h", "48h"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("natsjwt_operator.test", "expiry", "1440h"),
					resource.TestCheckResourceAttr("natsjwt_operator.test", "start", "48h"),
				),
			},
		},
	})
}

func testAccOperatorResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "natsjwt_operator" "test" {
  name = %[1]q
}
`, name)
}

func testAccOperatorResourceConfigWithSigningKey(name string) string {
	return fmt.Sprintf(`
resource "natsjwt_operator" "test" {
  name                 = %[1]q
  generate_signing_key = true
}
`, name)
}

func testAccOperatorResourceConfigWithExpiry(name, expiry, start string) string {
	return fmt.Sprintf(`
resource "natsjwt_operator" "test" {
  name   = %[1]q
  expiry = %[2]q
  start  = %[3]q
}
`, name, expiry, start)
}

func testAccCheckOperatorPublicKeyFormat(resourceName, attrName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		publicKey := rs.Primary.Attributes[attrName]
		if publicKey == "" {
			return fmt.Errorf("Public key attribute %s is empty", attrName)
		}

		// Check if it's a valid NATS operator public key (starts with 'O')
		if !regexp.MustCompile(`^O[A-Z0-9]{55}$`).MatchString(publicKey) {
			return fmt.Errorf("Invalid operator public key format: %s", publicKey)
		}

		return nil
	}
}

func testAccCheckOperatorSeedFormat(resourceName, attrName string) resource.TestCheckFunc {
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

		// Verify it's an operator seed by checking the seed prefix (SO)
		if !regexp.MustCompile(`^SO[A-Z0-9]{54,}$`).MatchString(seed) {
			return fmt.Errorf("Not an operator seed: %s", seed)
		}

		// Verify the keypair can derive a valid operator public key
		pubKey, err := kp.PublicKey()
		if err != nil {
			return fmt.Errorf("Failed to get public key from seed: %v", err)
		}
		if !regexp.MustCompile(`^O[A-Z0-9]{55}$`).MatchString(pubKey) {
			return fmt.Errorf("Seed does not produce a valid operator public key: %s", pubKey)
		}

		return nil
	}
}

func testAccOperatorImportStateIdFunc(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return "", fmt.Errorf("Resource not found: %s", resourceName)
		}

		name := rs.Primary.Attributes["name"]
		seed := rs.Primary.Attributes["seed"]

		if name == "" || seed == "" {
			return "", fmt.Errorf("Name or seed not found in state")
		}

		// Format: name/seed
		return fmt.Sprintf("%s/%s", name, seed), nil
	}
}