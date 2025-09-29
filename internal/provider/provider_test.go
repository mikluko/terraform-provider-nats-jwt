package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("TF_ACC", "1")
	m.Run()
}

const (
	providerConfig = `
provider "natsjwt" {}
`
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"natsjwt": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	// No special pre-checks needed for this provider
	// since all data is stored in state
}

func TestProvider(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig,
			},
		},
	})
}

// parseCredsContent extracts JWT and seed from the creds file format
func parseCredsContent(creds string) (jwt, seed string, err error) {
	const (
		jwtHeader  = "-----BEGIN NATS USER JWT-----"
		jwtFooter  = "------END NATS USER JWT------"
		seedHeader = "-----BEGIN USER NKEY SEED-----"
		seedFooter = "------END USER NKEY SEED------"
	)

	// Simple parsing - find the JWT content between headers
	lines := []string{}
	inJWT := false
	inSeed := false

	for _, line := range splitLines(creds) {
		if line == jwtHeader {
			inJWT = true
			continue
		}
		if line == jwtFooter {
			inJWT = false
			jwt = joinLines(lines)
			lines = []string{}
			continue
		}
		if line == seedHeader {
			inSeed = true
			continue
		}
		if line == seedFooter {
			inSeed = false
			seed = joinLines(lines)
			lines = []string{}
			continue
		}

		if inJWT || inSeed {
			lines = append(lines, line)
		}
	}

	if jwt == "" || seed == "" {
		return "", "", fmt.Errorf("failed to parse creds: jwt or seed not found")
	}

	// Handle wrapped base64 encoded seed
	seed = joinLines(splitLines(seed))

	return jwt, seed, nil
}

// splitLines splits a string into lines
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// joinLines joins lines into a single string
func joinLines(lines []string) string {
	result := ""
	for _, line := range lines {
		result += line
	}
	return result
}