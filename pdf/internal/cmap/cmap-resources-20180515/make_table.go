package main

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"../../cmap"
)

func main() {
	makeCMapTable()
	// makeCidToCode()
}

var dirList = []string{
	"Adobe-CNS1-7",
	"Adobe-Identity-0",
	"Adobe-Japan1-6",
	"Adobe-Korea1-2",
	"Adobe-GB1-5",
	"Adobe-KR-9",
}

func makeCMapTable() {
	nameDir := map[string]string{}
	cmapTable := map[string]*cmap.CMap{}
	allFiles := []string{}
	for _, dir := range dirList {
		for _, fn := range listFiles(dir) {
			name := path.Base(fn)
			if _, ok := nameDir[name]; ok {
				panic("duplicate name")
			}
			nameDir[name] = dir[:len(dir)-2]
			allFiles = append(allFiles, fn)
		}
	}
	fmt.Printf("// %d files\n", len(allFiles))
	for _, fn := range allFiles {
		cmap, err := cmap.LoadCmapFromFile(fn)
		if err != nil {
			panic(err)
		}
		cmapTable[path.Base(fn)] = cmap
	}
	order := sortNameDir(nameDir)
	printNameDir(nameDir, order, cmapTable)
	printCMapTable(cmapTable, order)
	// organizeCMaps(cmapTable)
}

func printNameDir(nameDir map[string]string, order []string, cmapTable map[string]*cmap.CMap) {
	fmt.Printf("var nameToCIDMap = map[string]string { // %d entries\n", len(nameDir))
	for _, name := range order {
		dir := nameDir[name]
		cmap := cmapTable[name]
		si := cmap.SystemInfo()
		fmt.Printf("\t%#20q: %q, // %s:%s:%d \n", name, dir, si.Registry, si.Ordering, si.Supplement)
	}
	fmt.Println("}")
}

func organizeCMaps(cmapTable map[string]*cmap.CMap) {
	regOrdSupFn := map[string]map[string]map[int][]string{}
	for fn, cmap := range cmapTable {
		si := cmap.SystemInfo()
		if _, ok := regOrdSupFn[si.Registry]; !ok {
			regOrdSupFn[si.Registry] = map[string]map[int][]string{}
		}
		if _, ok := regOrdSupFn[si.Registry][si.Ordering]; !ok {
			regOrdSupFn[si.Registry][si.Ordering] = map[int][]string{}
		}
		regOrdSupFn[si.Registry][si.Ordering][si.Supplement] = append(
			regOrdSupFn[si.Registry][si.Ordering][si.Supplement], fn)
	}
	fmt.Println("================================0")
	fmt.Printf("%d registries\n", len(regOrdSupFn))
	for r, reg := range sortRegistry(regOrdSupFn) {
		ordSupFn := regOrdSupFn[reg]
		fmt.Println("================================1")
		fmt.Printf("Registry %d of %d: %q %d vals\n", r+1, len(regOrdSupFn), reg, len(ordSupFn))
		for o, ord := range sortOrdering(ordSupFn) {
			supFn := ordSupFn[ord]
			fmt.Println("================================1")
			fmt.Printf("Ordering %d of %d: %q %d vals\n", o+1, len(ordSupFn), ord, len(supFn))
			for s, sup := range sortSupplement(supFn) {
				fnList := supFn[sup]
				fmt.Println("================================1")
				fmt.Printf("Supplement %d of %d: %d %d vals\n", s+1, len(supFn), sup, len(fnList))
				for f, fn := range sortFiles(fnList) {
					fmt.Printf("File %d of %d: %q \n", f+1, len(fnList), fn)
				}
			}
		}
	}
}

