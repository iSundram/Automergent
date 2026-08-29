package commands

import (
	"fmt"
	"runtime"
)

// /version — show the Automergent version.

func versionCommand() Command {
	return Command{
		Name:             "version",
		Description:      "Show the Automergent version",
		Category:         "System",
		Icon:             "󰬐",
		Tier:             TierTertiary,
		Immediate:        true,
		SupportsHeadless: true,
	}
}

func handleVersion(host Host, args []string) Result {
	host.AddSystemMessage(fmt.Sprintf("Automergent %s (%s %s/%s)", host.Version(), runtime.GOOS, runtime.GOARCH, runtime.Version()))
	return Done(nil)
}
