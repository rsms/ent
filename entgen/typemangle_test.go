package main

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"
	"testing"
)

type testTypeInfo struct {
	t types.Type
	s string
}
type testTypeInfoList []testTypeInfo

func (a testTypeInfoList) Len() int           { return len(a) }
func (a testTypeInfoList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a testTypeInfoList) Less(i, j int) bool { return a[i].s < a[j].s }

func TestTypemangle(t *testing.T) {
	// TODO: improve this test
	pkg, typelist, maxtypename := testParseTypes(`
import (
  "unsafe"
  "bytes"
)

type S string

var a, b, c = len(b), S(c), "hello"
var _ []byte
var _ []uint8

var _ bool
var _ int
var _ int8
var _ int16
var _ int32
var _ int64
var _ uint
var _ uint8
var _ uint16
var _ uint32
var _ uint64
var _ uintptr
var _ float32
var _ float64
var _ complex64
var _ complex128
var _ string
var _ unsafe.Pointer

var _ []uint32
var _ [][]uint32
var _ [12]uint32
var _ [12][9]uint32

var _ map[string][]uint32
var _ map[string]struct{}
var _ map[unsafe.Pointer]map[string][][]int

var _ chan int
var _ chan (<-chan []float32)
var _ chan (chan<- string)

var _ func(int, ...int)
var s []byte
var _ = append(s, "foo"...)
var _ Link

type Link struct {
  prev *Link
  next *Link
  bytes.Buffer
}

func fib(x int) int {
  if x < 2 {
    return x
  }
  return fib(x-1) - fib(x-2)
}
`)

	// print types
	m := Mangler{Pkg: pkg}
	t.Logf("%-*s   %s\n", maxtypename, "GO", "MANGLED")
	for _, v := range typelist {
		m.Reset()
		if err := m.Type(v.t); err != nil {
			panic(err)
		}
		t.Logf("%-*s   %s\n", maxtypename, v.s, m.Bytes())
	}

	os.Exit(0)
}

func testParseTypes(src string) (pkg *types.Package, sortedTypes []testTypeInfo, maxtypename int) {
	if strings.Index(src, "package ") == -1 {
		src = "package foo\n" + src
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, 0) // (f *ast.File, err error)
	if err != nil {
		panic(err)
	}
	info := types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
	}
	conf := types.Config{Importer: importer.Default()}
	pkg, err = conf.Check("foo", fset, []*ast.File{f}, &info) // (*Package, error)
	if err != nil {
		panic(err)
	}

	// collect unique types
	typem := map[types.Type]string{}
	for _, tv := range info.Types {
		s := tv.Type.String()
		typem[tv.Type] = s
		if len(s) > maxtypename {
			maxtypename = len(s)
		}
	}

	// sort types
	sortedTypes = make([]testTypeInfo, 0, len(typem))
	for t, s := range typem {
		sortedTypes = append(sortedTypes, testTypeInfo{t, s})
	}
	sort.Sort(testTypeInfoList(sortedTypes))
	return
}
