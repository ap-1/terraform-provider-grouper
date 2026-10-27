package main

import (
	"context"
	"flag"
	"log"

	"github.com/ap-1/terraform-provider-grouper/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Set by GoReleaser at release
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.opentofu.org/ap-1/grouper",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
