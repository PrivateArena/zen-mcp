package skills

// SkillRegistryEntry represents a skill entry in the registry.
type SkillRegistryEntry struct {
	ID          string
	Title       string
	Description string
	Framework   string
	Path        string
	Triggers    []string
}

// BundledResources represents bundled skill resources.
type BundledResources struct {
	Rules    []string
	Scripts  []string
	Assets   []string
}

// ResolvedReference represents a resolved file reference.
type ResolvedReference struct {
	AbsolutePath string
	RelativePath string
}

// ResolvedSkillContent represents enriched skill content with references.
type ResolvedSkillContent struct {
	Enriched      string
	CommandHints  []string
	FileReferences []ResolvedReference
}
