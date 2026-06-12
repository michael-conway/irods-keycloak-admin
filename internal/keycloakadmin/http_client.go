package keycloakadmin

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAdminRealm = "master"
	defaultClientID   = "admin-cli"
	groupPageSize     = 100
)

type HTTPClientConfig struct {
	BaseURL            string
	AdminRealm         string
	ClientID           string
	ClientSecret       string
	Username           string
	Password           string
	InsecureSkipVerify bool
	HTTPClient         *http.Client
}

// HTTPClient is a Keycloak Admin REST client for sync planning and controlled
// mirror repair apply workflows.
type HTTPClient struct {
	baseURL      *url.URL
	adminRealm   string
	clientID     string
	clientSecret string
	username     string
	password     string
	httpClient   *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

var _ Client = (*HTTPClient)(nil)

func NewHTTPClient(cfg HTTPClientConfig) (*HTTPClient, error) {
	baseURLText := strings.TrimSpace(cfg.BaseURL)
	if baseURLText == "" {
		return nil, errors.New("keycloak base url is required")
	}
	baseURL, err := url.Parse(baseURLText)
	if err != nil {
		return nil, fmt.Errorf("parse keycloak base url: %w", err)
	}
	if baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("keycloak base url must be absolute: %q", baseURLText)
	}

	adminRealm := strings.TrimSpace(cfg.AdminRealm)
	if adminRealm == "" {
		adminRealm = defaultAdminRealm
	}
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		clientID = defaultClientID
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		if cfg.InsecureSkipVerify {
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		}
		httpClient = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
	}

	return &HTTPClient{
		baseURL:      baseURL,
		adminRealm:   adminRealm,
		clientID:     clientID,
		clientSecret: strings.TrimSpace(cfg.ClientSecret),
		username:     strings.TrimSpace(cfg.Username),
		password:     cfg.Password,
		httpClient:   httpClient,
	}, nil
}

func (c *HTTPClient) GetUser(ctx context.Context, realm string, userID string) (*User, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("keycloak user id is required")
	}
	var user User
	if err := c.doJSON(ctx, http.MethodGet, c.adminPath(realm, "users", userID), nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *HTTPClient) FindUserByUsername(ctx context.Context, realm string, username string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("keycloak username is required")
	}
	values := url.Values{}
	values.Set("username", username)
	values.Set("exact", "true")
	var users []User
	if err := c.doJSON(ctx, http.MethodGet, c.adminPath(realm, "users")+"?"+values.Encode(), nil, &users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &users[0], nil
}

func (c *HTTPClient) ListGroups(ctx context.Context, realm string) ([]Group, error) {
	result := []Group{}
	seen := map[string]struct{}{}
	for first := 0; ; first += groupPageSize {
		values := url.Values{}
		values.Set("briefRepresentation", "false")
		values.Set("first", strconv.Itoa(first))
		values.Set("max", strconv.Itoa(groupPageSize))
		var groups []groupResponse
		if err := c.doJSON(ctx, http.MethodGet, c.adminPath(realm, "groups")+"?"+values.Encode(), nil, &groups); err != nil {
			return nil, err
		}
		for _, group := range groups {
			var err error
			result, err = c.appendGroupHierarchy(ctx, realm, result, seen, group)
			if err != nil {
				return nil, err
			}
		}
		if len(groups) < groupPageSize {
			break
		}
	}
	return result, nil
}

func (c *HTTPClient) ListGroupMembers(ctx context.Context, realm string, groupID string) ([]User, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, errors.New("keycloak group id is required")
	}
	if strings.HasPrefix(groupID, "/") {
		resolved, err := c.findGroupIDByPath(ctx, realm, groupID)
		if err != nil {
			return nil, err
		}
		groupID = resolved
	}
	values := url.Values{}
	values.Set("briefRepresentation", "false")
	var users []User
	if err := c.doJSON(ctx, http.MethodGet, c.adminPath(realm, "groups", groupID, "members")+"?"+values.Encode(), nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *HTTPClient) CreateOrUpdateUser(ctx context.Context, realm string, user User) (*User, error) {
	user.Username = strings.TrimSpace(user.Username)
	user.ID = strings.TrimSpace(user.ID)
	if user.Username == "" && user.ID == "" {
		return nil, errors.New("keycloak user username or id is required")
	}

	if user.ID == "" && user.Username != "" {
		existing, err := c.FindUserByUsername(ctx, realm, user.Username)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			user.ID = existing.ID
			if user.Attributes == nil {
				user.Attributes = existing.Attributes
			}
			if user.Email == "" {
				user.Email = existing.Email
			}
		}
	}

	if user.ID != "" {
		if user.Username == "" {
			existing, err := c.GetUser(ctx, realm, user.ID)
			if err != nil {
				return nil, err
			}
			if existing != nil {
				user.Username = existing.Username
				if user.Attributes == nil {
					user.Attributes = existing.Attributes
				}
				if user.Email == "" {
					user.Email = existing.Email
				}
			}
		}
		if err := c.doJSON(ctx, http.MethodPut, c.adminPath(realm, "users", user.ID), user, nil); err != nil {
			return nil, err
		}
		return c.GetUser(ctx, realm, user.ID)
	}

	if err := c.doJSON(ctx, http.MethodPost, c.adminPath(realm, "users"), user, nil); err != nil {
		if !isKeycloakStatus(err, http.StatusConflict) {
			return nil, err
		}
	}
	created, err := c.FindUserByUsername(ctx, realm, user.Username)
	if err != nil {
		return nil, err
	}
	if created != nil {
		return created, nil
	}
	return &user, nil
}

