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

// HTTPClient is a minimal Keycloak Admin REST client for sync planning. The
// current repair slice uses read operations only; mutation methods are left
// unimplemented until apply workflows are introduced.
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

func (c *HTTPClient) CreateOrUpdateUser(context.Context, string, User) (*User, error) {
	return nil, errors.New("keycloak user mutation is not implemented")
}

func (c *HTTPClient) CreateOrUpdateGroup(context.Context, string, Group) (*Group, error) {
	return nil, errors.New("keycloak group mutation is not implemented")
}

func (c *HTTPClient) DeleteGroup(context.Context, string, string) error {
	return errors.New("keycloak group deletion is not implemented")
}

func (c *HTTPClient) AddUserToGroup(context.Context, string, string, string) error {
	return errors.New("keycloak group membership mutation is not implemented")
}

func (c *HTTPClient) RemoveUserFromGroup(context.Context, string, string, string) error {
	return errors.New("keycloak group membership mutation is not implemented")
}

func (c *HTTPClient) findGroupIDByPath(ctx context.Context, realm string, groupPath string) (string, error) {
	groups, err := c.ListGroups(ctx, realm)
	if err != nil {
		return "", err
	}
	for _, group := range groups {
		if group.Path == groupPath {
			return group.ID, nil
		}
	}
	return "", fmt.Errorf("keycloak group path %q not found", groupPath)
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

	children, err := c.listGroupChildren(ctx, realm, group.ID)
	if err != nil {
		return nil, err
	}
	if len(children) == 0 {
		children = group.SubGroups
	}
	for _, subgroup := range children {
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
		return keycloakStatusError(resp)
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
		return "", keycloakStatusError(resp)
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

func keycloakStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("keycloak admin request failed: %s", resp.Status)
	}
	return fmt.Errorf("keycloak admin request failed: %s: %s", resp.Status, text)
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type groupResponse struct {
	ID         string              `json:"id"`
	Path       string              `json:"path"`
	Name       string              `json:"name"`
	Attributes map[string][]string `json:"attributes,omitempty"`
	SubGroups  []groupResponse     `json:"subGroups,omitempty"`
}
