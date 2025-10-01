package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCredsDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccCredsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.nsc_creds.test", "id"),
					resource.TestCheckResourceAttrSet("data.nsc_creds.test", "creds"),
					resource.TestMatchResourceAttr("data.nsc_creds.test", "creds", regexp.MustCompile(`-----BEGIN NATS USER JWT-----`)),
					resource.TestMatchResourceAttr("data.nsc_creds.test", "creds", regexp.MustCompile(`-----BEGIN USER NKEY SEED-----`)),
					resource.TestMatchResourceAttr("data.nsc_creds.test", "creds", regexp.MustCompile(`------END NATS USER JWT------`)),
					resource.TestMatchResourceAttr("data.nsc_creds.test", "creds", regexp.MustCompile(`------END USER NKEY SEED------`)),
				),
			},
		},
	})
}

func testAccCredsDataSourceConfig() string {
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
}

data "nsc_creds" "test" {
  jwt  = nsc_user.test.jwt
  seed = nsc_nkey.user.seed
}
`
}
