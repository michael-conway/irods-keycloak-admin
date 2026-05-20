package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "plan", "apply", "bootstrap-keycloak", "repair-keycloak":
		_, _ = fmt.Fprintf(os.Stderr, "irods-kc-sync %s is scaffolded but not implemented yet\n", os.Args[1])
		os.Exit(1)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: irods-kc-sync {plan|apply|bootstrap-keycloak|repair-keycloak}")
}
