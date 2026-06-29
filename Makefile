
test:
	go test -v -failfast `go list ./... | egrep -v /tests/`