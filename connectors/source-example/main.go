package main

import (
	"log"
	"os"

	airbyte "github.com/saturn4er/airbyte-go-sdk"
)

func main() {
	runner := airbyte.NewSourceRunner(NewExampleSource(), os.Stdout)

	if err := runner.Start(); err != nil {
		log.Fatal(err)
	}
}
