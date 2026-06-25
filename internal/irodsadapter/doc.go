// Package irodsadapter is the only direct iRODS boundary for this service and
// its local command-line workflows.
//
// The adapter owns go-irodsclient connection usage and is the integration point
// for shared reconciliation helpers from go-irodsclient-extensions/usersync.
// Higher layers should depend on interfaces in this package rather than using
// go-irodsclient, usersync, or REST-specific clients directly.
//
// This package must not call irods-go-rest. Generic irods-go-rest endpoints are
// reserved for external HTTP clients, while local CLIs should use the shared
// direct go-irodsclient library calls.
package irodsadapter
