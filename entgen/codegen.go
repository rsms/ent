package main

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/rsms/go-bits"
	"github.com/rsms/go-log"
)

type codecDir int

const (
	codecEncode = codecDir(iota)
	codecDecode
)

// "Automatically generated" header, used for improved safety when deleting unused files.
// Note that this should contain the regexp "go generate" expects, which is as follows:
//   ^// Code generated .* DO NOT EDIT\.$
// See `go help generate` for more information.
var generatedByHeaderPrefix = "// +build !entgen\n\n// Code generated by entgen. DO NOT EDIT."

type PkgImport struct {
	Path string
	Name string // optional local name
}

type Codegen struct {
	pkg        *Package
	srcdir     string
	entpkgPath string // path of the ent package

	generatedFunctions map[string]bool

	pos      token.Pos // best source pos for whater is currently being generated
	posstack []token.Pos

	// main write stream
	wbuf bytes.Buffer
	w    *tabwriter.Writer // conforms to io.Writer

	// helpers
	helperbuf bytes.Buffer
	helperw   *tabwriter.Writer // conforms to io.Writer
	helperm   map[string]error

	// imported packages (does not include the ent package)
	imports []PkgImport

	// options
	PrivateFieldSetters bool // generate "setField" methods instead of "SetField" methods
}

func NewCodegen(pkg *Package, srcdir, entpkgPath string) *Codegen {
	g := &Codegen{
		pkg:                pkg,
		srcdir:             srcdir,
		entpkgPath:         entpkgPath,
		posstack:           make([]token.Pos, 0, 8),
		generatedFunctions: map[string]bool{},
	}
	g.w = tabwriter.NewWriter(&g.wbuf, 0, 1, 1, ' ', tabwriter.TabIndent)
	g.helperw = tabwriter.NewWriter(&g.helperbuf, 0, 1, 1, ' ', tabwriter.TabIndent)
	return g
}

func (g *Codegen) goTypeName(t types.Type) string {
	// note: if things ever gets slow, this could be a place to start trying adding caching
	// which shouldn't have an effect for basic types but might have an impact for complex types.
	return goTypeName(t, g.pkg.Types)
}

func (g *Codegen) f(format string, args ...interface{}) {
	fmt.Fprintf(g.w, format, args...)
}

func (g *Codegen) s(s string) {
	g.w.Write([]byte(s))
}

func (g *Codegen) flush() {
	g.w.Flush()
	g.helperw.Flush()
}

func (g *Codegen) Finalize() []byte {
	g.flush()
	b := g.wbuf.Bytes()

	// helpers
	helpers := g.helperbuf.Bytes()
	if len(helpers) > 0 {
		b = append(b, "\n// ---- helpers ----\n"...)
		b = append(b, helpers...)
	}

	// header
	header := &bytes.Buffer{}
	wf := func(format string, args ...interface{}) {
		fmt.Fprintf(header, format, args...)
	}
	wf("%s by %s. Edit with caution!\n", generatedByHeaderPrefix, filepath.Base(os.Args[0]))
	wf("package %s\n", g.pkg.Name)

	if len(g.imports) == 0 {
		wf("import %#v\n", g.entpkgPath)
	} else {
		wf("import (\n  %#v\n", g.entpkgPath)
		for _, im := range g.imports {
			if im.Name != "" {
				wf("  %s %#v\n", im.Name, im.Path)
			} else {
				wf("  %#v\n", im.Path)
			}
		}
		header.WriteString(")\n")
	}
	header.WriteByte('\n')
	b = append(header.Bytes(), b...)

	return b
}

func (g *Codegen) logErrUnsupportedType(f *EntField) {
	f.ent.logSrcErr(f.pos, "unsupported type %s of field %s.%s", f.t.Type, f.ent.sname, f.sname)
}

func (g *Codegen) logSrcErr(format string, args ...interface{}) {
	logSrcErr(g.srcdir, g.pkg, g.pos, format, args...)
}

func (g *Codegen) logSrcWarn(format string, args ...interface{}) {
	logSrcWarn(g.srcdir, g.pkg, g.pos, format, args...)
}

func (g *Codegen) pushPos(pos token.Pos) {
	g.posstack = append(g.posstack, g.pos)
	g.pos = pos
}

func (g *Codegen) popPos() {
	g.pos = g.posstack[len(g.posstack)-1]
	g.posstack = g.posstack[:len(g.posstack)-1]
}

// isMutableRefType returns true if t is a type which underlying value may be changed without
// assignment. For example a slice.
func isMutableRefType(typ types.Type) bool {
	for {
		switch t := typ.(type) {
		case *types.Array, *types.Slice, *types.Map, *types.Pointer, *types.Struct:
			return true

		case *types.Named:
			typ = t.Underlying()

		default:
			return false

		}
	}
	return false
}

// —————————————————————————————————————————————————————————————————————————————————————————
// encode

func (g *Codegen) codegenEncodeField(f *EntField) error {
	cvar := "c"
	expr, err := g.genFieldEncoder(f, cvar, "e."+f.sname)
	if err != nil {
		return err
	}
	g.f("  %s.Key(%#v)\n", cvar, f.name)
	g.f("  %s\n", expr)
	return nil
}

func (g *Codegen) genFieldEncoder(f *EntField, cvar, valexpr string) (string, error) {
	g.pushPos(f.t.pos)
	defer g.popPos()
	expr, err := g.encoderExpr(f.t.Type, cvar, valexpr)
	if err == ErrUnsupportedType {
		g.logErrUnsupportedType(f)
	}
	return expr, err
}