func (c *HTTPClient) CreateOrUpdateGroup(ctx context.Context, realm string, group Group) (*Group, MutationOutcome, error) {
	groupPath, err := normalizeGroupPath(group)
	if err != nil {
		return nil, "", err
	}
	groupName := strings.TrimSpace(group.Name)
	if groupName == "" {
		groupName = groupNameFromPath(groupPath)
	}

	existing, err := c.findGroupByPath(ctx, realm, groupPath)
	if err != nil {
		return nil, "", err
	}
	if existing != nil {
		attributes := group.Attributes
		if attributes == nil {
			attributes = existing.Attributes
		}
		if groupMatches(*existing, groupPath, groupName, attributes) {
			return existing, MutationOutcomeUnchanged, nil
		}
		body := Group{
			ID:         existing.ID,
			Path:       groupPath,
			Name:       groupName,
			Attributes: attributes,
		}
		if err := c.doJSON(ctx, http.MethodPut, c.adminPath(realm, "groups", existing.ID), body, nil); err != nil {
			return nil, "", err
		}
		updated, err := c.findGroupByPath(ctx, realm, groupPath)
		return updated, MutationOutcomeUpdated, err
	}

	parentPath := parentGroupPath(groupPath)
	var createPath string
	if parentPath == "" {
		createPath = c.adminPath(realm, "groups")
	} else {
		parent, err := c.ensureGroupPath(ctx, realm, parentPath)
		if err != nil {
			return nil, "", err
		}
		createPath = c.adminPath(realm, "groups", parent.ID, "children")
	}

	body := Group{
		Name:       groupName,
		Attributes: group.Attributes,
	}
	if err := c.doJSON(ctx, http.MethodPost, createPath, body, nil); err != nil {
		if !isKeycloakStatus(err, http.StatusConflict) {
			return nil, "", err
		}
	}
	created, err := c.findGroupByPath(ctx, realm, groupPath)
	if err != nil {
		return nil, "", err
	}
	if created == nil {
		return nil, "", fmt.Errorf("keycloak group %q was not found after create", groupPath)
	}
	return created, MutationOutcomeCreated, nil
}

func (c *HTTPClient) DeleteGroup(ctx context.Context, realm string, groupIDOrPath string) (MutationOutcome, error) {
	groupIDOrPath = strings.TrimSpace(groupIDOrPath)
	if groupIDOrPath == "" {
		return "", errors.New("keycloak group id or path is required")
	}

	groupID := groupIDOrPath
	if strings.HasPrefix(groupIDOrPath, "/") {
		group, err := c.findGroupByPath(ctx, realm, groupIDOrPath)
		if err != nil {
			return "", err
		}
		if group == nil {
			return MutationOutcomeUnchanged, nil
		}
		groupID = group.ID
	}

	if err := c.doJSON(ctx, http.MethodDelete, c.adminPath(realm, "groups", groupID), nil, nil); err != nil {
		if isKeycloakStatus(err, http.StatusNotFound) {
			return MutationOutcomeUnchanged, nil
		}
		return "", err
	}
	return MutationOutcomeDeleted, nil
}

