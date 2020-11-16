package main

import (
	"errors"
	"flag"
	"fmt"
	"go/format"
	"go/scanner"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/rsms/go-log"
	"golang.org/x/tools/go/packages"
)

// cli options
var (
	opt_outfile   string
	opt_nofmt     bool
	opt_filter    string
	opt_filter_re *regexp.Regexp
	opt_verbose   bool
	opt_vverbose  bool
	opt_entpkg    string = "github.com/rsms/ent"

	opt_version bool
	opt_help    bool
)

var ErrUnsupportedType = errors.New("unsupported type")

const (
	intSize     = int(32 << (^uint(0) >> 63)) // bits of int on target platform
	uintptrSize = int(32 << (^uintptr(0) >> 63))
)

type Package = packages.Package

var (
	entTypeNameRegexp  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	entFieldNameRegexp = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

func parseopts() []string {
	versionstring := fmt.Sprintf("entgen %s", VERSION)

	// parse cli args
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [<srcdir> ...]\noptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.BoolVar(&opt_help, "h, -help", false, "Show help and exit")
	flag.BoolVar(&opt_version, "version", false, `Print "`+versionstring+`" and exit`)
	flag.BoolVar(&opt_verbose, "v", false, "Verbose logging")
	flag.BoolVar(&opt_vverbose, "debug", false, "Debug logging (implies -v)")
	flag.StringVar(&opt_outfile, "o", "ents.gen.go",
		`Filename of generated go code, relative to <srcdir>. Use "-" for stdout.`)
	flag.BoolVar(&opt_nofmt, "nofmt", false, `Disable "gofmt" formatting of generated code`)
	flag.StringVar(&opt_filter, "filter", "",
		`Only process go struct types which name matches the provided regular expression`)
	flag.StringVar(&opt_entpkg, "entpkg", opt_entpkg, `Import path of ent package`)

	flag.Parse()

	// maybe just print version and exit
	if opt_version {
		println(versionstring)
		os.Exit(0)
	}

	// configure logging
	if opt_vverbose {
		log.RootLogger.Level = log.LevelDebug
	} else if opt_verbose {
		log.RootLogger.Level = log.LevelInfo
	} else {
		log.RootLogger.Level = log.LevelWarn
	}
	log.RootLogger.SetWriter(os.Stderr)
	log.RootLogger.EnableFeatures(log.FSync)
	log.RootLogger.DisableFeatures(log.FTime | log.FPrefixInfo)
	log.Debug("%s (filter=%#v nofmt=%#v o=%#v v=%#v vv=%#v)",
		versionstring,
		opt_filter,
		opt_nofmt,
		opt_outfile,
		opt_verbose,
		opt_vverbose,
	)

	// check & verify options
	showUsageAndExit := func(exitCode int) {
		if exitCode == 0 {
			flag.Usage()
		} else {
			fmt.Fprintf(os.Stderr, "See %s -h for help\n", os.Args[0])
		}
		exit(exitCode)
	}
	if opt_help {
		showUsageAndExit(0)
	}
	args := flag.Args()
	if len(args) == 0 {
		// default to "." if no <srcdir> is given
		args = append(args, ".")
	}
	if len(args) > 1 && opt_outfile != "-" && filepath.IsAbs(opt_outfile) {
		fmt.Fprintf(os.Stderr,
			"%s: -o must name a relative filename when multiple <srcdir> are provided\n",
			os.Args[0])
		showUsageAndExit(1)
	}

	// compile filter regexp
	if opt_filter != "" && opt_filter != "*" && opt_filter != ".*" {
		var err error
		if opt_filter_re, err = regexp.Compile(opt_filter); err != nil {
			die("invalid regular expression in -filter: %s", err.Error())
		}
	}

	return args
}

func exit(status int) {
	log.Sync()
	os.Exit(status)
}

func printerr(format string, arg ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s: "+format+"\n", append([]interface{}{os.Args[0]}, arg...)...)
}

func die(format string, arg ...interface{}) {
	printerr(format, arg...)
	exit(1)
}

func main() {
	srcdirs := parseopts()
	errs := handleSrcDirs(srcdirs)
	if len(errs) > 0 {
		for _, err := range errs {
			printerr("%s", err)
		}
		exit(1)
	}
	log.Sync()
}

// call handleSrcDir for every srcdir, concurrently if len(srcdirs)>1
func handleSrcDirs(srcdirs []string) []error {
	var errors []error
	if len(srcdirs) == 1 {
		err := handleSrcDir(srcdirs[0])
		if err != nil {
			errors = append(errors, err)
		}
	} else {
		ch := make(chan error)
		for _, srcdir := range srcdirs {
			go func(srcdir string) {
				defer func() {
					if r := recover(); r != nil {
						ch <- fmt.Errorf("panic: %v", r)
					}
				}()
				ch <- handleSrcDir(srcdir)
			}(srcdir)
		}
		for range srcdirs {
			err := <-ch
			if err != nil {
				errors = append(errors, err)
			}
		}
	}
	return errors
}

// sortable list of ents
type EntInfoList []*EntInfo

func (a EntInfoList) Len() int           { return len(a) }
func (a EntInfoList) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a EntInfoList) Less(i, j int) bool { return a[i].name < a[j].name }