func sortRegistry(m map[string]map[string]map[int][]string) (sorted []string) {
	for k := range m {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	return
}

func sortOrdering(m map[string]map[int][]string) (sorted []string) {
	for k := range m {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	return
}

func sortSupplement(m map[int][]string) (sorted []int) {
	for k := range m {
		sorted = append(sorted, k)
	}
	sort.Ints(sorted)
	return
}

func sortFiles(m []string) (sorted []string) {
	for _, k := range m {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	return
}

func sortTable(m map[string]*cmap.CMap) (sorted []string) {
	for k := range m {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	return
}

func sortNameDir(nameDir map[string]string) (sorted []string) {
	for k := range nameDir {
		sorted = append(sorted, k)
	}
	sort.Slice(sorted, func(i, j int) bool {
		iName, jName := sorted[i], sorted[j]
		iDir, jDir := nameDir[iName], nameDir[jName]
		iId := strings.Contains(iDir, "Adobe-Identity")
		jId := strings.Contains(jDir, "Adobe-Identity")
		if iId != jId {
			return iId
		}
		if iDir != jDir {
			return iDir < jDir
		}
		return iName < jName
	})
	return
}

func printCMapTable(cmapTable map[string]*cmap.CMap, order []string) {
	fmt.Printf("var cmapTable =  map[string]CMap{\n")
	for _, fn := range order {
		printCMap(fn, cmapTable[fn])
	}
	fmt.Println("}")
}

func printCMap(name string, cmap *cmap.CMap) {
	fmt.Printf("\t%q : CMap { \n", name)
	fmt.Printf("\tname: %#q,\n", cmap.Name())
	fmt.Printf("\tctype: %d,\n", cmap.Type())
	if cmap.Usecmap() != "" {
		fmt.Printf("\tusecmp: %#q,\n", cmap.Usecmap())
	}
	si := cmap.SystemInfo()
	fmt.Printf("\tsystemInfo: CIDSystemInfo{Registry: %q, Ordering: %q, Supplement: %d},\n",
		si.Registry, si.Ordering, si.Supplement)
	codespaces := cmap.Codespaces()
	fmt.Printf("\tcodespaces: []Codespace{ // %d entries\n", len(codespaces))
	for _, r := range codespaces {
		fmt.Printf("\t\tCodespace{Low:0x%04x, High:0x%04x, NumBytes:%4d,},\n",
			r.Low, r.High, r.NumBytes)
	}
	fmt.Println("\t},")
	cidRanges := cmap.CidRanges()
	fmt.Printf("\tcidRanges: []CIDRange{ // %d entries\n", len(cidRanges))
	for _, r := range cidRanges {
		fmt.Printf("\t\tCIDRange{From:0x%04x, To:0x%04x, Cid:%4d},\n", r.From, r.To, r.Cid)
	}
	fmt.Println("\t},")
	fmt.Println("},")
	// fmt.Printf("\tcodespaces []codespace{ //%d codespaces\n", len(cmap.codespaces))
	// for i, cs := range cmap.codespaces {
	// 	fmt.Printf("\t%#v, // %d of %d\n", cs, i+1, len(cmap.codespaces))
	// }
	// fmt.Println("},")
	// // codeMap [4]map[uint64]string
	// fmt.Println("\tcodeMap [4]map[uint64]string{")
	// for i := 0; i < 4; i++ {
	// 	codeMap := cmap.codeMap[i]
	// 	fmt.Printf("\tmap[uint64]string{ //%d entries\n", len(codeMap))
	// 	for j, cs := range codeMap {
	// 		fmt.Printf("\t%#v, // %d of %d\n", cs, j+1, len(codeMap))
	// 	}
	// 	fmt.Println("},")
	// }

}

func listFiles(dir string) []string {
	mask := filepath.Join(dir, "CMap", "*")
	files0, err := filepath.Glob(mask)
	if err != nil {
		panic(err)
	}
	files := []string{}
	for _, fn := range files0 {
		mustExist(fn)
		if strings.Contains(fn, ".") {
			continue
		}
		files = append(files, fn)
	}
	sort.Strings(files)
	// fmt.Printf("files=%+v\n", files)
	return files
}

func makeCidToCode() {
	cid2Rune := map[string]map[int]rune{}
	fmt.Printf("%s\n", header)
	for i, dir := range dirList {
		c2r := readCid2Code(dir)
		fmt.Printf("// %d: %-20s %d codes\n", i, dir, len(c2r))
		cid2Rune[dir] = c2r
	}

	fmt.Println("var cid2Rune =  map[string]map[int]rune{")
	for _, dir := range dirList[0:] {
		c2r := readCid2Code(dir)
		printCid2Code(dir, c2r)
	}
	fmt.Println("}")
}

func printCid2Code(name string, c2r map[int]rune) {
	maxCode, maxRune := stats(c2r)
	fmt.Printf("\t%q : map[int]rune { // %d entries, max cid=0x%0x, max rune=0x%0x\n",
		name, len(c2r), maxCode, maxRune)
	for _, c := range keys(c2r) {
		if c == 0 {
			continue
		}
		r := c2r[c]
		s := " "
		if unicode.IsPrint(r) {
			s = fmt.Sprintf("%c", r)
		}
		s = strings.Replace(s, "\n", "", -1)
		s = strings.Replace(s, "\r", "", -1)
		fmt.Printf("\t\t0x%04x: '\\U%08x', // %2s\n", c, r, s)
	}
	fmt.Println("},")
}

var header = `package main

import "fmt"

func main() {
    c2r := cid2Rune["Adobe-CNS1-7"]
    r := c2r[0x0256]
    fmt.Printf("%c\n", r)
}`

func stats(c2r map[int]rune) (maxCode int, maxRune rune) {
	for c, r := range c2r {
		if c > maxCode {
			maxCode = c
		}
		if r > maxRune {
			maxRune = r
		}
	}
	return
}

func keys(c2r map[int]rune) []int {
	k := []int{}
	for c := range c2r {
		k = append(k, c)
	}
	sort.Ints(k)
	return k
}

func readCid2Code(dir string) map[int]rune {
	cidpath := cidPath(dir)
	f, err := os.Open(cidpath)
	if err != nil {
		panic(err)
	}
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.Comment = '#'
	fields, err := r.ReadAll()
	if err != nil {
		panic(err)
	}
	header := fields[0]
	iUtf8 := -1
	for i, v := range header {
		n := len(v)
		if n > 5 && v[n-5:] == "-UTF8" {
			iUtf8 = i
			break
		}
	}
	if iUtf8 == -1 {
		fmt.Println("No utf8 column.")
		fmt.Printf("%v\n", header)
		panic("No utf8 column")
	}
	if false {
		fmt.Printf("fields=%d\n", len(fields))
		for i, row := range fields[:3] {
			fmt.Printf("row %d=%d\n", i, len(row))
		}

		fmt.Printf("%v\n", header)
		fmt.Printf("utf-8 column=%d\n", iUtf8)
		fmt.Printf("fields=%d\n", len(fields))
		for i, row := range fields[:3] {
			fmt.Printf("row %d=%d\n", i, len(row))
		}
		fmt.Printf("fields=%d\n", len(fields))
		for i, row := range fields[2:12] {
			u := row[iUtf8]
			r, _ := toRune(u)
			fmt.Printf("%3d:%8s %8s %c\n", i, row[0], u, r)
		}
		for i, row := range fields[len(fields)-10:] {
			u := row[iUtf8]
			r, _ := toRune(u)
			fmt.Printf("%3d:%8s %8s %c\n", i, row[0], u, r)
		}
	}

	cid2rune := map[int]rune{}
	for i, row := range fields[2:] {
		c := row[0]
		if c == "*" {
			panic(fmt.Sprintf("%d: %v", i, row))
		}
		cid, err := strconv.Atoi(c)
		if err != nil {
			panic(fmt.Sprintf("%d: %v", i, row))
		}
		u := row[iUtf8]
		r, err := toRune(u)
		if err != nil {
			panic(fmt.Sprintf("%d: %v", i, row))
		}
		if cid == 0 || r == 0 {
			continue
		}
		cid2rune[cid] = r
	}
	return cid2rune
}

func cidPath(dir string) string {
	// gopath := os.Getenv("GOPATH")
	// mustExist(gopath)
	// unidoc := path.Join(gopath, "src/github.com/unidoc/unidoc")
	// mustExist(unidoc)
	// cmappath := path.Join(unidoc, "pdf", "internal", "cmap", "cmap-resources-20180515")
	// mustExist(cmappath)
	// cidpath := path.Join(cmappath, dir, "cid2code.txt")
	cidpath := path.Join(".", dir, "cid2code.txt")
	mustExist(cidpath)
	return cidpath
}

func mustExist(filename string) {
	if filename == "" {
		panic("no file")
	}
	if _, err := os.Stat(filename); err != nil {
		panic(filename)
	}
}

func toRune(v string) (r rune, error error) {
	v = strings.Split(v, ",")[0]
	b, err := hex.DecodeString(v)
	if err != nil {
		return
	}
	r, n := utf8.DecodeRune(b)
	if n != len(b) {
		panic(v)
	}
	// if int(r) > 0xffff {
	//  fmt.Fprintf(os.Stderr, "r=0x%0x\n", r)
	//  fmt.Fprintf(os.Stderr, "r=%c\n", r)
	//  s := fmt.Sprintf("'\\u%04x'", r)
	//  s2 := strconv.QuoteRune(r)
	//  fmt.Fprintf(os.Stderr, "s=%s\n", s)
	//  fmt.Fprintf(os.Stderr, "s2=%s\n", s2)
	//  c := '\U000200cc'
	//  fmt.Fprintf(os.Stderr, "c=%s\n", c)
	//  panic("Rune too big")
	// }
	return
}