func (c *HTTPClient) AddUserToGroup(ctx context.Context, realm string, userIDOrUsername string, groupIDOrPath string) (MutationOutcome, error) {
	groupID, err := c.resolveGroupID(ctx, realm, groupIDOrPath)
	if err != nil {
		return "", err
	}
	userID, user, err := c.resolveUserID(ctx, realm, userIDOrUsername)
	if err != nil {
		return "", err
	}
	if userID == "" {
		return "", &UserNotFoundError{Realm: realm, Ref: strings.TrimSpace(userIDOrUsername)}
	}
	members, err := c.ListGroupMembers(ctx, realm, groupID)
	if err != nil {
		if isKeycloakStatus(err, http.StatusNotFound) {
			return "", &GroupNotFoundError{Realm: realm, Ref: strings.TrimSpace(groupIDOrPath)}
		}
		return "", err
	}
	if memberMatches(members, userID, userIDOrUsername, user) {
		return MutationOutcomeUnchanged, nil
	}
	if err := c.doJSON(ctx, http.MethodPut, c.adminPath(realm, "users", userID, "groups", groupID), nil, nil); err != nil {
		return "", err
	}
	return MutationOutcomeUpdated, nil
}

func (c *HTTPClient) RemoveUserFromGroup(ctx context.Context, realm string, userIDOrUsername string, groupIDOrPath string) (MutationOutcome, error) {
	groupID, err := c.resolveExistingGroupID(ctx, realm, groupIDOrPath)
	if err != nil {
		return "", err
	}
	if groupID == "" {
		return MutationOutcomeUnchanged, nil
	}

	userID, user, err := c.resolveUserID(ctx, realm, userIDOrUsername)
	if err != nil {
		return "", err
	}
	members, err := c.ListGroupMembers(ctx, realm, groupID)
	if err != nil {
		if isKeycloakStatus(err, http.StatusNotFound) {
			return MutationOutcomeUnchanged, nil
		}
		return "", err
	}
	member := matchingMember(members, userID, userIDOrUsername, user)
	if member == nil {
		return MutationOutcomeUnchanged, nil
	}
	if userID == "" {
		userID = member.ID
	}
	if err := c.doJSON(ctx, http.MethodDelete, c.adminPath(realm, "users", userID, "groups", groupID), nil, nil); err != nil {
		if isKeycloakStatus(err, http.StatusNotFound) {
			return MutationOutcomeUnchanged, nil
		}
		return "", err
	}
	return MutationOutcomeUpdated, nil
}

func (c *HTTPClient) findGroupIDByPath(ctx context.Context, realm string, groupPath string) (string, error) {
	group, err := c.findGroupByPath(ctx, realm, groupPath)
	if err != nil {
		return "", err
	}
	if group == nil {
		return "", fmt.Errorf("keycloak group path %q not found", groupPath)
	}
	return group.ID, nil
}

func (c *HTTPClient) findGroupByPath(ctx context.Context, realm string, groupPath string) (*Group, error) {
	groupPath = normalizeAbsolutePath(groupPath)
	groups, err := c.ListGroups(ctx, realm)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		if normalizeAbsolutePath(group.Path) == groupPath {
			groupCopy := group
			return &groupCopy, nil
		}
	}
	return nil, nil
}

func (c *HTTPClient) ensureGroupPath(ctx context.Context, realm string, groupPath string) (*Group, error) {
	groupPath = normalizeAbsolutePath(groupPath)
	if groupPath == "" || groupPath == "/" {
		return nil, errors.New("keycloak group path is required")
	}
	existing, err := c.findGroupByPath(ctx, realm, groupPath)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	parentPath := parentGroupPath(groupPath)
	var createPath string
	if parentPath == "" {
		createPath = c.adminPath(realm, "groups")
	} else {
		parent, err := c.ensureGroupPath(ctx, realm, parentPath)
		if err != nil {
			return nil, err
		}
		createPath = c.adminPath(realm, "groups", parent.ID, "children")
	}

	body := Group{Name: groupNameFromPath(groupPath)}
	if err := c.doJSON(ctx, http.MethodPost, createPath, body, nil); err != nil {
		if !isKeycloakStatus(err, http.StatusConflict) {
			return nil, err
		}
	}
	created, err := c.findGroupByPath(ctx, realm, groupPath)
	if err != nil {
		return nil, err
	}
	if created == nil {
		return nil, fmt.Errorf("keycloak group %q was not found after create", groupPath)
	}
	return created, nil
}

func (c *HTTPClient) resolveGroupID(ctx context.Context, realm string, groupIDOrPath string) (string, error) {
	groupID, err := c.resolveExistingGroupID(ctx, realm, groupIDOrPath)
	if err != nil {
		return "", err
	}
	if groupID == "" {
		return "", &GroupNotFoundError{Realm: realm, Ref: strings.TrimSpace(groupIDOrPath)}
	}
	return groupID, nil
}