func (g *Codegen) encoderExpr(typ types.Type, cvar, valexpr string) (expr string, err error) {
	typ, cast := g.unwrapNamedType(typ)
	if cast != "" {
		// flip cast
		cast = g.goTypeName(typ)
	}

	switch t := typ.(type) {

	case *types.Basic:
		m, cast, advice := g.basicCodecCall(t, codecEncode, cast != "")
		if advice != "" {
			advice = ", " + advice
		}
		expr = fmt.Sprintf("%s.%s(%s%s)", cvar, m, wrapstr(valexpr, cast), advice)
		return

	case *types.Slice, *types.Array:
		if cast != "" {
			cast = ""
		}
		var elemt types.Type
		if st, ok := t.(*types.Slice); ok {
			elemt = st.Elem()
		} else {
			elemt = t.(*types.Array).Elem()
			// use slice of all arrays (even those that are not [N]byte)
			valexpr += "[:]"
			// convert to slice so that genComplexEncoder uses slice encoders instead of generating
			// array encoders.
			typ = types.NewSlice(elemt)
		}
		// special case for []byte
		if bt, ok := elemt.(*types.Basic); ok && bt.Kind() == types.Uint8 {
			expr = fmt.Sprintf("%s.Blob(%s)", cvar, wrapstr(valexpr, cast))
			return
		}
	}
	expr, err = g.getOrBuildTypeHelper(typ, cvar, "ent_encode_", g.genComplexEncoder)
	expr += "(" + cvar + ", " + wrapstr(valexpr, cast) + ")"
	return
}

func (g *Codegen) genComplexEncoder(typ types.Type, cvar string, buf *bytes.Buffer) error {
	wf := func(format string, args ...interface{}) {
		fmt.Fprintf(buf, format, args...)
	}
	goType := g.goTypeName(typ)
	wf("(%s ent.Encoder, v %s) {\n", cvar, goType)

	switch t := typ.(type) {

	case *types.Slice:
		expr, err := g.encoderExpr(t.Elem(), cvar, "val")
		if err != nil {
			return err
		}
		wf("  %s.BeginList(len(v))\n", cvar)
		wf("  for _, val := range v {\n")
		wf("    %s\n", expr)
		wf("  }\n")
		wf("  %s.EndList()\n", cvar)

	case *types.Map:
		expr, err := g.encoderExpr(t.Elem(), cvar, "val")
		if err != nil {
			return err
		}
		if kt, ok := t.Key().(*types.Basic); !ok || kt.Kind() != types.String {
			g.logSrcErr("unsupported map key type %s; only string map keys are supported", t.Key())
			return ErrUnsupportedType
		}
		wf("  %s.BeginDict(len(v))\n", cvar)
		wf("  for k, val := range v {\n")
		wf("    %s.Key(k)\n", cvar)
		wf("    %s\n", expr)
		wf("  }\n")
		wf("  %s.EndDict()\n", cvar)

	default:
		return ErrUnsupportedType

	} // switch typ

	wf("}\n")
	return nil
}

// —————————————————————————————————————————————————————————————————————————————————————————
// decode

func (g *Codegen) codegenDecodeField(f *EntField) error {
	g.pushPos(f.t.pos)
	defer g.popPos()
	expr, cast, err := g.decoderExpr(f.t.Type, "c")
	if err != nil {
		if err == ErrUnsupportedType {
			g.logErrUnsupportedType(f)
		}
		return err
	}
	g.f("  e.%s = %s\n", f.sname, wrapstr(expr, cast))
	return nil
}

// decoderExpr generates & returns a "decode" expression like "c.Int(64)"
func (g *Codegen) decoderExpr(typ types.Type, cvar string) (expr, cast string, err error) {
	typ, cast = g.unwrapNamedType(typ)
	switch t := typ.(type) {

	case *types.Basic:
		m, basicCast, advice := g.basicCodecCall(t, codecDecode, false)
		if cast == "" {
			cast = basicCast
		}
		expr = fmt.Sprintf("%s.%s(%s)", cvar, m, advice)
		return

	case *types.Slice:
		// special case for []byte
		if bt, ok := t.Elem().(*types.Basic); ok && bt.Kind() == types.Uint8 {
			expr = cvar + ".Blob()"
			return
		}

	case *types.Array:
		// arrays are decoded as slices then copied into arrays via a ent_slice_to_AN_T helper
		expr, err = g.getOrBuildTypeHelper(typ, cvar, "ent_slice_to_", g.genCopyHelper)
		if bt, ok := t.Elem().(*types.Basic); ok && bt.Kind() == types.Uint8 {
			// special case for [N]byte
			expr += "(" + cvar + ".Blob())"
		} else {
			slicet := types.NewSlice(t.Elem())
			expr2, err2 := g.getOrBuildTypeHelper(slicet, cvar, "ent_decode_", g.genComplexDecoder)
			err = err2
			expr += "(" + expr2 + "(" + cvar + "))"
		}
		return

	} // switch t:=typ.(type)
	expr, err = g.getOrBuildTypeHelper(typ, cvar, "ent_decode_", g.genComplexDecoder)
	expr += "(" + cvar + ")"
	return
}

// genCopyHelper assumes typ is *types.Array
func (g *Codegen) genCopyHelper(typ types.Type, cvar string, buf *bytes.Buffer) error {
	wf := func(format string, args ...interface{}) {
		fmt.Fprintf(buf, format, args...)
	}
	elemt := typ.(*types.Array).Elem()
	goType := g.goTypeName(typ)
	elemGoType := g.goTypeName(elemt)
	wf("(s []%s) (r %s) {\n", elemGoType, goType)
	wf("  copy(r[:], s)\n")
	wf("  return\n" +
		"}\n")
	return nil
}

