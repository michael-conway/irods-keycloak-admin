package keycloakadmin

import "fmt"

type GroupNotFoundError struct {
	Realm string
	Ref   string
}

func (e *GroupNotFoundError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("keycloak group %q not found in realm %q", e.Ref, e.Realm)
}

type UserNotFoundError struct {
	Realm string
	Ref   string
}

func (e *UserNotFoundError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("keycloak user %q not found in realm %q", e.Ref, e.Realm)
}
