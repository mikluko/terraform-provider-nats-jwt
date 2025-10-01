package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/nats-io/nkeys"
)

func TestAccAccountKeyResource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccAccountKeyResourceConfig("test-account"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nsc_account_key.test", "id"),
					resource.TestCheckResourceAttrSet("nsc_account_key.test", "public_key"),
					resource.TestCheckResourceAttrSet("nsc_account_key.test", "seed"),
					resource.TestCheckResourceAttr("nsc_account_key.test", "name", "test-account"),
				),
			},
			// ImportState testing - import using the seed
			{
				ResourceName: "nsc_account_key.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources["nsc_account_key.test"].Primary.Attributes["seed"], nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"name"}, // Name defaults to "imported-account" on import
			},
			// Update and Read testing
			{
				Config: testAccAccountKeyResourceConfig("test-account-updated"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("nsc_account_key.test", "name", "test-account-updated"),
				),
			},
		},
	})
}

func TestAccAccountKeyResource_withProvidedSeed(t *testing.T) {
	// Generate a valid seed dynamically
	kp, err := nkeys.CreateAccount()
	if err != nil {
		t.Fatalf("Failed to create test account key: %v", err)
	}
	testSeed, err := kp.Seed()
	if err != nil {
		t.Fatalf("Failed to get test seed: %v", err)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with provided seed
			{
				Config: testAccAccountKeyResourceConfigWithSeed("imported-account", string(testSeed)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("nsc_account_key.test", "id"),
					resource.TestCheckResourceAttrSet("nsc_account_key.test", "public_key"),
					resource.TestCheckResourceAttr("nsc_account_key.test", "seed", string(testSeed)),
					resource.TestCheckResourceAttr("nsc_account_key.test", "name", "imported-account"),
				),
			},
		},
	})
}

func testAccAccountKeyResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "nsc_account_key" "test" {
  name = %[1]q
}
`, name)
}

func testAccAccountKeyResourceConfigWithSeed(name string, seed string) string {
	return fmt.Sprintf(`
resource "nsc_account_key" "test" {
  name = %[1]q
  seed = %[2]q
}
`, name, seed)
}
