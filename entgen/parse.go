package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rsms/go-log"
	"golang.org/x/tools/go/packages"
)

type EntInfo struct {
	pkg          *Package
	srcdir       string
	pos          token.Pos
	sname        string
	name         string
	nameTagPos   token.Pos // position of "name" in "ent.EntBase `name`"
	file         *token.File
	doc          []string
	fields       []*EntField
	fieldsByName map[string]*EntField
	userMethods  map[string]*EntMethod // cached value for getUserMethods()
}

func (e *EntInfo) logSrcErr(pos token.Pos, format string, args ...interface{}) {
	logSrcErr(e.srcdir, e.pkg, pos, format, args...)
}

func (e *EntInfo) logSrcWarn(pos token.Pos, format string, args ...interface{}) {
	logSrcWarn(e.srcdir, e.pkg, pos, format, args...)
}

type EntField struct {
	ent   *EntInfo // owning EntInfo
	index int      // field index
	sname string   // name in struct
	uname string   // upper cased version of sname
	name  string   // name used for storage
	tags  EntFieldTags
	t     EntFieldType
	doc   []string
	pos   token.Pos

	storageIndex *EntFieldIndex
}

type EntFieldType struct {
	types.Type
	literal string
	pos     token.Pos
}

type EntMethod struct {
	obj types.Object
	pos token.Pos
}

type fieldIndexFlags int

const (
	fieldIndexUnique = 1 << iota
)

type EntFieldIndex struct {
	index  int // in the generated entIndex_TYPE table
	name   string
	fields []*EntField
	flags  fieldIndexFlags
}

type EntFieldTags []string

