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
	groupPath = strings.TrimSpace(groupPath)
	prefix := p.root + "/"
	if !strings.HasPrefix(groupPath, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(groupPath, prefix))
}

func (p mirrorPathPolicy) IsManagedPath(groupPath string) bool {
	groupPath = strings.TrimSpace(groupPath)
	return strings.HasPrefix(groupPath, p.root+"/")
}
