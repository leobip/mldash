package api

import (
	"regexp"
	"strings"
)

// rsPodName matches pods created by a ReplicaSet: <workload>-<hash>-<suffix>.
var rsPodName = regexp.MustCompile(`^(.*)-[0-9a-z]{7,10}-([0-9a-z]{5})$`)

// ImageRepo returns an image reference without its registry host and without
// the tag or digest, e.g. "harbor.example.com/aidev/svc:v1.4.1" becomes
// "aidev/svc". The host is identical on every row of a cluster with an
// internal registry, so it is noise in the table; the full reference stays in
// the JSON API and in the row's title attribute.
func ImageRepo(image string) string {
	repo := image
	if i := strings.LastIndex(repo, "@"); i >= 0 {
		repo = repo[:i]
	} else if i := strings.LastIndex(repo, ":"); i > strings.LastIndex(repo, "/") {
		repo = repo[:i] // a ":" before the last "/" is a registry port, not a tag
	}
	if i := strings.Index(repo, "/"); i >= 0 && strings.ContainsAny(repo[:i], ".:") {
		repo = repo[i+1:]
	}
	return repo
}

// ImageTag returns the tag or digest of an image reference, or "" when it has
// neither.
func ImageTag(image string) string {
	if i := strings.LastIndex(image, "@"); i >= 0 {
		return image[i+1:]
	}
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		return image[i+1:]
	}
	return ""
}

// PodBase returns the workload part of a pod name, dropping the ReplicaSet
// hash: "orec-webservice-l4-6b57c886f5-6xcbs" becomes "orec-webservice-l4".
// Names that are not ReplicaSet pods are returned unchanged.
func PodBase(name string) string {
	if m := rsPodName.FindStringSubmatch(name); m != nil {
		return m[1]
	}
	return name
}

// PodSuffix returns the short pod discriminator ("6xcbs") that keeps rows
// distinguishable once the ReplicaSet hash is dropped, or "" for names that are
// not ReplicaSet pods. The full pod name stays in the row's title attribute.
func PodSuffix(name string) string {
	if m := rsPodName.FindStringSubmatch(name); m != nil {
		return m[2]
	}
	return ""
}
