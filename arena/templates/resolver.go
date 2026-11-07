package templates

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// RepoResolver handles repository resolution and template reference parsing.
type RepoResolver struct {
	config *RepoConfig
}

// NewRepoResolver creates a new repository resolver with the given config.
func NewRepoResolver(config *RepoConfig) *RepoResolver {
	return &RepoResolver{config: config}
}

// TemplateRef represents a parsed template reference.
type TemplateRef struct {
	RepoName     string     // e.g., "local", "community"
	TemplateName string     // e.g., "iot-maintenance-demo"
	Version      string     // e.g., "1.0.0" or empty
	Repository   Repository // The resolved repository
	// CacheKey is the directory name the template is cached under. It equals
	// RepoName for a configured repo; for a raw index path or URL it is a
	// stable hash, because the raw value is not a safe directory name.
	CacheKey string
}

// ParseTemplateRef parses a template reference like "local/template@version" or "template@version".
// If no repo prefix is present, uses the provided defaultRepo.
func (r *RepoResolver) ParseTemplateRef(ref, defaultRepo string) (*TemplateRef, error) {
	result := &TemplateRef{}

	// Extract version if present (name@version)
	name := ref
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		result.Version = ref[idx+1:]
		name = ref[:idx]
	}

	// Extract repo prefix if present (repo/name)
	const maxParts = 2
	parts := strings.SplitN(name, "/", maxParts)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		// Has repo prefix
		result.RepoName = parts[0]
		result.TemplateName = parts[1]
	} else {
		// No repo prefix, use default
		result.RepoName = defaultRepo
		if result.RepoName == "" {
			result.RepoName = DefaultRepoName
		}
		result.TemplateName = name
	}

	// Resolve the repository
	result.Repository = ResolveIndex(result.RepoName, r.config)
	result.CacheKey = CacheRepoKey(result.RepoName, r.config)

	return result, nil
}

// cacheKeyHexLen is how many hex characters of the index hash name a cache
// directory for an unnamed index — enough to keep distinct indexes apart.
const cacheKeyHexLen = 12

// CacheRepoKey returns the cache directory name for a repo reference. A repo
// configured in cfg is cached under its own name. Anything else is a raw index
// path or URL (`--index ../templates/index.yaml`), which cannot be used as a
// directory name: a relative path escapes the cache directory and an absolute
// one nests the whole path inside it. Those are keyed by a hash of the
// absolute path or the URL, so `templates fetch` and `templates render` given
// the same --index agree on where the template lives.
func CacheRepoKey(repoName string, cfg *RepoConfig) string {
	if cfg != nil {
		if _, ok := cfg.Repos[repoName]; ok {
			return repoName
		}
	}
	location := repoName
	if !isHTTPURL(location) {
		if abs, err := filepath.Abs(location); err == nil {
			location = abs
		}
	}
	sum := sha256.Sum256([]byte(location))
	return "index-" + hex.EncodeToString(sum[:])[:cacheKeyHexLen]
}

// GetCachePath returns the cache path for a template within a repository.
func (r *RepoResolver) GetCachePath(cacheDir string, ref *TemplateRef) string {
	key := ref.CacheKey
	if key == "" {
		key = ref.RepoName
	}
	return filepath.Join(cacheDir, key, ref.TemplateName, ref.Version, "template.yaml")
}

// SplitTemplateRef is a standalone utility that splits "repo/template" into parts.
// Returns empty repo string if no repo prefix is present.
func SplitTemplateRef(ref string) (repo, name string) {
	const maxParts = 2
	parts := strings.SplitN(ref, "/", maxParts)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		// Check if this looks like a file path (starts with . or /)
		if strings.HasPrefix(ref, "./") || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "../") {
			return "", ref
		}
		return parts[0], parts[1]
	}
	return "", ref
}

// ResolveRepoForTemplate determines which repository to use for a template reference.
// It checks if the template name has a repo prefix and uses that, otherwise uses defaultRepo.
func (r *RepoResolver) ResolveRepoForTemplate(
	templateName, defaultRepo string,
) (repo Repository, cleanName string) {
	repoName := defaultRepo
	if repo, _ := SplitTemplateRef(templateName); repo != "" {
		repoName = repo
	}
	if repoName == "" {
		repoName = DefaultRepoName
	}
	return ResolveIndex(repoName, r.config), repoName
}