func (tags EntFieldTags) Has(tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (fx *EntFieldIndex) IsUnique() bool { return (fx.flags & fieldIndexUnique) != 0 }

// getUserMethods returns a map of user-defined methods on the type
func (e *EntInfo) getUserMethods() map[string]*EntMethod {
	if e.userMethods != nil {
		return e.userMethods
	}

	e.userMethods = make(map[string]*EntMethod)

	handleMethod := func(m *types.Selection) {
		o := m.Obj()

		// ignore unnamed methods [rsms: when does this happen?]
		name := o.Id()
		if name == "" {
			return
		}

		// ignore methods from other packages, which comes from composition
		if o.Pkg().Path() != e.pkg.PkgPath {
			return
		}

		// Note: This used to be needed, before the "+build !entgen" trick.
		// // ignore method generated by a previous call to entgen
		// p := e.pkg.Fset.Position(o.Pos())
		// if filepath.Base(p.Filename) == filepath.Base(opt_outfile) {
		// 	return
		// }

		// log.Debug("- %s", m)
		// log.Debug("  Recv() %v", m.Recv())
		// log.Debug("  Obj() %q %v", name, o.Type())
		// log.Debug("  Obj().Pos() %v", e.pkg.Fset.Position(o.Pos()))

		e.userMethods[name] = &EntMethod{
			obj: o,
			pos: o.Pos(),
		}
	}

	// existing methods
	// stype := pkg.TypesInfo.TypeOf(st)
	stype := e.pkg.Types.Scope().Lookup(e.sname).Type()

	// methods with e receiver as pointer
	methods := types.NewMethodSet(types.NewPointer(stype)) // *types.MethodSet
	for i := 0; i < methods.Len(); i++ {
		handleMethod(methods.At(i))
	}

	// methods with e receiver without pointer
	methods = types.NewMethodSet(stype)
	for i := 0; i < methods.Len(); i++ {
		m := methods.At(i)
		// ignore methods from the loop above
		if _, ok := e.userMethods[m.Obj().Id()]; ok {
			continue
		}
		handleMethod(m)
	}

	// Note: this didn't work:
	// o, i, ind := types.LookupFieldOrMethod(e.t, true, e.pkg.Types, "Meow")
	// log.Debug("o, i, ind = %v, %v, %v", o, i, ind)

	return e.userMethods
}

func buildEntInfo(
	srcdir string,
	pkg *Package,
	file *token.File,
	ts *ast.TypeSpec,
	st *ast.StructType,
	decl *ast.GenDecl,
) (*EntInfo, error) {
	// note: this function is called after verifying that
	// - st.Fields.List is len() > 0
	// - st.Fields.List[0] is known to be the `ent.EntBase` field.
	//

	e := &EntInfo{
		pkg: pkg,
		// t:            pkg.TypesInfo.TypeOf(st).(*types.Struct),
		pos:          st.Pos(),
		srcdir:       srcdir,
		sname:        ts.Name.Name,
		file:         file,
		doc:          parseCommentGroup(decl.Doc),
		fieldsByName: make(map[string]*EntField, len(st.Fields.List)-1),
	}

	relfilename := relfile(srcdir, file.Name())
	log.Debug("processing type %s.%s in %s", pkg.Name, e.sname, relfilename)

	// EntBase
	field := st.Fields.List[0]
	tags := parseFieldTags(field.Tag)
	if len(tags) == 0 {
		// no `typename` tag
		logSrcErr(srcdir, pkg, field.Type.Pos(),
			"missing `name` tag for EntBase field in %s", e.sname)
		return nil, fmt.Errorf("missing `name` tag")
	}
	e.nameTagPos = field.Tag.ValuePos
	e.name = tags[0]
	if !entTypeNameRegexp.MatchString(e.name) {
		logSrcErr(srcdir, pkg, field.Tag.ValuePos,
			"invalid ent type name %q; does not match regexp %v", e.name, entTypeNameRegexp)
		return nil, fmt.Errorf("invalid ent type name")
	}

	log.Info("parsing ent %q (%s.%s) in %s", e.name, pkg.Name, e.sname, relfilename)

	nfields := len(st.Fields.List)
	if nfields == 1 {
		log.Warn("ent type %q (%s.%s) does not have any fields", e.name, pkg.Name, e.sname)
	}

	fieldIndex := 0
	for i := 1; i < nfields; i++ {
		field := st.Fields.List[i]
		var tags []string
		if field.Tag != nil {
			tags = parseFieldTags(field.Tag)
		}
		typ := EntFieldType{
			Type:    pkg.TypesInfo.TypeOf(field.Type),
			literal: fmt.Sprint(field.Type),
			pos:     field.Type.Pos(),
		}

		// field.Names examples:
		//   type Foo struct {
		//     Hello          // field.Names=[]
		//     foo int        // field.Names=["foo"]
		//     bar, baz bool  // field.Names=["bar", "baz"]
		//   }
		//
		fieldNames := field.Names

		// infer name from type
		if len(fieldNames) == 0 {
			name := typ.Type.String()
			idx := strings.LastIndexByte(name, '.')
			if idx != -1 {
				name = name[idx+1:]
			}
			fieldNames = append(fieldNames, &ast.Ident{
				NamePos: field.Type.Pos(),
				Name:    name,
			})
			log.Debug("inferred name of field %d in %s.%s of type %s as %q",
				i, pkg.Name, e.sname, typ.literal, fieldNames[0].Name)
		}

		// read names, possibly dequeued from tags
		names := make([]string, len(fieldNames))
		for j, nameIdent := range fieldNames {
			var name string
			name, tags = selectEntFieldName(nameIdent.Name, tags)
			names[j] = name
		}

		// for each name
		for j, nameIdent := range fieldNames {
			name := names[j]
			if name == "" {
				log.Debug("skipping ignored field %s.%s (name tag \"-\")", e.sname, nameIdent.Name)
				// ignore field
				continue
			}

			if j > 0 {
				tags2 := make([]string, len(tags))
				copy(tags2, tags)
				tags = tags2
			}

			f := &EntField{
				ent:   e,
				index: fieldIndex,
				tags:  EntFieldTags(tags),
				name:  name,
				uname: capitalize(nameIdent.Name),
				sname: nameIdent.Name,
				t:     typ,
				doc:   parseCommentGroup(field.Doc),
				pos:   nameIdent.NamePos,
				// Note: We intentionally ignore field.Comment which is usually used for comments,
				// not documentation (trailing comment e.g. "foo int // magnitude")
			}

			fieldIndex++

			// best source pos for ent field name
			pos := f.pos
			if field.Tag != nil && nameIdent.Name != name {
				pos = field.Tag.ValuePos
			}

			// check for duplicate field names
			if f2 := e.fieldsByName[name]; f2 != nil {
				logSrcErr(srcdir, pkg, pos, "%s.%s: Duplicate field name %q", e.sname, f.sname, name)
				logSrcErr(srcdir, pkg, f2.pos, "%s.%s: Other field name %q", e.sname, f2.sname, name)
				return nil, fmt.Errorf("duplicate field names")
			}

			// verify field name
			if !entFieldNameRegexp.MatchString(name) {
				logSrcErr(srcdir, pkg, pos,
					"invalid ent field name %q; does not match regexp %v", name, entFieldNameRegexp)
				return nil, fmt.Errorf("invalid ent field name")
			}

			// store
			e.fieldsByName[name] = f
			e.fields = append(e.fields, f)
			// fmt.Printf("%s   \t %v %#v\n", name, typ, typ)
		}

		// // field comment
		// if field.Comment != nil && len(field.Comment.List) > 0 {
		//  for _, c := range field.Comment.List {
		//    fmt.Printf("          %s\n", c.Text)
		//  }
		// }
	}
	return e, nil
}

func selectEntFieldName(name string, tags []string) (string, []string) {
	if len(tags) > 0 {
		tag0 := tags[0]
		tags = tags[1:]
		if tag0 != "" {
			name = tag0
			if name == "-" {
				name = ""
			}
			return name, tags
		}
	}
	// no explicit field name; use Go field name, converting first letter to lower case if needed
	r, z := utf8.DecodeRuneInString(name)
	if z > 0 /* && unicode.IsUpper(r) always upper; we check before call */ {
		name = string(unicode.ToLower(r)) + name[z:]
	}
	return name, tags
}

// Examples:
//   `ent:"bob"` => ["bob"]
//   `ent:",foo, bar baz,lolcat " x:"y"` => ["", "foo", "bar baz", "lolcat"]
//   `ent:"foo" json:"bar"` => ["foo"]
//   `json:"cat"` => ["cat"]
//   `json:"-"` => []
//
func parseFieldTags(tag *ast.BasicLit) []string {
	if tag == nil || len(tag.Value) == 0 {
		return nil
	}
	s := strings.TrimSpace(trimSyntaxString(tag.Value))
	if len(s) == 0 {
		return nil
	}
	if strings.IndexByte(s, ':') == -1 {
		// no keys; single tag
		return []string{s}
	}
	st := reflect.StructTag(s)
	tags := splitCommaSeparated(st.Get("ent"))
	if len(tags) == 0 {
		jsontags := splitCommaSeparated(st.Get("json"))
		if len(jsontags) > 0 && jsontags[0] != "-" {
			tags = jsontags[:1]
		}
	}
	return tags
}

// e.g. ` some\`thing` => some`thing
// e.g. "lol\"cat\""   => lol"cat"
func trimSyntaxString(s string) string {
	c := s[0]
	s = s[1 : len(s)-1]
	switch c {
	case '`':
		s = strings.ReplaceAll(s, "\\`", "`")
	case '"':
		s = strings.ReplaceAll(s, `\"`, `"`)
	}
	return s
}

// E.g. ",foo, bar baz,lolcat " => ["", "foo", "bar baz", "lolcat"]
func splitCommaSeparated(s string) []string {
	if len(s) == 0 {
		return nil
	}
	tags := strings.Split(s, ",")
	for i, s := range tags {
		tags[i] = strings.TrimSpace(s)
	}
	return tags
}

func parseCommentGroup(doc *ast.CommentGroup) (lines []string) {
	if doc != nil && len(doc.List) > 0 {
		for _, c := range doc.List {
			var s string
			if strings.HasPrefix(c.Text, "//") {
				s = strings.TrimSpace(c.Text[2:])
			} else if strings.HasPrefix(c.Text, "/*") {
				s = c.Text[2:]
				if strings.HasSuffix(s, "*/") {
					s = s[:len(s)-2]
				}
				s = strings.Trim(s, "*\t\r\n ")
			}
			if len(s) > 0 {
				lines = append(lines, s)
			}
		}
	}
	return lines
}

func logSrcErr(srcdir string, pkg *Package, pos token.Pos, format string, args ...interface{}) {
	p := pkg.Fset.Position(pos)
	srcfile := relfile(srcdir, p.Filename)
	log.Error("%s:%d:%d: "+format, append([]interface{}{srcfile, p.Line, p.Column}, args...)...)
}

func logSrcWarn(srcdir string, pkg *Package, pos token.Pos, format string, args ...interface{}) {
	p := pkg.Fset.Position(pos)
	srcfile := relfile(srcdir, p.Filename)
	log.Warn("%s:%d:%d: "+format, append([]interface{}{srcfile, p.Line, p.Column}, args...)...)
}

func scanDir(srcdir, dstfile string) (entsByName map[string]*EntInfo, pkg *Package, err error) {
	config := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo,

		Dir: srcdir,

		// This is a neat trick we use to avoid parsing code previously generated by entgen.
		// When we generate code, we add a "+build !entgen" directive to the file.
		BuildFlags: []string{"-tags=entgen"},
	}
	pkgs, err := packages.Load(config, srcdir)
	if err != nil {
		return
	}

	// Note: There's an error that may occur here with the following error message:
	//   internal error: nil Pkg importing "bytes" from "github.com/rsms/foo/cmd/foo"
	// The issue has to do with binary compatibilit somehow and this may be needed:
	//   go get -u all
	//

	if len(pkgs) == 0 {
		return nil, nil, nil
	}

	pkg = pkgs[0]

	if len(pkgs) > 1 {
		err = fmt.Errorf("multiple packages in directory %q", srcdir)
		return
	}

	log.Info("scanning package %q in directory %s", pkg.Name, srcdir)

	// report errors
	if len(pkg.Errors) > 0 {
		// reportScanErrors returns an error in case of syntax errors (show-stopper)
		if err = reportScanErrors(srcdir, pkg); err != nil {
			err = fmt.Errorf("scan error")
			return
		}
	}

	// // imports
	// for ident, impkg := range pkg.Imports { // map[string]*Package
	//  fmt.Printf("import %q %v\n", ident, impkg)
	// }

	// // TypesInfo
	// for ident, def := range pkg.TypesInfo.Defs {
	//  if ident.Name != "Account" && ident.Name != "ent" {
	//    continue
	//  }
	//  fmt.Printf("THINGY %q\n", ident.Name)
	//  if typename, ok := def.(*types.TypeName); ok && typename.Exported() {
	//    fmt.Printf("  %v => %T %v\n", ident, typename, def)
	//    fmt.Printf("  typename.Name %v\n", typename.Name())
	//    fmt.Printf("  typename.Type %v\n", typename.Type())
	//  }
	// }

	entsByName = make(map[string]*EntInfo)

	// filter
	files := make([]*ast.File, 0, len(pkg.Syntax))
	var dstfileabs string
	if dstfile != "" {
		dstfileabs, _ = filepath.Abs(dstfile)
	}
	for _, file := range pkg.Syntax {
		filename := pkg.Fset.File(file.Pos()).Name()
		if filename == dstfileabs {
			log.Debug("scanner ignoring outfile %s", dstfile)
			// don't consider a file we generated outselves
			continue
		}
		files = append(files, file)
	}

	// fan out-in channel
	ch := make(chan struct {
		ents []*EntInfo
		err  error
	})

	// fan out
	for _, file := range files {
		go func(file *ast.File) {
			ents, err := scanFile(srcdir, pkg, file)
			ch <- struct {
				ents []*EntInfo
				err  error
			}{ents, err}
		}(file)
	}

	// fan in
	err = nil
	for range files {
		r := <-ch
		if r.err != nil && err == nil {
			err = r.err
		}
		if err != nil {
			continue
		}
		for _, e := range r.ents {
			// check & register ent name
			if other := entsByName[e.name]; other != nil {
				logSrcErr(srcdir, pkg, e.nameTagPos,
					"Duplicate ent name %q by type %s.", e.name, e.sname)
				logSrcErr(srcdir, pkg, other.nameTagPos,
					"Previous definition of %q by type %s.", e.name, other.sname)
				if err == nil {
					err = fmt.Errorf("semantic error")
				}
			} else {
				entsByName[e.name] = e
			}
		}
	}

	return
}

