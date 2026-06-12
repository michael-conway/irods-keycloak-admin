package repair

import (
	"path"
	"strings"
)

const defaultKeycloakGroupRoot = "/irods"

type mirrorPathPolicy struct {
	root string
}

func newMirrorPathPolicy(root string) mirrorPathPolicy {
	root = strings.TrimSpace(root)
	if root == "" {
		root = defaultKeycloakGroupRoot
	}
	if !strings.HasPrefix(root, "/") {
		root = "/" + root
	}
	root = path.Clean(root)
	if root == "." || root == "" {
		root = defaultKeycloakGroupRoot
	}
	return mirrorPathPolicy{root: root}
}

func (p mirrorPathPolicy) Root() string {
	return p.root
}

func (p mirrorPathPolicy) GroupPath(groupName string) string {
	groupName = strings.Trim(strings.TrimSpace(groupName), "/")
	if groupName == "" {
		return p.root
	}
	return path.Clean(p.root + "/" + groupName)
}

func (p mirrorPathPolicy) GroupNameFromPath(groupPath string) string {
	groupPath = normalizeAbsoluteMirrorPath(groupPath)
	prefix := p.root + "/"
	if !strings.HasPrefix(groupPath, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(groupPath, prefix))
}

func (p mirrorPathPolicy) IsManagedPath(groupPath string) bool {
	groupPath = normalizeAbsoluteMirrorPath(groupPath)
	return groupPath == p.root || strings.HasPrefix(groupPath, p.root+"/")
}

func normalizeAbsoluteMirrorPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return ""
	}
	return cleaned
}
