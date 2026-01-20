package version

import "runtime/debug"

// GitSHA is the git commit SHA. It's automatically populated from
// Go's built-in VCS info when installed via `go install`.
// Can also be overridden at build time using ldflags:
//   go build -ldflags "-X github.com/abdullathedruid/cmux/internal/version.GitSHA=abc123"
var GitSHA = ""

func init() {
	if GitSHA != "" {
		return // ldflags override takes precedence
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		GitSHA = "dev"
		return
	}

	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			GitSHA = setting.Value
			if len(GitSHA) > 7 {
				GitSHA = GitSHA[:7] // short SHA
			}
			return
		}
	}

	GitSHA = "dev"
}

// Short returns a short version string suitable for display.
func Short() string {
	return GitSHA
}
