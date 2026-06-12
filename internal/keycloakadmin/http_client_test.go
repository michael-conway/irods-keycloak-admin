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

func TestHTTPClientGroupAndMembershipMutationsAreIdempotent(t *testing.T) {
	var groupCreated bool
	var memberPresent bool
	var createGroupCalls int
	var updateGroupCalls int
	var addMemberCalls int
	var removeMemberCalls int
	var deleteGroupCalls int
	var updateUserCalls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/realms/master/protocol/openid-connect/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "token",
				"expires_in":   300,
			})
		case "/admin/realms/irods/groups":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected groups method: %s", r.Method)
			}
			subGroupCount := 0
			if groupCreated {
				subGroupCount = 1
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{
				"id":            "root-id",
				"name":          "irods",
				"path":          "/irods",
				"subGroupCount": subGroupCount,
				"subGroups":     []map[string]any{},
			}})
		case "/admin/realms/irods/groups/root-id/children":
			switch r.Method {
			case http.MethodGet:
				if !groupCreated {
					_ = json.NewEncoder(w).Encode([]map[string]any{})
					return
				}
				_ = json.NewEncoder(w).Encode([]map[string]any{projectAlphaGroupResponse()})
			case http.MethodPost:
				createGroupCalls++
				var body Group
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode create group body: %v", err)
				}
				if body.Name != "project-alpha" || body.Attributes["authority"][0] != "irods" {
					t.Fatalf("unexpected create group body: %+v", body)
				}
				groupCreated = true
				w.WriteHeader(http.StatusCreated)
			default:
				t.Fatalf("unexpected root children method: %s", r.Method)
			}
		case "/admin/realms/irods/groups/project-id":
			switch r.Method {
			case http.MethodPut:
				updateGroupCalls++
				var body Group
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode update group body: %v", err)
				}
				if body.ID != "project-id" || body.Path != "/irods/project-alpha" {
					t.Fatalf("unexpected update group body: %+v", body)
				}
				w.WriteHeader(http.StatusNoContent)
			case http.MethodDelete:
				if !groupCreated {
					http.NotFound(w, r)
					return
				}
				deleteGroupCalls++
				groupCreated = false
				memberPresent = false
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected group method: %s", r.Method)
			}
		case "/admin/realms/irods/groups/project-id/children":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/admin/realms/irods/users":
			if r.Method != http.MethodGet || r.URL.Query().Get("username") != "alice" || r.URL.Query().Get("exact") != "true" {
				t.Fatalf("unexpected users request: %s %s", r.Method, r.URL.String())
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "user-id", "username": "alice"}})
		case "/admin/realms/irods/users/user-id":
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "user-id", "username": "alice", "email": "alice@example.org"})
			case http.MethodPut:
				updateUserCalls++
				var body User
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode update user body: %v", err)
				}
				if body.ID != "user-id" || body.Username != "alice" || body.Email != "alice-updated@example.org" {
					t.Fatalf("unexpected update user body: %+v", body)
				}
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected user method: %s", r.Method)
			}
		case "/admin/realms/irods/groups/project-id/members":
			if !memberPresent {
				_ = json.NewEncoder(w).Encode([]map[string]any{})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": "user-id", "username": "alice"}})
		case "/admin/realms/irods/users/user-id/groups/project-id":
			switch r.Method {
			case http.MethodPut:
				addMemberCalls++
				memberPresent = true
				w.WriteHeader(http.StatusNoContent)
			case http.MethodDelete:
				removeMemberCalls++
				memberPresent = false
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected membership method: %s", r.Method)
			}
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

	if updated, err := client.CreateOrUpdateUser(context.Background(), "irods", User{Username: "alice", Email: "alice-updated@example.org"}); err != nil {
		t.Fatalf("CreateOrUpdateUser returned error: %v", err)
	} else if updated.ID != "user-id" || updated.Username != "alice" {
		t.Fatalf("unexpected updated user: %+v", updated)
	}
	if updateUserCalls != 1 {
		t.Fatalf("expected one user update, got %d", updateUserCalls)
	}

	group := Group{
		Name: "project-alpha",
		Path: "/irods/project-alpha",
		Attributes: map[string][]string{
			"irods_group_name": {"project-alpha"},
			"irods_zone":       {"tempZone"},
			"authority":        {"irods"},
		},
	}
	if created, outcome, err := client.CreateOrUpdateGroup(context.Background(), "irods", group); err != nil {
		t.Fatalf("CreateOrUpdateGroup create returned error: %v", err)
	} else if outcome != MutationOutcomeCreated {
		t.Fatalf("unexpected create outcome: %s", outcome)
	} else if created.ID != "project-id" {
		t.Fatalf("unexpected created group: %+v", created)
	}
	if _, outcome, err := client.CreateOrUpdateGroup(context.Background(), "irods", group); err != nil {
		t.Fatalf("CreateOrUpdateGroup update returned error: %v", err)
	} else if outcome != MutationOutcomeUnchanged {
		t.Fatalf("unexpected update outcome: %s", outcome)
	}
	if createGroupCalls != 1 || updateGroupCalls != 0 {
		t.Fatalf("unexpected group mutation calls: create=%d update=%d", createGroupCalls, updateGroupCalls)
	}

	if outcome, err := client.AddUserToGroup(context.Background(), "irods", "alice", "/irods/project-alpha"); err != nil {
		t.Fatalf("AddUserToGroup returned error: %v", err)
	} else if outcome != MutationOutcomeUpdated {
		t.Fatalf("unexpected add outcome: %s", outcome)
	}
	if outcome, err := client.AddUserToGroup(context.Background(), "irods", "alice", "/irods/project-alpha"); err != nil {
		t.Fatalf("repeat AddUserToGroup returned error: %v", err)
	} else if outcome != MutationOutcomeUnchanged {
		t.Fatalf("unexpected repeat add outcome: %s", outcome)
	}
	if addMemberCalls != 1 {
		t.Fatalf("expected idempotent add member, got %d calls", addMemberCalls)
	}

	if outcome, err := client.RemoveUserFromGroup(context.Background(), "irods", "alice", "/irods/project-alpha"); err != nil {
		t.Fatalf("RemoveUserFromGroup returned error: %v", err)
	} else if outcome != MutationOutcomeUpdated {
		t.Fatalf("unexpected remove outcome: %s", outcome)
	}
	if outcome, err := client.RemoveUserFromGroup(context.Background(), "irods", "alice", "/irods/project-alpha"); err != nil {
		t.Fatalf("repeat RemoveUserFromGroup returned error: %v", err)
	} else if outcome != MutationOutcomeUnchanged {
		t.Fatalf("unexpected repeat remove outcome: %s", outcome)
	}
	if removeMemberCalls != 1 {
		t.Fatalf("expected idempotent remove member, got %d calls", removeMemberCalls)
	}

	if outcome, err := client.DeleteGroup(context.Background(), "irods", "/irods/project-alpha"); err != nil {
		t.Fatalf("DeleteGroup returned error: %v", err)
	} else if outcome != MutationOutcomeDeleted {
		t.Fatalf("unexpected delete outcome: %s", outcome)
	}
	if outcome, err := client.DeleteGroup(context.Background(), "irods", "/irods/project-alpha"); err != nil {
		t.Fatalf("repeat DeleteGroup returned error: %v", err)
	} else if outcome != MutationOutcomeUnchanged {
		t.Fatalf("unexpected repeat delete outcome: %s", outcome)
	}
	if deleteGroupCalls != 1 {
		t.Fatalf("expected idempotent delete group, got %d calls", deleteGroupCalls)
	}
}

