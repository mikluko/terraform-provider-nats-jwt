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
					resource.TestCheckResourceAttr("nsc_operator.test", "name", "TestOperator"),
					resource.TestCheckResourceAttrSet("nsc_operator.test", "jwt"),
					resource.TestCheckResourceAttrSet("nsc_operator.test", "subject"),
					resource.TestCheckResourceAttrSet("nsc_operator.test", "issuer_seed"),
					resource.TestCheckResourceAttrSet("nsc_operator.test", "public_key"),
					testAccCheckOperatorPublicKeyFormat("nsc_operator.test", "public_key"),
					testAccCheckOperatorPublicKeyFormat("nsc_operator.test", "subject"),
				),
			},
			// Update and Read testing
			{
				Config: testAccOperatorResourceConfig("UpdatedOperator"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_operator.test", "name", "UpdatedOperator"),
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
					resource.TestCheckResourceAttr("nsc_operator.test", "name", "TestOperator"),
					resource.TestCheckResourceAttr("nsc_operator.test", "signing_keys.#", "1"),
					resource.TestCheckResourceAttrSet("nsc_operator.test", "signing_keys.0"),
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
					resource.TestCheckResourceAttr("nsc_operator.test", "name", "TestOperator"),
					resource.TestCheckResourceAttr("nsc_operator.test", "expires_in", "720h"),
					resource.TestCheckResourceAttr("nsc_operator.test", "starts_in", "24h"),
				),
			},
			// Update expiry
			{
				Config: testAccOperatorResourceConfigWithExpiry("TestOperator", "1440h", "48h"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_operator.test", "expires_in", "1440h"),
					resource.TestCheckResourceAttr("nsc_operator.test", "starts_in", "48h"),
				),
			},
		},
	})
}

func testAccOperatorResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_operator" "test" {
  name        = %[1]q
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}
`, name)
}

func testAccOperatorResourceConfigWithSigningKey(name string) string {
	return fmt.Sprintf(`
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "signing_key" {
  type = "operator"
}

resource "nsc_operator" "test" {
  name         = %[1]q
  subject      = nsc_nkey.operator.public_key
  issuer_seed  = nsc_nkey.operator.seed
  signing_keys = [nsc_nkey.signing_key.public_key]
}
`, name)
}

func testAccOperatorResourceConfigWithExpiry(name, expiry, start string) string {
	return fmt.Sprintf(`
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_operator" "test" {
  name        = %[1]q
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
  expires_in  = %[2]q
  starts_in   = %[3]q
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