func scanFile(srcdir string, pkg *Package, file *ast.File) ([]*EntInfo, error) {
	var entInfos []*EntInfo
	fset := pkg.Fset // *token.FileSet

	tfile := fset.File(file.Pos())

	if log.RootLogger.Level <= log.LevelDebug {
		filename := relfile(srcdir, tfile.Name())
		log.Debug("scanning file %s of package %s", filename, pkg.Name)
	}

	// if file.Scope == nil {
	//  return nil, nil
	// }

	// fmt.Printf("\nFile %s\n", fset.File(file.Pos()).Name())

	// fmt.Printf("  Imports:\n")
	// // imports := map[string]string{}
	// for _, im := range file.Imports {
	//  if im.Name != nil {
	//    fmt.Printf("    import %s as %s\n", im.Path.Value, im.Name)
	//  } else {
	//    fmt.Printf("    import %s\n", im.Path.Value)
	//  }
	// }

	for _, decl := range file.Decls {
		d, ok := decl.(*ast.GenDecl)
		if !ok || d.Tok != token.TYPE {
			continue
		}

		// // definition comment
		// if d.Doc != nil && len(d.Doc.List) > 0 {
		//  for _, c := range d.Doc.List {
		//    fmt.Printf("  %s\n", c.Text)
		//  }
		// }

		for _, spec := range d.Specs {
			ts, st := astSpecIsEntTypeDef(spec)
			if ts == nil {
				continue
			}

			// filter
			if opt_filter_re != nil && !opt_filter_re.MatchString(ts.Name.Name) {
				log.Debug("skipping %s.%s; doesn't pass filter %q", pkg.Name, ts.Name.Name, opt_filter_re)
				continue
			}

			// note: at this point st.Fields.List is guaranteed to be len()>0 and not nil.
			// Also, st.Fields.List[0] is known to be the `ent.EntBase` field.
			entInfo, err := buildEntInfo(srcdir, pkg, tfile, ts, st, d)
			if err != nil {
				return nil, err
			}
			entInfos = append(entInfos, entInfo)
		}
	}
	return entInfos, nil
}