func TestHTTPClientResolveUserIDPrefersExactUsernameBeforeIDLookup(t *testing.T) {
	var getUserCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/realms/master/protocol/openid-connect/token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "token",
				"expires_in":   300,
			})
		case "/admin/realms/irods/users":
			if got := r.URL.Query().Get("username"); got != "shared-ref" {
				t.Fatalf("unexpected username query: %q", got)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "user-by-username", "username": "shared-ref"},
			})
		case "/admin/realms/irods/users/shared-ref":
			getUserCalls++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       "shared-ref",
				"username": "different-user",
			})
		case "/admin/realms/irods/groups":
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
			_ = json.NewEncoder(w).Encode([]map[string]any{projectAlphaGroupResponse()})
		case "/admin/realms/irods/groups/project-id/children":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/admin/realms/irods/groups/project-id/members":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case "/admin/realms/irods/users/user-by-username/groups/project-id":
			if r.Method != http.MethodPut {
				t.Fatalf("unexpected membership method: %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
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

	if outcome, err := client.AddUserToGroup(context.Background(), "irods", "shared-ref", "/irods/project-alpha"); err != nil {
		t.Fatalf("AddUserToGroup returned error: %v", err)
	} else if outcome != MutationOutcomeUpdated {
		t.Fatalf("unexpected add outcome: %s", outcome)
	}
	if getUserCalls != 0 {
		t.Fatalf("expected exact username match to win before id lookup, got %d id lookups", getUserCalls)
	}
}

func projectAlphaGroupResponse() map[string]any {
	return map[string]any{
		"id":   "project-id",
		"name": "project-alpha",
		"path": "/irods/project-alpha",
		"attributes": map[string][]string{
			"irods_group_name": {"project-alpha"},
			"irods_zone":       {"tempZone"},
			"authority":        {"irods"},
		},
	}
}
