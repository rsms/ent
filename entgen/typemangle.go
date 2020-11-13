package main

/*
Typemangle composes a go identifer-safe string from a go type

Examples:

GO                                                     MANGLED

*foo.Link                                              PNLink
*foo.Link                                              PNLink
<-chan []float32                                       CrVf04
[12][9]uint32                                          A12_A9_u04
[12]uint32                                             A12_u04
[9]uint32                                              A9_u04
[][]int                                                VVi00
[][]uint32                                             VVu04
[]byte                                                 Vu01
[]byte                                                 Vu01
[]float32                                              Vf04
[]int                                                  Vi00
[]int                                                  Vi00
[]uint32                                               Vu04
[]uint32                                               Vu04
[]uint32                                               Vu04
[]uint8                                                Vu01
bool                                                   b
byte                                                   u01
bytes.Buffer                                           Nbytes_Buffer
chan (<-chan []float32)                                CbCrVf04
chan chan<- string                                     CbCss
chan int                                               Cbi00
chan<- string                                          Css
complex128                                             c16
complex64                                              c08
float32                                                f04
float64                                                f08
foo.Link                                               NLink
foo.S                                                  NS
func([]byte, string...) []byte                         FT2_Vu01XsRVu01
func(int, ...int)                                      FT2_i00Xi00
func(string) int                                       FT1_sRi00
func(x int) int                                        FT1_i00Ri00
int                                                    i00
int16                                                  i02
int32                                                  i04
int64                                                  i08
int8                                                   i01
map[string][][]int                                     MsVVi00
map[string][]uint32                                    MsVu04
map[string]struct{}                                    MsS
map[unsafe.Pointer]map[string][][]int                  MpMsVVi00
string                                                 s
struct{prev *foo.Link; next *foo.Link; bytes.Buffer}   SprevPNLink_nextPNLink__Nbytes_Buffer
struct{}                                               S
uint                                                   u00
uint16                                                 u02
uint32                                                 u04
uint64                                                 u08
uint8                                                  u01
uintptr                                                upp
unsafe.Pointer                                         p
untyped bool                                           kb
untyped int                                            ki
untyped string                                         ks

*/

import (
	"bytes"
	"fmt"
	"go/types"
)

func Typemangle(pkg *types.Package, typ types.Type) (string, error) {
	m := Mangler{Pkg: pkg}
	err := m.Type(typ)
	return string(m.Bytes()), err
}

const (
	csep      = '_'
	carray    = 'A'
	cfunc     = 'F'
	cpointer  = 'P'
	cresult   = 'R'
	cslice    = 'V'
	ctuple    = 'T'
	cstruct   = 'S'
	cnamed    = 'N'
	cmap      = 'M'
	cvariadic = 'X'
)

const (
	cChanSendRecv = "Cb"
	cChanSendOnly = "Cs"
	cChanRecvOnly = "Cr"
)

var cBasic = []string{
	types.Invalid: "?",

	types.Bool:          "b",   // bool
	types.Int:           "i00", // int
	types.Int8:          "i01", // int8
	types.Int16:         "i02", // int16
	types.Int32:         "i04", // int32
	types.Int64:         "i08", // int64
	types.Uint:          "u00", // uint
	types.Uint8:         "u01", // uint8
	types.Uint16:        "u02", // uint16
	types.Uint32:        "u04", // uint32
	types.Uint64:        "u08", // uint64
	types.Uintptr:       "upp", // uintptr
	types.Float32:       "f04", // float32
	types.Float64:       "f08", // float64
	types.Complex64:     "c08", // complex64
	types.Complex128:    "c16", // complex128
	types.String:        "s",   // string
	types.UnsafePointer: "p",   // unsafe.Pointer

	types.UntypedBool:    "kb", // untyped bool
	types.UntypedInt:     "ki", // untyped int
	types.UntypedRune:    "kr", // untyped rune
	types.UntypedFloat:   "kf", // untyped float
	types.UntypedComplex: "kc", // untyped complex
	types.UntypedString:  "ks", // untyped string
	types.UntypedNil:     "kz", // untyped nil
}

type Mangler struct {
	Pkg *types.Package
	buf bytes.Buffer
	err error
}

func (m *Mangler) Type(typ types.Type) error {
	m.wtype(typ, make([]types.Type, 0, 8))
	return m.err
}

func (m *Mangler) Bytes() []byte {
	b := m.buf.Bytes()
	if len(b) > 0 && b[0] == csep {
		b = b[1:] // strip leading "_"
	}
	return b
}

func (m *Mangler) Reset() {
	m.buf.Reset()
	m.err = nil
}

func (m *Mangler) wbyte(b byte)  { m.buf.WriteByte(b) }
func (m *Mangler) wstr(s string) { m.buf.WriteString(s) }
func (m *Mangler) setErr(err error) {
	if m.err == nil {
		m.err = err
	}
}

