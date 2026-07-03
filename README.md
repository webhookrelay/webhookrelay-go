# Webhook Relay API Go client

[![GoDoc](https://img.shields.io/badge/godoc-reference-5673AF.svg?style=flat-square)](https://godoc.org/github.com/webhookrelay/webhookrelay-go)

> This library is currently actively developed so the API might change a little bit.

## Installation

You need a working [Go](https://golang.org/) environment. 

```shell
go get github.com/webhookrelay/webhookrelay-go
```

## Authentication

Generate credentials [here](https://my.webhookrelay.com/tokens). There are two ways to authenticate:

- **API key (recommended)** — a single key (looks like `sk-...`) sent as a Bearer token:

  ```golang
  api, err := webhookrelay.NewWithAPIKey(os.Getenv("RELAY_API_KEY"))
  ```

- **Key & secret pair** — sent as HTTP Basic auth:

  ```golang
  api, err := webhookrelay.New(os.Getenv("RELAY_KEY"), os.Getenv("RELAY_SECRET"))
  ```

## Usage

```golang
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/webhookrelay/webhookrelay-go"
)

func main() {
	// Construct a new Webhook Relay API object to perform requests. Use an API
	// key (Bearer token); alternatively use webhookrelay.New(key, secret).
	api, err := webhookrelay.NewWithAPIKey(os.Getenv("RELAY_API_KEY"))
	if err != nil {
		log.Fatal(err)
  }
  
  bucket, err := api.CreateBucket(&webhookrelay.BucketCreateOptions{
    Name: "sendgrid-to-segment",
  })
  if err != nil {
		log.Fatal(err)
  }
  // all buckets get a default input that you can use to receive webhooks, 
  // it can either be used with custom domain + path prefix (https://xxx.hooks.webhookrelay.com) 
  // or input ID such as https://my.webhookrelay.com/v1/webhooks/xxx
  fmt.Println(bucket.Inputs[0].EndpointURL()) 

  // Create a webhook forwarding destination for this webhook
  _, err = api.CreateOutput(&webhookrelay.Output{
    BucketID: bucket.ID,
    Name: "segment",
    Destination: "https://webhooks.segment.com?b=yyyy",
  })
  if err != nil {
		log.Fatal(err)
  }

  // list all buckets
  buckets, err := api.ListBuckets(&webhookrelay.BucketListOptions{})
  if err != nil {
		log.Fatal(err)
  }
  fmt.Println(buckets) // print buckets
}
```