func (g *Codegen) genComplexDecoder(typ types.Type, cvar string, buf *bytes.Buffer) error {
	wf := func(format string, args ...interface{}) {
		fmt.Fprintf(buf, format, args...)
	}

	goType := g.goTypeName(typ)
	wf("(%s ent.Decoder) (r %s) {\n", cvar, goType)

	switch t := typ.(type) {

	case *types.Slice:
		expr, cast, err := g.decoderExpr(t.Elem(), cvar)
		if err != nil {
			return err
		}
		wf("  n := %s.ListHeader()\n", cvar)
		wf("  if n > -1 {\n")
		wf("    r = make(%s, 0, n)\n", goType)
		wf("    for i := 0; i < n; i++ {\n")
		wf("      r = append(r, %s)\n", wrapstr(expr, cast))
		wf("    }\n")
		wf("  } else {\n")
		wf("    for %s.More() {\n", cvar)
		wf("      r = append(r, %s)\n", wrapstr(expr, cast))
		wf("    }\n")
		wf("  }\n")

	case *types.Map:
		expr, cast, err := g.decoderExpr(t.Elem(), cvar)
		if err != nil {
			return err
		}
		// note: we don't check that key type is string since we already check for that in
		// genComplexEncoder
		valueGoType := g.goTypeName(t.Elem())
		wf("  n := %s.DictHeader()\n", cvar)
		wf("  r = make(map[string]%s, n)\n", valueGoType)
		wf("  if n > -1 {\n")
		wf("    for i := 0; i < n; i++ {\n")
		wf("      k := %s.Key()\n", cvar)
		wf("      r[k] = %s\n", wrapstr(expr, cast))
		wf("    }\n")
		wf("  } else {\n")
		wf("    for %s.More() {\n", cvar)
		wf("      k := %s.Key()\n", cvar)
		wf("      r[k] = %s\n", wrapstr(expr, cast))
		wf("    }\n")
		wf("  }\n")

	// case *types.Array:
	// 	expr, cast, err := g.decoderExpr(t.Elem())
	// 	if err != nil {
	// 		return err
	// 	}
	// 	wf("  n := c.ListHeader()\n")
	// 	wf("  if n < 0 {\n")
	// 	wf("    for i := 0; c.More(); i++ {\n")
	// 	wf("      if i < %d {\n", t.Len())
	// 	wf("        r[i] = %s\n", wrapstr(expr, cast))
	// 	wf("      } else {\n")
	// 	wf("        c.Discard()\n")
	// 	wf("      }\n")
	// 	wf("    }\n")
	// 	wf("  } else {\n")
	// 	wf("    if n > %d { n = %d }\n", t.Len(), t.Len())
	// 	wf("    for i := 0; i < n; i++ {\n")
	// 	wf("      r[i] = %s\n", wrapstr(expr, cast))
	// 	wf("    }\n")
	// 	wf("  }\n")

	default:
		return ErrUnsupportedType

	} // switch typ

	wf("  return\n}\n") // end of `func {fname}(cvar ent.Decoder) ...`

	return nil
}

// —————————————————————————————————————————————————————————————————————————————————————————
// both encoding & decoding

type HelperBuilder = func(t types.Type, cvar string, b *bytes.Buffer) error

func (g *Codegen) getOrBuildTypeHelper(
	typ types.Type,
	cvar, fnamePrefix string,
	builder HelperBuilder,
) (string, error) {
	fname, err := Typemangle(g.pkg.Types, typ)
	if err != nil {
		return "", err
	}
	fname = fnamePrefix + fname
	err = g.getOrBuildHelper(fname, cvar, typ, builder)
	return fname, err
}

func (g *Codegen) getOrBuildHelper(fname, cvar string, t types.Type, builder HelperBuilder) error {
	if g.helperm == nil {
		g.helperm = map[string]error{}
	} else {
		err, ok := g.helperm[fname]
		if ok {
			return err
		}
	}
	log.Debug("codegen helper %s", fname)
	var buf bytes.Buffer
	buf.WriteString("\nfunc ")
	buf.WriteString(fname)
	err := builder(t, cvar, &buf)
	g.helperm[fname] = err
	if err == nil {
		g.helperw.Write(buf.Bytes())
	}
	return err
}

func (g *Codegen) unwrapNamedType(typ types.Type) (canonical types.Type, cast string) {
	// unwrap named type (does not include aliases, which do not need casting)
	canonical = typ
	for {
		t, ok := canonical.(*types.Named)
		if !ok {
			break
		}
		canonical = t.Underlying()
		if canonical == typ {
			break
		}
		if cast == "" {
			cast = g.goTypeName(typ)
		}
	}
	return
}

func (g *Codegen) basicCodecCall(
	t *types.Basic, cdir codecDir, mustcast bool,
) (m, cast, advice string) {
	kind := t.Kind()
	var wantadvice bool

	setcast := func(enccast string) {
		if cdir == codecDecode {
			cast = g.goTypeName(t)
		} else {
			cast = enccast
		}
	}

	switch kind {

	case types.Bool, types.UntypedBool:
		m = "Bool"

	case types.Int, types.UntypedInt, types.Int8, types.Int16, types.Int32, types.Int64,
		types.UntypedRune:
		if mustcast || kind != types.Int64 {
			setcast("int64")
		}
		m = "Int"
		wantadvice = true

	case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
		types.Uintptr, types.UnsafePointer:
		if mustcast || kind != types.Uint64 {
			setcast("uint64")
		}
		m = "Uint"
		wantadvice = true

	case types.UntypedFloat, types.Float32, types.Float64:
		if mustcast || kind != types.Float64 {
			setcast("float64")
		}
		m = "Float"
		wantadvice = true

	case types.UntypedComplex, types.Complex64, types.Complex128:
		if mustcast || kind != types.Complex128 {
			setcast("complex128")
		}
		m = "Complex"
		wantadvice = true

	case types.String, types.UntypedString:
		m = "Str"

	}
	if wantadvice {
		advice = basicKindSizeAdvice(t.Kind())
	}
	return
}

// —————————————————————————————————————————————————————————————————————————————————————————
// ent

