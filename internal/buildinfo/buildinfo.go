package buildinfo

import "strings"

var Version = "1.0.14"

func DisplayVersion() string {
	version := strings.TrimSpace(Version)
	if version == "" {
		return "dev"
	}
	return version
}
