//go:build !windows

package main

func runApp() int {
	return runCLI()
}
