.PHONY: quicktest
quicktest:
	CGO_ENABLED=0 go test -v ./...

.PHONY: ui
ui:
	CGO_ENABLED=0 go run main.go ui --raw postgresql:postgres://athena:athena@localhost:5432/athena

.PHONY: build
build:
	@mkdir -p bin/
	@go build -o bin/ferro main.go
	@cp bin/ferro ${HOME}/bin/ferro || echo "Failed to copy to ~/bin"

.PHONY: changelog
changelog:
	git-chglog -o CHANGELOG.md --next-tag ${TAG}

.PHONY: ai
ai:
	claude --resume 91366a35-3943-4c68-9cd2-e87d22814819