func (g *Codegen) codegenEnt(e *EntInfo) error {
	w := g.w
	g.pushPos(e.pos)
	defer g.popPos()
	log.Info("codegen ent %q (%s.%s)", e.name, e.pkg.Name, e.sname)

	linefeed := []byte{'\n'}
	wstr := func(s string) { w.Write([]byte(s)) }
	wline := func() { w.Write(linefeed) }
	// wbyte := func(b byte) { w.Write([]byte{b}) }
	userMethods := e.getUserMethods()
	generatedMethods := map[string]bool{}

	var err error

	// methodIsUndefined checks if user has defined "name" method
	methodIsUndefined := func(name string) bool {
		return userMethods[name] == nil && !generatedMethods[name]
	}

	// methodMustBeUndefined checks if user has defined "name" method. If not, returns true.
	// If there is a definition, logs an error and returns false.
	methodMustBeUndefined := func(name, help string) bool {
		m := userMethods[name]
		if m == nil {
			if generatedMethods[name] {
				panic("trying to generate method " + name + " twice")
			}
			return true
		}
		if help != "" {
			help = " " + help
		}
		e.logSrcErr(m.pos, "ent method %s already defined for %s.%s", name, e.sname, help)
		if err == nil {
			err = fmt.Errorf("method definition conflict")
		}
		// TODO error
		return false
	}

	methodWarnIfDefinedAlt := func(name, help string) string {
		if m := userMethods[name]; m != nil {
			lname := inverseCapitalize(name)
			if lname != name {
				if help != "" {
					help = " " + help
				}
				e.logSrcWarn(m.pos,
					"ent method %s is already defined for %s. Naming the generated method %s instead.%s",
					name, e.sname, lname, help)
				help = "" // so we don't print it twice in case there's an error next
			}
			name = lname
			methodMustBeUndefined(name, help)
		} else if generatedMethods[name] {
			panic("trying to generate method " + name + " twice")
		}
		return name
	}

	funcIsUndefined := func(name string) bool {
		// TODO user functions
		return !g.generatedFunctions[name]
	}

	// compile indexes
	fieldIndexes := g.collectFieldIndexes(e.fields)

	g.f(
		"// ----------------------------------------------------------------------------\n// %s\n\n",
		e.sname)

	// // replicate original struct documentation
	// if len(e.doc) > 0 {
	//  wstr("// ")
	//  wstr(strings.Join(e.doc, "\n// "))
	//  wline()
	// }

	// variables & constants

	// // ent.Register
	// wline()
	// g.f("var _ = ent.Register(&%s{})\n", e.sname)
	// wline()

	// LoadTYPEById(s ent.Storage, id uint64) (*TYPE, error)
	fname := "Load" + e.sname + "ById"
	if funcIsUndefined(fname) {
		g.generatedFunctions[fname] = true
		g.f("// %s loads %s with id from storage\n"+
			"func %s(storage ent.Storage, id uint64) (*%s, error)\t{\n"+
			"  e := &%s{}\n"+
			"  return e, ent.LoadEntById(e, storage, id)\n"+
			"}\n\n",
			fname, e.sname,
			fname, e.sname,
			e.sname)
	}

	// FindTYPEByINDEX
	// LoadTYPEByINDEX
	for _, fx := range fieldIndexes {
		if err := g.genFindTYPEByINDEX(e, fx); err != nil {
			return err
		}
	}

	mname := "EntTypeName"
	if methodMustBeUndefined(mname, "Use tag on EntBase field instead (e.g. `typename`)") {
		generatedMethods[mname] = true
		g.f("// %s returns the ent's storage name (%q)\n"+
			"func (e %s) %s() string\t{ return %#v }\n\n",
			mname, e.name,
			e.sname, mname, e.name)
	}

	mname = "EntNew"
	if methodIsUndefined(mname) {
		generatedMethods[mname] = true
		g.f("// %s returns a new empty %s. Used by the ent package for loading ents.\n"+
			"func (e %s) %s() ent.Ent\t{ return &%s{} }\n\n",
			mname, e.sname,
			e.sname, mname, e.sname)
	}

	mname = "MarshalJSON"
	if methodIsUndefined(mname) {
		generatedMethods[mname] = true
		g.f("// %s returns a JSON representation of e. Conforms to json.Marshaler.\n"+
			"func (e *%s) %s() ([]byte, error) { return ent.JsonEncode(e) }\n\n",
			mname,
			e.sname, mname)
	}

	mname = "UnmarshalJSON"
	if methodIsUndefined(mname) {
		generatedMethods[mname] = true
		g.f("// %s populates the ent from JSON data. Conforms to json.Unmarshaler.\n"+
			"func (e *%s) %s(b []byte) error { return ent.JsonDecode(e, b) }\n\n",
			mname,
			e.sname, mname)
	}

	mname = "Create"
	if methodIsUndefined(mname) {
		generatedMethods[mname] = true
		g.f("// %s a new %s ent in storage\n"+
			"func (e *%s) %s(storage ent.Storage) error\t{ return ent.CreateEnt(e, storage) }\n",
			mname, e.name,
			e.sname, mname)
	}

	mname = "Save"
	if methodIsUndefined(mname) {
		generatedMethods[mname] = true
		g.f("// %s pending changes to whatever storage this ent was created or loaded from\n"+
			"func (e *%s) %s() error\t{ return ent.SaveEnt(e) }\n",
			mname,
			e.sname, mname)
	}

	mname = "Reload"
	if methodIsUndefined(mname) {
		generatedMethods[mname] = true
		g.f("// %s fields to latest values from storage, discarding any unsaved changes\n"+
			"func (e *%s) %s() error\t{ return ent.ReloadEnt(e) }\n",
			mname,
			e.sname, mname)
	}

	mname = "PermanentlyDelete"
	if methodIsUndefined(mname) {
		generatedMethods[mname] = true
		g.f("// %s deletes this ent from storage. This can usually not be undone.\n"+
			"func (e *%s) %s() error\t{ return ent.DeleteEnt(e) }\n",
			mname,
			e.sname, mname)
	}

	wline()

	// begin field accessor methods
	if len(e.fields) > 0 {
		wstr("// ---- field accessor methods ----\n\n")

		fieldsWithTags := []*EntField{}

		// field getters
		// func (e *ENTTYPE) Field() { return e.field }
		didGenerateGetters := false
		for _, field := range e.fields {
			if len(field.tags) > 0 {
				fieldsWithTags = append(fieldsWithTags, field)
			}

			// skip gettter if the field has a public name
			if field.uname == field.sname {
				continue
			}

			// skip if user has defined a method with the same name
			if !methodIsUndefined(field.uname) {
				continue
			}

			// sname is lower case; generate getter function
			// if the field has documentation, add it to the getter for nice godoc
			if len(field.doc) > 0 {
				wstr("// ")
				wstr(strings.Join(field.doc, "\n// "))
				wline()
			}
			g.f("func (e *%s) %s() %s\t{ return e.%s }\n",
				e.sname,
				field.uname,
				g.goTypeName(field.t.Type),
				field.sname,
			)
			generatedMethods[field.uname] = true
			didGenerateGetters = true
		}
		if didGenerateGetters {
			wline()
		}

		// field setters
		// func (e *Type) SetField() { e.field  }
		fieldSetterPrefix := "Set"
		if g.PrivateFieldSetters {
			fieldSetterPrefix = "set"
		}
		var genChangedSetters []*EntField
		for _, field := range e.fields {
			mname := fieldSetterPrefix + field.uname

			if !methodIsUndefined(mname) {
				if g.PrivateFieldSetters {
					continue
				}
				// If the user has defined a method with the same name, define a private lower-case version
				//
				// This can be useful for the author to use in composition, e.g.
				//   // user-defined
				//   func (e *Foo) SetThing(v int) {
				//     if someCondition() {
				//       e.setThing()  // call entgen-generated method
				//     }
				//   }
				//
				mname = "set" + field.uname
				if !methodIsUndefined(mname) {
					// user has defined that one too; they don't want it to be generated.
					continue
				}
			}
			g.f("func (e *%s) %s(v %s)\t{"+
				" e.%s = v;"+
				" ent.SetFieldChanged(&e.EntBase, %d)"+
				"}\n",
				e.sname, mname, g.goTypeName(field.t.Type),
				field.sname,
				field.index,
			)
			generatedMethods[mname] = true
			if isMutableRefType(field.t.Type) {
				genChangedSetters = append(genChangedSetters, field)
			}
		}

		// SetFIELDChanged(bool)
		if len(genChangedSetters) > 0 {
			wline()
			for _, field := range genChangedSetters {
				mname := fieldSetterPrefix + field.uname + "Changed"
				if methodIsUndefined(mname) {
					g.f("func (e *%s) %s()\t{ ent.SetFieldChanged(&e.EntBase, %d) }\n",
						e.sname, mname, field.index)
					generatedMethods[mname] = true
				}
			}
		}

		// EntEncode & EntDecode
		wstr("// ---- encode & decode methods ----\n\n")

		// -- EntEncode --
		mname := methodWarnIfDefinedAlt(
			"EntEncode",
			"Make sure to call entEncode from your EntEncode method",
		)
		generatedMethods[mname] = true
		g.f("\nfunc (e *%s) %s(c ent.Encoder, fields uint64) {", e.sname, mname)
		// g.f("\n\teb := &e.EntBase\n")
		for _, field := range e.fields {
			g.pushPos(field.pos)
			// Note: Rather than precomputing (1<<field.index), let the compiler apply constant
			// evaluation instead. This makes the generated code more readable.
			g.f("\tif (fields & (1 << %d)) != 0\t{", field.index)
			err := g.codegenEncodeField(field)
			wstr(" }\n")
			g.popPos()
			if err != nil {
				return err
			}
		}
		wstr("}\n")

		// -- EntDecode --
		mname = methodWarnIfDefinedAlt(
			"EntDecode",
			"Make sure to call entDecode from your EntDecode method",
		)
		generatedMethods[mname] = true
		if err := g.genEntDecode(e, mname); err != nil {
			return err
		}
	}

	// -- EntDecodePartial --
	mname = methodWarnIfDefinedAlt(
		"EntDecodePartial",
		"Make sure to call entDecodeIndexed from your EntDecodePartial method",
	)
	generatedMethods[mname] = true
	if err := g.genEntDecodePartial(e, mname); err != nil {
		return err
	}

	// EntFields
	if methodMustBeUndefined("EntFields", "") {
		g.genEntFields(e)
	}

	// data & methods for ents with indexes
	if len(fieldIndexes) > 0 {
		// -- EntIndexes --
		if methodIsUndefined("EntIndexes") {
			generatedMethods["EntIndexes"] = true
			g.f("\n// Indexes (Name, Fields, Flags)\n")
			g.f("var entIndexes_%s = []ent.EntIndex{\n", e.sname)
			for _, x := range fieldIndexes {
				var flags []string
				if (x.flags & fieldIndexUnique) != 0 {
					flags = append(flags, "ent.EntIndexUnique")
				}
				if len(flags) == 0 {
					flags = append(flags, "0")
				}
				fieldIndices := genFieldmap(e, x.fields)
				g.f("{ %#v, %s, %s },\n", x.name, fieldIndices, strings.Join(flags, "|"))
			}
			g.f("}\n\n")
			g.f("// EntIndexes returns information about secondary indexes\n")
			g.f("func (e *%s) EntIndexes() []ent.EntIndex { return entIndexes_%s }\n",
				e.sname, e.sname)
		}
	} // if len(fieldIndexes) > 0

	if log.RootLogger.Level <= log.LevelDebug {
		log.Debug("methods generated for %s:%s", e.sname, fmtMappedNames(generatedMethods))
	}

	g.scanImportsNeededForEnt(e)

	return err
}

