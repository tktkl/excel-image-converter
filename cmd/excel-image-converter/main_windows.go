//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/wutong/excel-image-converter/internal/appwin"
	"github.com/wutong/excel-image-converter/internal/buildinfo"
)

func runApp() int {
	if err := appwin.Run(os.Args[1:]); err != nil {
		showError(fmt.Sprintf("Excel 图片转换器 v%s", buildinfo.DisplayVersion()), err)
		return 1
	}
	return 0
}
