// Command terraform-provider-tebibcloud is the TebibCloud provider for Terraform and OpenTofu.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/tebibcloud/terraform-provider-tebibcloud/internal/provider"
)

// Generate the provider docs (docs/) from the schema and examples/.
//go:generate go tool tfplugindocs generate --provider-name tebibcloud

// version is set by goreleaser.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/tebibcloud/tebibcloud",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
