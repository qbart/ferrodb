.PHONY: test.clickhouse
test.clickhouse:
	CGO_ENABLED=0 TEST_DRIVER=clickhouse go test ./...

.PHONY: test.mariadb
test.mariadb:
	CGO_ENABLED=0 TEST_DRIVER=mariadb go test ./...

.PHONY: test.mysql
test.mysql:
	CGO_ENABLED=0 TEST_DRIVER=mysql go test ./...

.PHONY: test.sqlite
test.sqlite:
	CGO_ENABLED=0 TEST_DRIVER=sqlite go test ./...

.PHONY: test.pg
test.pg:
	CGO_ENABLED=0 TEST_DRIVER=postgresql go test ./...

.PHONY: ui
ui:
	CGO_ENABLED=0 go run main.go ui --raw "mysql:root:test@tcp(localhost:3307)/test"

.PHONY: build
build:
	@mkdir -p bin/
	@CGO_ENABLED=0 go build -o bin/ferro main.go

.PHONY: install
install: build
	@cp bin/ferro ${HOME}/bin/ferro || echo "Failed to copy to ~/bin"

.PHONY: changelog
changelog:
	git-chglog -o CHANGELOG.md --next-tag ${TAG}

.PHONY: ai
ai:
	claude --resume 91366a35-3943-4c68-9cd2-e87d22814819
