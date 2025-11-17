
# generate proto file for Functions module
proto:
	go get github.com/mitchellh/protoc-gen-go-json
	protoc --gofast_out=. --go-json_out=. api/reactor/v1/reactor.proto

test:
	go test -v -failfast `go list ./... | egrep -v /tests/` 