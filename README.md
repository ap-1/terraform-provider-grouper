# Grouper Provider

An OpenTofu provider for [Internet2 Grouper](https://www.internet2.edu/products-services/trust-identity/grouper/) that manages folders, groups, memberships, and privileges through Grouper Web Services.

```hcl
terraform {
  required_providers {
    grouper = {
      source = "ap-1/grouper"
    }
  }
}
```

See the [documentation](docs/) for configuration and resources.

## Building

With [Go](https://go.dev/doc/install) 1.25 or newer:

```shell
go install
```

## Developing

| Command             | Does                                                                      |
| ------------------- | ------------------------------------------------------------------------- |
| `make build`        | builds the provider                                                       |
| `make lint`         | runs golangci-lint                                                        |
| `make generate`     | regenerates `docs/` from the schemas and `examples/`                      |
| `make test`         | runs unit tests                                                           |
| `make testacc`      | starts a local Grouper in Docker and runs the acceptance tests against it |
| `make grouper-down` | stops the local Grouper and deletes its data                              |

With Podman, add `COMPOSE=podman-compose` to the `make` commands.