func (m *Mangler) wtype(typ types.Type, visited []types.Type) {
	// This code has been adopted from the go source, specifically go/types/typestring.go
	buf := &m.buf

	// Theoretically, this is a quadratic lookup algorithm, but in
	// practice deeply nested composite types with unnamed component
	// types are uncommon. This code is likely more efficient than
	// using a map.
	for _, t := range visited {
		if t == typ {
			m.wstr("CYCLE")
			m.setErr(fmt.Errorf("cyclic type %T", typ))
			return
		}
	}
	visited = append(visited, typ)

	// m.wbyte(csep)

	switch t := typ.(type) {
	case nil:
		m.setErr(fmt.Errorf("unknown/nil typeof %v", typ))
		m.wbyte('?')

	case *types.Basic:
		kind := t.Kind()
		// // unwrap aliases
		// switch kind {
		// case types.Byte:
		// 	kind = types.Uint8
		// 	// t = types.Typ[types.Uint8]
		// case types.Rune:
		// 	kind = types.Int32
		// 	// t = types.Typ[types.Int32]
		// }
		// Note: t.Name() returns the go name of the basic type, e.g. uint32
		m.wstr(cBasic[kind])

	case *types.Array:
		fmt.Fprintf(buf, "%c%d%c", carray, t.Len(), csep)
		m.wtype(t.Elem(), visited)

	case *types.Slice:
		m.wbyte(cslice)
		m.wtype(t.Elem(), visited)

	case *types.Pointer:
		m.wbyte(cpointer)
		m.wtype(t.Elem(), visited)

	case *types.Tuple:
		m.wbyte(ctuple)
		m.wtuple(t, false, visited)

	case *types.Signature:
		m.wbyte(cfunc)
		m.wsignature(t, visited)

	case *types.Struct:
		m.wstruct(t, visited)

	case *types.Named:
		m.wbyte(cnamed)
		if obj := t.Obj(); obj != nil {
			if pkg := obj.Pkg(); pkg != nil {
				if m.wpkg(pkg) {
					m.wbyte(csep)
				}
			}
			m.wstr(obj.Name())
		} else {
			m.setErr(fmt.Errorf("named type %v without object", t))
		}

	case *types.Map:
		m.wbyte(cmap)
		m.wtype(t.Key(), visited)
		m.wtype(t.Elem(), visited)

	case *types.Chan:
		switch t.Dir() {
		case types.SendRecv:
			m.wstr(cChanSendRecv)
		case types.SendOnly:
			m.wstr(cChanSendOnly)
		case types.RecvOnly:
			m.wstr(cChanRecvOnly)
		}
		m.wtype(t.Elem(), visited)

	// TODO: case *types.Interface:

	default:
		m.setErr(fmt.Errorf("type not supported by Typemangle: %s", t))
		m.wstr(t.String())
	}

}

func (m *Mangler) wstruct(st *types.Struct, visited []types.Type) {
	m.wbyte(cstruct)
	z := st.NumFields()
	for i := 0; i < z; i++ {
		f := st.Field(i)
		if i > 0 {
			// m.wstr("\u01C1")
			m.wbyte(csep)
		}
		if !f.Embedded() {
			// m.wbyte(csep)
			// m.wstr("ɑ")
			m.wstr(f.Name())
		} else {
			m.wbyte(csep)
		}
		m.wtype(f.Type(), visited)
		// if tag := st.Tag(i); tag != "" {
		// 	fmt.Fprintf(&m.buf, " %q", tag)
		// }
	}
}

func (m *Mangler) wsignature(sig *types.Signature, visited []types.Type) {
	m.wtuple(sig.Params(), sig.Variadic(), visited)

	results := sig.Results()
	n := results.Len()
	if n == 0 {
		// no result
		return
	}

	// m.wbyte(csep)
	m.wbyte(cresult)
	if n == 1 && results.At(0).Name() == "" {
		// single unnamed result
		m.wtype(results.At(0).Type(), visited)
		return
	}

	// multiple or named result(s)
	m.wtuple(results, false, visited)
}

func (m *Mangler) wtuple(tup *types.Tuple, variadic bool, visited []types.Type) {
	if tup == nil {
		m.wstr("T0")
		m.wbyte(csep)
		return
	}
	z := tup.Len()
	fmt.Fprintf(&m.buf, "T%d%c", z, csep)
	for i := 0; i < z; i++ {
		v := tup.At(i)
		// if i > 0 {
		// 	m.wbyte(csep)
		// }
		// ignore name of argument
		// if v.Name() != "" {
		// 	m.wstr(v.Name())
		// 	m.wbyte(csep)
		// }
		typ := v.Type()

		if variadic && i == z-1 {
			if s, ok := typ.(*types.Slice); ok {
				typ = s.Elem()
			} else {
				// special case:
				// append(s, "foo"...) leads to signature func([]byte, string...)
				if t, ok := typ.Underlying().(*types.Basic); !ok || t.Kind() != types.String {
					panic("internal error: string type expected")
				}
				// continue
			}
			m.wbyte(cvariadic)
		}
		m.wtype(typ, visited)
	}
}

func (m *Mangler) wpkg(pkg *types.Package) bool {
	if pkg == nil {
		return false
	}
	if m.Pkg != nil && (m.Pkg == pkg || m.Pkg.Path() == pkg.Path()) {
		// pkg == m.Pkg
		return false
	}
	m.wstr(pkg.Path())
	return true
}
