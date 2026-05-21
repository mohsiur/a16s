VERSION := $(shell grep -o 'AppVersion = "[^"]*"' internal/utils/info.go | cut -d '"' -f 2)

run:
	go run ./cmd/a16s/main.go

test:
	go test -v ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

lint:
	docker run --rm -v "$(PWD):/app" -w /app golangci/golangci-lint:latest golangci-lint run

tag:
	echo "Tagging version $(VERSION)"
	git tag -a $(VERSION) -m "Release $(VERSION)"
	git push origin $(VERSION)


plan:
	cd tests && terraform plan -var="cluster_count=10" -var="service_count=10" -var="task_count=1"

apply:
	cd tests && terraform apply -var="cluster_count=10" -var="service_count=10" -var="task_count=1"

.PHONY: \
	dep \
	install \
	build \
	vet \
	test \
	test-race \
	lint