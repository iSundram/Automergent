package installer

import "fmt"

var (
	Version   = "0.0.0"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func VersionString() string {
	return fmt.Sprintf("installer/%s (commit=%s, built=%s)", Version, Commit, BuildDate)
}
