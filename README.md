# Webhook Relay API Go client

## Features

Currently available features:
- [x] Buckets
- [x] Inputs
- [x] Outputs
- [x] Domain reservations
- [x] Tokens
- [x] Functions
- [ ] Function execution logs
- [x] Tunnels
- [x] Regions
- [ ] Webhook Logs

## Installation

You need a working [Go](https://golang.org/) environment. 

```shell
go get github.com/webhookrelay/webhookrelay-go
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
	// Construct a new Webhook Relay API object to perform requests
	api, err := webhookrelay.New(os.Getenv("RELAY_KEY"), os.Getenv("RELAY_SECRET"))
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