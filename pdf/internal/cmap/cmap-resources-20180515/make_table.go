package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/unidoc/unidoc/pdf/internal/cmap"
)

func main() {
	makeTable()
}

func makeTable() {
	files0, err := filepath.Glob("*")
	if err != nil {
		panic(err)
	}
	files := []string{}
	for _, fn := range files0 {
		if strings.Contains(fn, ".") {
			continue
		}
		files = append(files, fn)
	}
	sort.Strings(files)
	files = files[:1]
	table := map[string]*cmap.CMap{}
	for _, fn := range files {
		cmap, err := cmap.LoadCmapFromFile(fn)
		if err != nil {
			panic(err)
		}
		table[fn] = cmap
	}
	fmt.Printf("%d cmaps\n", len(table))

	fmt.Println("var cmapTable map[string]cmap.CMap{")
	for _, fn := range files {
		cmap := table[fn]
		fmt.Printf("\t%#q: %#v\n", fn, *cmap)
		cmap.Print()
	}
	fmt.Println("}")

}
