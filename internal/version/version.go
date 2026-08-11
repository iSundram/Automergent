package version

import "fmt"

var (
	Version   = "0.0.0"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("automergent/%s (commit=%s, built=%s)", Version, Commit, BuildDate)
}