// main entry point for a single srcdir
func handleSrcDir(srcdir string) error {
	log.Debug("processing directory %q", srcdir)

	// outfile (we check for "-" before we use this)
	dstfile := filepath.Join(srcdir, opt_outfile)

	// parse
	var dstfile1 string
	if opt_outfile != "-" {
		dstfile1 = dstfile
	}
	entsByName, pkg, err := scanDir(srcdir, dstfile1)
	if err != nil {
		return err
	}

	// did we find any ents?
	if len(entsByName) == 0 {
		if opt_outfile == "-" {
			return nil
		}
		// do we need to remove a previously-generated file?
		fd, err := os.Open(dstfile)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Warn("open %q: %v; you man need to manually remove or edit this file", dstfile, err)
			}
			return nil
		}
		buf := make([]byte, len(generatedByHeaderPrefix))
		_, err = fd.Read(buf)
		fd.Close()
		if err != nil || string(buf) != generatedByHeaderPrefix {
			// leave the file be
			log.Debug("leaving %q as it does not appears to be generated by me")
			return nil
		}
		// we can safely remove the file
		if err := os.Remove(dstfile); err != nil {
			return fmt.Errorf("failed to remove %q: %v", dstfile, err)
		}
		log.Info("removed %s", dstfile)
		return nil
	}

	// build an ordered list of ents from the unordered map
	ents := EntInfoList(make([]*EntInfo, len(entsByName)))
	i := 0
	for _, ent := range entsByName {
		ents[i] = ent
		i++
	}
	sort.Sort(ents)

	// codegen
	g := NewCodegen(pkg, srcdir, opt_entpkg)
	for _, ei := range ents {
		g.w.Write([]byte{'\n'})
		if err := g.codegenEnt(ei); err != nil {
			return err
		}
	}
	if log.RootLogger.Level <= log.LevelDebug {
		log.Debug("functions generated:%s", fmtMappedNames(g.generatedFunctions))
	}

	// gofmt output
	gosrc := g.Finalize()
	if !opt_nofmt {
		gosrcin := gosrc
		gosrc, err = format.Source(gosrc)
		if err != nil {
			if log.RootLogger.Level <= log.LevelDebug {
				// log generated code with numbered lines
				log.Debug("gofmt error:\n------------------------\n%s", fmtSourceCode(gosrcin, err))
			}
			return fmt.Errorf("gofmt: %v\n", err)
		}
	}

	// write output
	if opt_outfile == "-" {
		os.Stdout.Write(gosrc)
	} else {
		// log.Debug("—————————————————————— output ————————————————————\n%s\n", gosrc)
		fd, err := os.Create(dstfile)
		if err != nil {
			return err
		}
		fd.Write(gosrc)
		err = fd.Close()
		log.Info("wrote code for %d ents to %s", len(ents), dstfile)
	}

	return err
}

func fmtSourceCode(src []byte, err error) string {
	issueLineNumbers := map[int]*scanner.Error{}
	if e, ok := err.(scanner.ErrorList); ok {
		//scanner.ErrorList
		e.RemoveMultiples() // sorts and remove all but the first error per line
		issueLineNumbers = make(map[int]*scanner.Error, e.Len())
		for _, err := range e {
			issueLineNumbers[err.Pos.Line] = err
		}
	}
	lines := strings.Split(string(src), "\n")
	w := len(fmt.Sprintf("%d", len(lines)))
	for i, line := range lines {
		marker := "  "
		lineno := i + 1
		err := issueLineNumbers[lineno]
		if err != nil {
			marker = "> "
		}
		line = fmt.Sprintf("%s%*d  %s", marker, w, lineno, line)
		if err != nil {
			ind := fmt.Sprintf("\n  %*s  ", w, "")
			if err.Pos.Column > 0 {
				line += fmt.Sprintf("%s%*s", ind, err.Pos.Column, "^")
			}
			line += fmt.Sprintf("%s%s", ind, err.Msg)
			line += ind
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