// typePkgName returns the package name for a type that is from an external package.
// E.g:
//   package foo
//   "int" => ""
//   "foo.Thing" => ""
//   "bar.Thing" => "bar"
//   "[]bar.Thing" => "bar"
//
func (g *Codegen) typePkgName(t types.Type) string {
	if t, ok := t.(*types.Named); ok {
		if o := t.Obj(); o != nil {
			if pkg := o.Pkg(); pkg != nil && pkg != g.pkg.Types && pkg.Path() != g.pkg.Types.Path() {
				return pkg.Name()
			}
		}
	}
	return ""
}

func (g *Codegen) scanImportsNeededForEnt(e *EntInfo) {
	// collect all unique named types which has package information
	uniqueNamedTypes := make(map[*types.Named]*types.TypeName)
	for _, field := range e.fields {
		if t, ok := field.t.Type.(*types.Named); ok {
			if o := t.Obj(); o != nil {
				if o.Pkg() != nil {
					uniqueNamedTypes[t] = o
				}
			}
		}
	}

	// for each unique named type...
	ePkgPath := g.pkg.Types.Path()
	for _, o := range uniqueNamedTypes {
		pkg := o.Pkg() // note: never nil
		// log.Debug("%v\n  o.id=%v, o.name=%v, pkg.name=%s, pkg.path=%q", t,
		// 	o.Id(), o.Name(), pkg.Name(), pkg.Path())
		pkgPath := pkg.Path()
		if pkgPath != ePkgPath && pkgPath != g.entpkgPath {
			g.imports = append(g.imports, PkgImport{Path: pkgPath})
		}
	}
}

