// Command loop1-manifest is the step of the loop-1 build that copies the plugin
// trees into the product directory and writes its manifest. The build script runs
// it with `go run`.
package main

import (
	"context"
	"os"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
)

func main() { os.Exit(product.Main(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) }
