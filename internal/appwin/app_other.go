//go:build !windows

package appwin

import "errors"

func Run(initialFiles []string) error {
	return errors.New("GUI is only available on Windows")
}
