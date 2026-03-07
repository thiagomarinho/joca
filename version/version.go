package version

// Version and Commit are set at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
)
