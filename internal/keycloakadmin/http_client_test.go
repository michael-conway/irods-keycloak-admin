package keycloakadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientListsFlattenedGroupsAndMembers(t *testing.T) {
	var tokenRequests int
	var membersRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/realms/master/protocol/openid-connect/token":
			tokenRequests++
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token request form: %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "password" {
				t.Fatalf("unexpected grant_type: %q", got)
			}
			if got := r.Form.Get("client_id"); got != "admin-cli" {
				t.Fatalf("unexpected client_id: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "token",
				"expires_in":   300,
			})
		case "/admin/realms/irods/groups":
			if got := r.Header.Get("Authorization"); got != "Bearer token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			if got := r.URL.Query().Get("briefRepresentation"); got != "false" {
				t.Fatalf("unexpected briefRepresentation: %q", got)
			}
			if got := r.URL.Query().Get("first"); got != "0" {
				t.Fatalf("unexpected first: %q", got)
			}
			if got := r.URL.Query().Get("max"); got != "100" {
				t.Fatalf("unexpected max: %q", got)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":            "root-id",
					"name":          "irods",
					"path":          "/irods",
					"subGroupCount": 1,
					"subGroups":     []map[string]any{},
				},
			})
		case "/admin/realms/irods/groups/root-id/children":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"id":   "project-id",
					"name": "project-alpha",
					"path": "/irods/project-alpha",
					"attributes": map[string][]string{
						"irods_group_name": {"project-alpha"},
						"authority":        {"irods"},
					},
				},
			})
		case "/admin/realms/irods/groups/project-id/children":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/admin/realms/irods/groups/project-id/members":
			membersRequests++
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "user-id", "username": "alice"},
			})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientConfig{
		BaseURL:  server.URL,
		Username: "admin",
		Password: "admin",
	})
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}

	groups, err := client.ListGroups(context.Background(), "irods")
	if err != nil {
		t.Fatalf("ListGroups returned error: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected flattened root and child groups, got %+v", groups)
	}
	if groups[1].ID != "project-id" || groups[1].Path != "/irods/project-alpha" {
		t.Fatalf("unexpected flattened child group: %+v", groups[1])
	}

	members, err := client.ListGroupMembers(context.Background(), "irods", "project-id")
	if err != nil {
		t.Fatalf("ListGroupMembers returned error: %v", err)
	}
	if len(members) != 1 || members[0].Username != "alice" {
		t.Fatalf("unexpected members: %+v", members)
	}
	if tokenRequests != 1 {
		t.Fatalf("expected cached token to be reused, token requests: %d", tokenRequests)
	}
	if membersRequests != 1 {
		t.Fatalf("expected one members request, got %d", membersRequests)
	}
}

func TestHTTPClientResolvePreservesBasePath(t *testing.T) {
	client, err := NewHTTPClient(HTTPClientConfig{
		BaseURL:  "https://keycloak.example/auth",
		Username: "admin",
		Password: "admin",
	})
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}

	got := client.resolve("/realms/master/protocol/openid-connect/token?x=y")
	want := "https://keycloak.example/auth/realms/master/protocol/openid-connect/token?x=y"
	if got != want {
		t.Fatalf("unexpected resolved URL:\nwant %s\ngot  %s", want, got)
	}
}
