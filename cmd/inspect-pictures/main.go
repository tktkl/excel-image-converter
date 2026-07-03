package main

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: inspect-pictures <file.xlsx> <sheet> <cell>")
		os.Exit(2)
	}

	f, err := excelize.OpenFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	pictures, err := f.GetPictures(os.Args[2], os.Args[3])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(len(pictures))
	for idx, picture := range pictures {
		sum := sha256.Sum256(picture.File)
		fmt.Printf("%d\tinsertType=%v\text=%s\tsize=%d\tsha256=%x\n", idx+1, picture.InsertType, picture.Extension, len(picture.File), sum)
	}
}