func (c *HTTPClient) resolveExistingGroupID(ctx context.Context, realm string, groupIDOrPath string) (string, error) {
	groupIDOrPath = strings.TrimSpace(groupIDOrPath)
	if groupIDOrPath == "" {
		return "", errors.New("keycloak group id or path is required")
	}
	if !strings.HasPrefix(groupIDOrPath, "/") {
		return groupIDOrPath, nil
	}
	group, err := c.findGroupByPath(ctx, realm, groupIDOrPath)
	if err != nil {
		return "", err
	}
	if group == nil {
		return "", nil
	}
	return group.ID, nil
}

func (c *HTTPClient) resolveUserID(ctx context.Context, realm string, userIDOrUsername string) (string, *User, error) {
	userIDOrUsername = strings.TrimSpace(userIDOrUsername)
	if userIDOrUsername == "" {
		return "", nil, errors.New("keycloak user id or username is required")
	}
	user, err := c.FindUserByUsername(ctx, realm, userIDOrUsername)
	if err != nil {
		return "", nil, err
	}
	if user != nil {
		return user.ID, user, nil
	}
	user, err = c.GetUser(ctx, realm, userIDOrUsername)
	if err != nil {
		if isKeycloakStatus(err, http.StatusNotFound) {
			return "", nil, nil
		}
		return "", nil, err
	}
	if user != nil {
		return user.ID, user, nil
	}
	return "", nil, nil
}

func groupMatches(existing Group, groupPath string, groupName string, attributes map[string][]string) bool {
	if normalizeAbsolutePath(existing.Path) != normalizeAbsolutePath(groupPath) {
		return false
	}
	if strings.TrimSpace(existing.Name) != strings.TrimSpace(groupName) {
		return false
	}
	return reflect.DeepEqual(existing.Attributes, attributes)
}

func (c *HTTPClient) appendGroupHierarchy(ctx context.Context, realm string, result []Group, seen map[string]struct{}, group groupResponse) ([]Group, error) {
	if group.ID != "" {
		if _, ok := seen[group.ID]; ok {
			return result, nil
		}
		seen[group.ID] = struct{}{}
	}

	result = append(result, Group{
		ID:         group.ID,
		Path:       group.Path,
		Name:       group.Name,
		Attributes: group.Attributes,
	})

	children := group.SubGroups
	if group.SubGroupCount > 0 {
		loadedChildren, err := c.listGroupChildren(ctx, realm, group.ID)
		if err != nil {
			return nil, err
		}
		if len(loadedChildren) > 0 {
			children = loadedChildren
		}
	}
	for _, subgroup := range children {
		var err error
		result, err = c.appendGroupHierarchy(ctx, realm, result, seen, subgroup)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *HTTPClient) listGroupChildren(ctx context.Context, realm string, groupID string) ([]groupResponse, error) {
	if strings.TrimSpace(groupID) == "" {
		return nil, nil
	}

	result := []groupResponse{}
	for first := 0; ; first += groupPageSize {
		values := url.Values{}
		values.Set("briefRepresentation", "false")
		values.Set("first", strconv.Itoa(first))
		values.Set("max", strconv.Itoa(groupPageSize))

		var groups []groupResponse
		if err := c.doJSON(ctx, http.MethodGet, c.adminPath(realm, "groups", groupID, "children")+"?"+values.Encode(), nil, &groups); err != nil {
			return nil, err
		}
		result = append(result, groups...)
		if len(groups) < groupPageSize {
			break
		}
	}
	return result, nil
}

func normalizeGroupPath(group Group) (string, error) {
	if strings.TrimSpace(group.Path) != "" {
		return normalizeAbsolutePath(group.Path), nil
	}
	if strings.TrimSpace(group.Name) != "" {
		return normalizeAbsolutePath("/" + group.Name), nil
	}
	return "", errors.New("keycloak group path or name is required")
}

func normalizeAbsolutePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := pathpkg.Clean(value)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func groupNameFromPath(groupPath string) string {
	groupPath = normalizeAbsolutePath(groupPath)
	if groupPath == "" || groupPath == "/" {
		return ""
	}
	return pathpkg.Base(groupPath)
}

func parentGroupPath(groupPath string) string {
	groupPath = normalizeAbsolutePath(groupPath)
	if groupPath == "" || groupPath == "/" {
		return ""
	}
	parent := pathpkg.Dir(groupPath)
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}

func memberMatches(members []User, userID string, userIDOrUsername string, resolvedUser *User) bool {
	return matchingMember(members, userID, userIDOrUsername, resolvedUser) != nil
}

func matchingMember(members []User, userID string, userIDOrUsername string, resolvedUser *User) *User {
	userID = strings.TrimSpace(userID)
	userIDOrUsername = strings.TrimSpace(userIDOrUsername)
	resolvedUsername := ""
	if resolvedUser != nil {
		resolvedUsername = strings.TrimSpace(resolvedUser.Username)
	}
	for i := range members {
		member := &members[i]
		if userID != "" && strings.TrimSpace(member.ID) == userID {
			return member
		}
		if userIDOrUsername != "" && strings.TrimSpace(member.Username) == userIDOrUsername {
			return member
		}
		if resolvedUsername != "" && strings.TrimSpace(member.Username) == resolvedUsername {
			return member
		}
	}
	return nil
}

func (c *HTTPClient) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	if c == nil {
		return errors.New("keycloak client is required")
	}
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}

	var requestBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		requestBody = bytes.NewReader(payload)
	}

	requestURL := c.resolve(path)
	req, err := http.NewRequestWithContext(ctx, method, requestURL, requestBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return keycloakStatusError(resp, method, requestURL)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *HTTPClient) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		token := c.token
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	values := url.Values{}
	values.Set("client_id", c.clientID)
	if c.clientSecret != "" {
		values.Set("client_secret", c.clientSecret)
	}
	if c.username != "" || c.password != "" {
		if c.username == "" || c.password == "" {
			return "", errors.New("both keycloak admin username and password are required for password grant")
		}
		values.Set("grant_type", "password")
		values.Set("username", c.username)
		values.Set("password", c.password)
	} else if c.clientSecret != "" {
		values.Set("grant_type", "client_credentials")
	} else {
		return "", errors.New("keycloak admin credentials are required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.realmPath(c.adminRealm, "protocol", "openid-connect", "token"), strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", keycloakStatusError(resp, http.MethodPost, c.realmPath(c.adminRealm, "protocol", "openid-connect", "token"))
	}

	var token tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", errors.New("keycloak token response did not include access_token")
	}
	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 60
	}

	c.mu.Lock()
	c.token = token.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(expiresIn) * time.Second)
	c.mu.Unlock()

	return token.AccessToken, nil
}

