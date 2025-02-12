package main

import (
	"log"
	"os"
	"strconv"

	"github.com/sourcegraph/sourcegraph/cmd/repo-updater/shared"
)

func main() {
	debug, _ := strconv.ParseBool(os.Getenv("DEBUG"))
	if debug {
		log.Println("enterprise edition")
	}
	shared.Main()
}
