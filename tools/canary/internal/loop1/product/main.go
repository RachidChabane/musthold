package product

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit statuses of Main.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

const (
	flagSetName    = "loop1-product"
	filePermission = 0o644
)

// Main stages the plugin trees into the product directory, digests what the
// directory holds and writes its manifest. The binaries must already be in
// bin/. It returns 0 on success, 1 when staging or writing fails, and 2 on a
// usage error.
func Main(_ context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(flagSetName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	var (
		productDir = flags.String("product", study.DefaultProduct(), "the product directory")
		repository = flags.String("source", "", "the staged tree of the repository at the built commit")
		shallnot   = flags.String("shallnot-tree", "", "the tree of the pinned Shallnot commit")
		manifest   = Manifest{Arch: runtime.GOARCH}
	)
	flags.StringVar(&manifest.Commit, "commit", "", "the commit the product was built from")
	flags.BoolVar(&manifest.Dirty, "dirty", false, "the working tree differed from the commit")
	flags.StringVar(&manifest.ShallnotPin, "shallnot-pin", "", "the commit of Shallnot")
	flags.StringVar(&manifest.Images.Build, "build-image", "", "the Go image, by digest")
	flags.StringVar(&manifest.Images.Base, "base-image", "", "the Node.js image, by digest")
	flags.StringVar(&manifest.Images.UV, "uv-image", "", "the uv image, by digest")
	flags.StringVar(&manifest.Images.Built, "built-image", "", "the id of the built session image")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if *repository == "" || *shallnot == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: -source DIR -shallnot-tree DIR [-product DIR] -commit SHA [-dirty] -shallnot-pin SHA -build-image REF -base-image REF -uv-image REF -built-image ID")
		return exitUsage
	}
	if err := write(*productDir, Sources{Repository: *repository, Shallnot: *shallnot}, manifest); err != nil {
		fmt.Fprintf(stderr, "loop1-product: %v\n", err)
		return exitFailed
	}
	fmt.Fprintf(stdout, "loop1-product: %s\n", filepath.Join(*productDir, ManifestFile))
	return exitOK
}

func write(productDir string, sources Sources, manifest Manifest) error {
	if err := StagePlugins(productDir, sources); err != nil {
		return err
	}
	digests, err := Describe(productDir)
	if err != nil {
		return err
	}
	manifest.Binaries, manifest.Plugins = digests.Binaries, digests.Plugins
	data, err := manifest.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(productDir, ManifestFile), data, filePermission)
}
