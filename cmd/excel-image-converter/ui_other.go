//go:build !windows

package main

import (
	"errors"
	"fmt"
)

func chooseExcelFiles() ([]string, error) {
	return nil, errors.New("please pass one or more .xlsx files as arguments")
}

func showInfo(title, message string) {
	fmt.Printf("%s\n%s\n", title, message)
}

func showWarning(title, message string) {
	fmt.Printf("%s\n%s\n", title, message)
}

func showError(title string, err error) {
	fmt.Printf("%s\n%s\n", title, err)
}
