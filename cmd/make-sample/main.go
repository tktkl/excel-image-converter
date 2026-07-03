package main

import (
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: make-sample <image-url> <output.xlsx>")
		os.Exit(2)
	}

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "Product")
	_ = f.SetCellValue(sheet, "B1", "Image")
	_ = f.SetCellValue(sheet, "A2", "Demo")
	_ = f.SetCellFormula(sheet, "B2", fmt.Sprintf(`=IMAGE("%s")`, os.Args[1]))
	_ = f.SetColWidth(sheet, "B", "B", 20)
	_ = f.SetRowHeight(sheet, 2, 80)

	if err := f.SaveAs(os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
