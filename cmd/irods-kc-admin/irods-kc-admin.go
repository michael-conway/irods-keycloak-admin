package main

import (
	"fmt"
	"os"
)

func main() {
	_, _ = fmt.Fprintln(os.Stderr, "irods-kc-admin is reserved for optional diagnostics and is not implemented yet")
	os.Exit(1)
}
