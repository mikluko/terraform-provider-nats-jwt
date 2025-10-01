package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNKeyResource_operator(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNKeyResourceConfig("operator"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "id"),
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "public_key"),
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "seed"),
					resource.TestCheckResourceAttr("nsc_nkey.test", "type", "operator"),
					testAccCheckNKeyPublicKeyPrefix("nsc_nkey.test", "O"),
					testAccCheckNKeySeedPrefix("nsc_nkey.test", "SO"),
				),
			},
			// ImportState testing
			{
				ResourceName: "nsc_nkey.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["nsc_nkey.test"].Primary.Attributes["seed"], nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNKeyResource_account(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNKeyResourceConfig("account"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "id"),
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "public_key"),
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "seed"),
					resource.TestCheckResourceAttr("nsc_nkey.test", "type", "account"),
					testAccCheckNKeyPublicKeyPrefix("nsc_nkey.test", "A"),
					testAccCheckNKeySeedPrefix("nsc_nkey.test", "SA"),
				),
			},
			// ImportState testing
			{
				ResourceName: "nsc_nkey.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["nsc_nkey.test"].Primary.Attributes["seed"], nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNKeyResource_user(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccNKeyResourceConfig("user"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "id"),
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "public_key"),
					resource.TestCheckResourceAttrSet("nsc_nkey.test", "seed"),
					resource.TestCheckResourceAttr("nsc_nkey.test", "type", "user"),
					testAccCheckNKeyPublicKeyPrefix("nsc_nkey.test", "U"),
					testAccCheckNKeySeedPrefix("nsc_nkey.test", "SU"),
				),
			},
			// ImportState testing
			{
				ResourceName: "nsc_nkey.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["nsc_nkey.test"].Primary.Attributes["seed"], nil
				},
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNKeyResource_importWithType(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a resource first
			{
				Config: testAccNKeyResourceConfig("account"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_nkey.test", "type", "account"),
					testAccCheckNKeyPublicKeyPrefix("nsc_nkey.test", "A"),
				),
			},
			// Import with seed - type should be auto-detected
			{
				ResourceName:      "nsc_nkey.test",
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["nsc_nkey.test"].Primary.Attributes["seed"], nil
				},
			},
		},
	})
}

func testAccNKeyResourceConfig(keyType string) string {
	return fmt.Sprintf(`
resource "nsc_nkey" "test" {
  type = %[1]q
}
`, keyType)
}

func testAccCheckNKeyPublicKeyPrefix(resourceName, expectedPrefix string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		publicKey := rs.Primary.Attributes["public_key"]
		if publicKey == "" {
			return fmt.Errorf("public_key is empty")
		}

		if publicKey[:1] != expectedPrefix {
			return fmt.Errorf("public_key has wrong prefix: expected %s, got %s", expectedPrefix, publicKey[:1])
		}

		return nil
	}
}

func testAccCheckNKeySeedPrefix(resourceName, expectedPrefix string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Resource not found: %s", resourceName)
		}

		seed := rs.Primary.Attributes["seed"]
		if seed == "" {
			return fmt.Errorf("seed is empty")
		}

		if seed[:2] != expectedPrefix {
			return fmt.Errorf("seed has wrong prefix: expected %s, got %s", expectedPrefix, seed[:2])
		}

		return nil
	}
}