func (g *Codegen) genEntFields(e *EntInfo) {
	// entField* constants for symbolic field indices
	var fieldmap uint64
	if len(e.fields) > 0 {
		g.s("// Symbolic field indices, for use with ent.*FieldChanged methods\n")
		g.s("const (\n")
		for _, field := range e.fields {
			fieldmap |= (1 << field.index)
			g.f("  ent_%s_%s\t= %d\n", e.sname, field.sname, field.index)
		}
		g.s(")\n\n")
	}

	g.f("// EntFields returns information about %s fields\n", e.sname)
	g.f("var ent_%s_fields = ent.Fields{\n", e.sname)
	g.f("  Names: []string{\n")
	for _, field := range e.fields {
		g.f("    %#v,\n", field.name)
	}
	g.f("  },\n")
	g.f("  Fieldmap:\t0b%b,\n", fieldmap)
	g.f("}\n\n")

	g.f("// EntFields returns information about %s fields\n", e.sname)
	g.f("func (e %s) EntFields() ent.Fields { return ent_%s_fields }\n", e.sname, e.sname)
}

func genFieldmap(e *EntInfo, fields []*EntField) string {
	v := make([]string, len(fields))
	for i, field := range fields {
		v[i] = fmt.Sprintf("1<<ent_%s_%s", e.sname, field.sname)
	}
	if len(v) == 1 {
		return v[0]
	}
	return "(" + strings.Join(v, ") | (") + ")"
}

