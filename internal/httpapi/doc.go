// Package httpapi exposes the private iRODS-Keycloak admin control-plane API.
//
// The package owns HTTP routing, request decoding, response encoding, and
// narrow authentication checks for service callbacks. Workflow decisions,
// iRODS mutation, Keycloak lookup, audit, retry, and repair behavior belong in
// service packages below this boundary.
package httpapi