func (c *HTTPClient) adminPath(realm string, parts ...string) string {
	segments := append([]string{"admin", "realms", realm}, parts...)
	return pathJoin(segments...)
}

func (c *HTTPClient) realmPath(realm string, parts ...string) string {
	segments := append([]string{"realms", realm}, parts...)
	return c.resolve(pathJoin(segments...))
}

func (c *HTTPClient) resolve(path string) string {
	ref, err := url.Parse(path)
	if err != nil {
		ref = &url.URL{Path: path}
	}
	if ref.IsAbs() {
		return ref.String()
	}
	base := *c.baseURL
	basePath := strings.TrimRight(base.Path, "/")
	if basePath != "" && strings.HasPrefix(ref.Path, "/") {
		ref.Path = basePath + ref.Path
	}
	return base.ResolveReference(ref).String()
}

func pathJoin(parts ...string) string {
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	return "/" + strings.Join(escaped, "/")
}

func keycloakStatusError(resp *http.Response, method string, requestURL string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(body))
	return &StatusError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       text,
		Method:     method,
		URL:        requestURL,
	}
}

type StatusError struct {
	StatusCode int
	Status     string
	Body       string
	Method     string
	URL        string
}

func (e *StatusError) Error() string {
	if e == nil {
		return ""
	}
	request := strings.TrimSpace(strings.Join([]string{e.Method, e.URL}, " "))
	if request == "" {
		request = "keycloak admin request"
	} else {
		request = "keycloak admin request " + request
	}
	if e.Body == "" {
		return fmt.Sprintf("%s failed: %s", request, e.Status)
	}
	return fmt.Sprintf("%s failed: %s: %s", request, e.Status, e.Body)
}

func isKeycloakStatus(err error, statusCodes ...int) bool {
	var statusErr *StatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	for _, statusCode := range statusCodes {
		if statusErr.StatusCode == statusCode {
			return true
		}
	}
	return false
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type groupResponse struct {
	ID            string              `json:"id"`
	Path          string              `json:"path"`
	Name          string              `json:"name"`
	Attributes    map[string][]string `json:"attributes,omitempty"`
	SubGroupCount int                 `json:"subGroupCount,omitempty"`
	SubGroups     []groupResponse     `json:"subGroups,omitempty"`
}
