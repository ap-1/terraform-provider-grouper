.PHONY: build install lint fmt generate test testacc grouper-up grouper-down

COMPOSE ?= docker compose
GROUPER_COMPOSE = $(COMPOSE) -f testdata/grouper/compose.yml

build:
	go build ./...

install:
	go install .

lint:
	golangci-lint run

fmt:
	gofmt -s -w .

generate:
	cd tools && go generate ./...

test:
	go test -cover ./...

grouper-up:
	$(GROUPER_COMPOSE) up -d --wait
	$(GROUPER_COMPOSE) exec -T postgres psql -U postgres -v ON_ERROR_STOP=1 -q < testdata/grouper/seed.sql

grouper-down:
	$(GROUPER_COMPOSE) down -v

testacc: grouper-up
	TF_ACC=1 \
	TF_ACC_TERRAFORM_PATH="$$(command -v tofu)" \
	TF_ACC_PROVIDER_HOST=registry.opentofu.org \
	TF_ACC_PROVIDER_NAMESPACE=ap-1 \
	GROUPER_URL=http://localhost:8080/grouper/servicesRest/v2_6_000 \
	GROUPER_USERNAME=GrouperSystem \
	GROUPER_PASSWORD=grouper \
	go test -v -cover -timeout 120m ./...