func astSpecIsEntTypeDef(spec ast.Spec) (*ast.TypeSpec, *ast.StructType) {
	ts, ok := spec.(*ast.TypeSpec)
	if !ok {
		return nil, nil
	}
	st, ok := ts.Type.(*ast.StructType)
	if !ok || st.Fields == nil || len(st.Fields.List) == 0 {
		return nil, nil
	}

	// `ent.EntBase`
	field0 := st.Fields.List[0]
	field0t, ok := field0.Type.(*ast.SelectorExpr)
	if !ok {
		return nil, nil
	}
	if field0t.Sel.Name != "EntBase" {
		return nil, nil
	}
	xident, ok := field0t.X.(*ast.Ident)
	if !ok {
		return nil, nil
	}

	// Note: xident is "ent" but xident.Obj==nil; we can't tell what package it is from.
	// file.Scope.Lookup(xident.Name) also yields nil.
	// So, we resort to assuming that the import name is "ent".
	// TODO: investigate further, if there's a way to look up xident.Name to package name.
	if xident.Name != "ent" {
		return nil, nil
	}
	return ts, st
}

// —————————————————————————————————————————————————————————————————————————————————————————
// error reporting

func reportScanErrors(srcdir string, pkg *Package) error {
	// called by scanDir

	// Note: Parse errors are normal since with an ent definition like this:
	//   type Foo struct {
	//     ent.EntBase `foo`
	//     name string
	//   }
	// Some code might include
	//   foo.GetName()
	// Where GetName does not exist since its generated by entgen.
	//

	// first report any non-type errors (syntax or io); these are show-stoppers
	var err error
	ntypeErrs := 0
	for _, e := range pkg.Errors {
		if e.Kind == packages.TypeError {
			ntypeErrs++
			continue
		}
		if err == nil {
			err = e
		}
		fmt.Fprintf(os.Stderr, "%s\n", fmtPackagesError(srcdir, e, true))
	}
	if err != nil {
		return err
	}

	// report type errors
	if ntypeErrs > 0 && log.RootLogger.Level <= log.LevelDebug {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%d type error(s) in package %s: (%q)", ntypeErrs, pkg.Name, srcdir)
		for _, e := range pkg.Errors {
			if e.Kind != packages.TypeError {
				continue
			}
			fmt.Fprintf(&sb, "\n%s", fmtPackagesError(srcdir, e, false))
		}
		log.Debug("%s", sb.String())
	}

	return nil
}