func (g *Codegen) genFindTYPEByINDEX(e *EntInfo, fx *EntFieldIndex) error {
	svar, cvar, rvar, evar, errvar, tmpvar := "s", "c", "r", "e", "err", "v"

	// package names
	var pkgnames map[string]struct{}
	for _, f := range fx.fields {
		if pkgname := g.typePkgName(f.t.Type); pkgname != "" {
			if pkgnames == nil {
				pkgnames = make(map[string]struct{})
			}
			pkgnames[pkgname] = struct{}{}
		}
	}
	// log.Debug("package names: %v", pkgnames)

	// args
	var prevGoType string
	argchunks := make([]string, 0, len(fx.fields))
	argnames := make([]string, 0, len(fx.fields))
	for i, f := range fx.fields {
		goType := g.goTypeName(f.t.Type)
		if prevGoType == goType {
			argchunks = append(argchunks[:i-1], inverseCapitalize(fx.fields[i-1].sname))
		}
		argname := inverseCapitalize(f.sname)
		for {
			if _, ok := pkgnames[argname]; !ok {
				break
			}
			argname = argname + "_"
		}
		argchunks = append(argchunks, argname+" "+goType)
		argnames = append(argnames, argname)
		prevGoType = goType
		if argname == svar {
			svar = "_" + svar
		} else if argname == cvar {
			cvar = "_" + cvar
		} else if argname == rvar {
			rvar = "_" + rvar
		} else if argname == evar {
			evar = "_" + evar
		} else if argname == errvar {
			errvar = "_" + errvar
		} else if argname == tmpvar {
			tmpvar = "_" + tmpvar
		}
	}

	// fieldIndices := genFieldmap(e, fx.fields)
	params := strings.Join(argchunks, ", ")

	var argsComment string
	if len(fx.fields) == 1 {
		argsComment = "with " + argnames[0]
	} else {
		argsComment = "matching " + strings.Join(argnames, " AND ")
	}

	// use an optimization where the index query is a single field that is a string or byte slice
	useSingleStringKeyOpt := len(fx.fields) == 1 &&
		(isStringType(fx.fields[0].t.Type) || isByteSliceType(fx.fields[0].t.Type))

	// arg0 is used by useSingleStringKeyOpt and is argnames[0] as []byte
	var arg0 string
	if useSingleStringKeyOpt {
		arg0 = argnames[0]
		if isStringType(fx.fields[0].t.Type) {
			arg0 = "[]byte(" + arg0 + ")"
		}
	}

	// both load and find needs key encoder code, so generate that up front
	var keyEncoderCode []byte
	if !useSingleStringKeyOpt {
		var b bytes.Buffer
		fmt.Fprintf(&b, "func(%s ent.Encoder) {\n", cvar)
		for i, f := range fx.fields {
			expr, err := g.genFieldEncoder(f, cvar, argnames[i])
			if err != nil {
				return err
			}
			if len(fx.fields) > 1 {
				fmt.Fprintf(&b, "    %s.Key(%#v)\n", cvar, f.name)
			}
			fmt.Fprintf(&b, "    %s\n", expr)
		}
		b.WriteString("  }")
		keyEncoderCode = b.Bytes()
	}

	//
	// Load__By__
	fname := "Load" + e.sname + "By" + capitalize(fx.name)
	if fx.IsUnique() {
		g.f("// %s loads %s %s\n", fname, e.sname, argsComment)
		g.f("func %s(%s ent.Storage, %s) (*%s, error)\t{\n", fname, svar, params, e.sname)
		g.f("  %s := &%s{}\n", evar, e.sname)
		if useSingleStringKeyOpt {
			g.f("  %s := ent.LoadEntByIndexKey(%s, %s, %#v, %s)\n",
				errvar, svar, evar, fx.name, arg0)
		} else {
			g.f("  %s := ent.LoadEntByIndex(%s, %s, %#v, %s)\n",
				errvar, svar, evar, fx.name, keyEncoderCode)
		}
		g.f("  return %s, %s\n", evar, errvar)
		g.s("}\n\n")
	} else {
		sliceCast, err := g.getEntSliceCastHelper(e)
		if err != nil {
			return err
		}
		g.f("// %s loads all %s ents %s\n", fname, e.sname, argsComment)
		g.f("func %s(%s ent.Storage, %s) ([]*%s, error)\t{\n", fname, svar, params, e.sname)
		g.f("  %s := &%s{}\n", evar, e.sname)
		if useSingleStringKeyOpt {
			g.f("  %s, %s := %s.LoadEntsByIndex(%s, %#v, %s)\n",
				rvar, errvar, svar, evar, fx.name, arg0)
		} else {
			g.f("  %s, %s := ent.LoadEntsByIndex(%s, %s, %#v, %d, %s)\n",
				rvar, errvar, svar, evar, fx.name, len(fx.fields), keyEncoderCode)
		}
		g.f("  return %s(%s), %s\n", sliceCast, rvar, errvar)
		g.s("}\n\n")
	}

	//
	// Find__By__
	fname = "Find" + e.sname + "By" + capitalize(fx.name)
	if fx.IsUnique() {
		g.f("// %s looks up %s id %s\n", fname, e.sname, argsComment)
		g.f("func %s(%s ent.Storage, %s) (uint64, error)\t{\n", fname, svar, params)
		if useSingleStringKeyOpt {
			g.f("  return ent.FindEntIdByIndexKey(%s, %#v, %#v, %s)\n", svar, e.name, fx.name, arg0)
		} else {
			g.f("  return ent.FindEntIdByIndex(%s, %#v, %#v, %s)\n",
				svar, e.name, fx.name, keyEncoderCode)
		}
		g.s("}\n\n")
	} else {
		g.f("// %s looks up %s ids %s\n", fname, e.sname, argsComment)
		g.f("func %s(%s ent.Storage, %s) ([]uint64, error)\t{\n", fname, svar, params)
		if useSingleStringKeyOpt {
			g.f("  return %s.FindEntIdsByIndex(%#v, %#v, %s)\n", svar, e.name, fx.name, arg0)
		} else {
			g.f("  return ent.FindEntIdsByIndex(%s, %#v, %#v, %d, %s)\n",
				svar, e.name, fx.name, len(fx.fields), keyEncoderCode)
		}
		g.s("}\n\n")
	}

	return nil
}

func (g *Codegen) getEntSliceCastHelper(e *EntInfo) (string, error) {
	fname := fmt.Sprintf("ent_%s_slice_cast", e.sname)
	err := g.getOrBuildHelper(fname, "c", nil,
		func(typ types.Type, cvar string, buf *bytes.Buffer) error {
			wf := func(format string, args ...interface{}) {
				fmt.Fprintf(buf, format, args...)
			}
			wf("(s []ent.Ent) []*%s {\n", e.sname)
			wf("  v := make([]*%s, len(s))\n", e.sname)
			wf("  for i := 0; i < len(s); i++ {\n")
			wf("    v[i] = s[i].(*%s)\n", e.sname)
			wf("  }\n")
			wf("  return v\n")
			wf("}\n")
			return nil
		})
	return fname, err
}

func (g *Codegen) genEntDecode(e *EntInfo, mname string) error {
	g.f("\n// %s populates fields from a decoder\n", mname)
	g.f("func (e *%s) %s(c ent.Decoder) (id, version uint64) {\n", e.sname, mname)
	g.s("  for {\n")
	g.s("    switch string(c.Key()) {\n")
	g.s("    case \"\": return\n")
	g.s("    case ent.FieldNameId:  id = c.Uint(64)\n")
	g.s("    case ent.FieldNameVersion:  version = c.Uint(64)\n")
	for _, field := range e.fields {
		g.pushPos(field.pos)
		g.f("    case %#v:\n", field.name)
		err := g.codegenDecodeField(field)
		g.popPos()
		if err != nil {
			return err
		}
	}
	g.s("    default:  c.Discard()\n")
	g.s("    }\n")
	g.s("  }\n")
	g.s("  return\n")
	g.s("}\n")
	return nil
}

