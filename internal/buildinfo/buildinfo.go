package buildinfo

import "strings"

var Version = "1.0.15"

func DisplayVersion() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		return "dev"
	}
	return version
}