func fmtPackagesError(srcdir string, e packages.Error, showkind bool) string {
	i := strings.IndexByte(e.Pos, ':')
	srcfile := e.Pos
	srcloc := ""
	if i != -1 {
		srcfile = e.Pos[:i]
		srcloc = e.Pos[i:]
	}
	var kind string
	if showkind {
		kind = packagesErrorKindName(e.Kind) + ": "
	}
	if srcfile == "" && srcloc == "" {
		return fmt.Sprintf("%s: %s%s", srcdir, kind, e.Msg)
	}
	srcfile = relfile(srcdir, srcfile)
	return fmt.Sprintf("%s%s: %s%s", relfile(srcdir, srcfile), srcloc, kind, e.Msg)
}

func packagesErrorKindName(kind packages.ErrorKind) string {
	switch kind {
	case packages.ListError:
		return "list error"
	case packages.ParseError:
		return "parse error"
	case packages.TypeError:
		return "type error"
	}
	return "error"
}

// —————————————————————————————————————————————————————————————————————————————————————————
// misc

// relfile returns file as relative to srcdir if file is rooted in dir,
// otherwise file is returned verbatim.
func relfile(dir, file string) string {
	absfile, err := filepath.Abs(file)
	if err != nil && absfile == "" {
		absfile = file
	}
	absdir, err := filepath.Abs(dir)
	if err != nil && absdir == "" {
		absdir = dir
	}
	if strings.HasPrefix(absfile, absdir) {
		return dir + absfile[len(absdir):]
	}
	return file
}

// capitalize makes the first rune upper case, e.g.
//   "foo" => "Foo"
//   "Foo" => "Foo"
//   "fooBar" => "FooBar"
//   "foo_bar" => "Foo_bar"
//
func capitalize(name string) string {
	r, z := utf8.DecodeRuneInString(name)
	if z > 0 && !unicode.IsUpper(r) {
		name = string(unicode.ToUpper(r)) + name[z:]
	}
	return name
}

// inverseCapitalize makes the first rune lower case, e.g.
//   "Foo" => "foo"
//   "foo" => "foo"
//   "FooBar" => "fooBar"
//   "Foo_bar" => "foo_bar"
//
func inverseCapitalize(name string) string {
	r, z := utf8.DecodeRuneInString(name)
	if z > 0 && !unicode.IsLower(r) {
		name = string(unicode.ToLower(r)) + name[z:]
	}
	return name
}