func (g *Codegen) genEntDecodePartial(e *EntInfo, mname string) error {
	var indexedFields []*EntField
	for _, field := range e.fields {
		if field.storageIndex != nil {
			indexedFields = append(indexedFields, field)
		}
	}
	g.f("\n// %s is used internally by ent.Storage during updates.\n", mname)
	g.f("func (e *%s) %s(c ent.Decoder, fields uint64) (version uint64) {\n", e.sname, mname)
	g.f("  for n := %d; n > 0; {\n", len(indexedFields))
	g.s("    switch string(c.Key()) {\n")
	g.s("    case \"\": return\n")
	g.s("    case ent.FieldNameVersion: version = c.Uint(64); continue\n")
	for _, field := range indexedFields {
		g.pushPos(field.pos)
		g.f("    case %#v:\n", field.name)
		g.s("      n--\n")
		g.f("      if (fields & (1 << %d)) != 0 {\n", field.index)
		err := g.codegenDecodeField(field)
		g.s("        continue\n")
		g.s("      }\n")
		g.popPos()
		if err != nil {
			return err
		}
	}
	g.s("    }\n")
	g.s("    c.Discard()\n")
	g.s("  }\n")
	g.s("  return\n")
	g.s("}\n")
	return nil
}

// —————————————————————————————————————————————————————————————————————————————————————————

// collectFieldIndexes builds EntFieldIndex for all indexes defined by field tags.
// The returned list is sorted on name
func (g *Codegen) collectFieldIndexes(fields []*EntField) []*EntFieldIndex {
	// check field tags and pick out fields with an index
	m := make(map[string]*EntFieldIndex, len(fields))

	addIndex := func(index *EntFieldIndex) *EntFieldIndex {
		x := m[index.name]
		if x == nil {
			m[index.name] = index
			x = index
		} else {
			x.flags |= index.flags
			x.fields = append(x.fields, index.fields[0])
		}
		return x
	}

	for _, field := range fields {
		if len(field.tags) == 0 {
			continue
		}

		g.pushPos(field.pos)

		for _, tag := range field.tags {
			// tag="key=foo=bar"  =>  key="key", val="foo=bar"
			// tag="key"          =>  key="key", val="fieldname"
			key, val := tag, field.name
			if i := strings.IndexByte(key, '='); i != -1 {
				val = key[i+1:]
				key = key[:i]
			}
			key = strings.ToLower(key)

			var index *EntFieldIndex
			switch key {
			case "index":
				index = &EntFieldIndex{name: val}
			case "unique":
				index = &EntFieldIndex{name: val, flags: fieldIndexUnique}
			case "":
				// silently ignore
			default:
				g.logSrcWarn("unknown field tag %q on field %s; ignoring", tag, field.sname)
			}
			if index != nil {
				if field.storageIndex != nil {
					g.logSrcErr("multiple indexes defined for field %s", field.sname)
				} else {
					index.fields = []*EntField{field}
					field.storageIndex = addIndex(index)
				}
			}
		}
		g.popPos()
	}

	if len(m) == 0 {
		return nil
	}

	// sort by name
	indexes := make([]*EntFieldIndex, 0, len(m))
	for _, x := range m {
		indexes = append(indexes, x)
	}

	sort.Sort(EntFieldIndexes(indexes))
	return indexes
}

type EntFieldIndexes []*EntFieldIndex

func (a EntFieldIndexes) Len() int           { return len(a) }
func (a EntFieldIndexes) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a EntFieldIndexes) Less(i, j int) bool { return a[i].name < a[j].name }

// —————————————————————————————————————————————————————————————————————————————————————————

func fmtMappedNames(m map[string]bool) []byte {
	names := make([]string, 0, len(m))
	for mname := range m {
		names = append(names, mname)
	}
	sort.Strings(names)
	columns := 3

	if len(names) <= columns {
		return []byte(" " + strings.Join(names, "  "))
	}

	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 1, 1, ' ', tabwriter.TabIndent)
	for i, s := range names {
		if i%columns == 0 {
			w.Write([]byte("\n  "))
		}
		w.Write([]byte(s + "\t "))
	}
	w.Write([]byte("\n"))
	w.Flush()
	return buf.Bytes()
}

func goTypeName(t types.Type, inpkg *types.Package) string {
	return types.TypeString(t, func(pkg *types.Package) string {
		if inpkg == pkg || inpkg.Path() == pkg.Path() {
			return ""
		}
		return pkg.Name()
	})
}

// wrapstr("foo", "bar") => "bar(foo)"
// wrapstr("foo", "") => "foo"
func wrapstr(s, wrapper string) string {
	if wrapper != "" {
		s = wrapper + "(" + s + ")"
	}
	return s
}

func basicKindSizeAdvice(kind types.BasicKind) string {
	var bitsize int
	switch kind {
	case types.Int8, types.Uint8:
		bitsize = 8
	case types.Int16, types.Uint16:
		bitsize = 16
	case types.Int32, types.Uint32, types.Float32:
		bitsize = 32
	case types.Int64, types.Uint64, types.Float64:
		bitsize = 64
	case types.UntypedInt, types.Int, types.Uint:
		bitsize = intSize
	case types.Uintptr, types.UnsafePointer:
		bitsize = uintptrSize
	default:
		return ""
	}
	return strconv.FormatUint(uint64(bitsize), 10)
}

func filterFieldsByIndex(fields []*EntField, fieldmap uint64) []*EntField {
	n := bits.PopcountUint64(fieldmap)
	fields2 := make([]*EntField, 0, n)
	for _, f := range fields {
		if (fieldmap & (1 << f.index)) != 0 {
			fields2 = append(fields2, f)
		}
		n--
		if n == 0 {
			break
		}
	}
	return fields2
}

func isStringType(typ types.Type) bool {
	t, ok := typ.(*types.Basic)
	return ok && t.Kind() == types.String
}

func isByteSliceType(typ types.Type) bool {
	if t, ok := typ.(*types.Slice); ok {
		et, ok := t.Elem().(*types.Basic)
		return ok && et.Kind() == types.Uint8
	}
	return false
}