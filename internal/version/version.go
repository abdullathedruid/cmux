package version

// These variables are set at build time using ldflags.
// Example: go build -ldflags "-X github.com/abdullathedruid/cmux/internal/version.GitSHA=$(git rev-parse --short HEAD)"
var (
	// GitSHA is the git commit SHA (short form) at build time.
	GitSHA = "dev"
)

// Short returns a short version string suitable for display.
func Short() string {
	return GitSHA
}
