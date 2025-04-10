/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package cc

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcPump "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/pump"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
	commonUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"

	"github.com/google/shlex"
	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/unicode"
)

func getEnv(n string) string {
	return os.Getenv(n)
}

func hasSpace(s string) bool {
	if s == "" {
		return false
	}

	for _, v := range s {
		if v == ' ' {
			return true
		}
	}

	return false
}

func checkCharset(rawBytes []byte) (string, error) {
	detector := chardet.NewTextDetector()
	charset, err := detector.DetectBest(rawBytes)
	if err != nil {
		return "", err
	}

	return charset.Charset, nil
}

func checkResponseFileCharset(f string) (string, error) {
	data, err := ioutil.ReadFile(f)
	if err != nil {
		return "", err
	}

	return checkCharset(data)
}

func readBom(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		//os.Exit(1)
		return "", err
	}
	defer func() {
		_ = f.Close()
	}()

	dec := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
	scn := bufio.NewScanner(dec.Reader(f))
	data := ""
	for scn.Scan() {
		data = data + scn.Text()
	}
	if err := scn.Err(); err != nil {
		return "", err
	}

	return data, nil
}

func readUtf8(filename string) (string, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// return compile options and source files
func readResponse(f, dir string) (string, string, error) {
	newf := f
	if !dcFile.Stat(newf).Exist() {
		// try with dir
		tempf, _ := filepath.Abs(filepath.Join(dir, newf))
		if !dcFile.Stat(tempf).Exist() {
			return "", "", fmt.Errorf("%s or %s dose not exist", newf, tempf)
		} else {
			newf = tempf
		}
	}

	charset, err := checkResponseFileCharset(newf)
	if err != nil {
		return "", "", err
	}

	data := ""
	if charset == "UTF-16LE" {
		data, err = readBom(newf)
	} else {
		data, err = readUtf8(newf)
	}
	if err != nil {
		return "", "", err
	}

	if data == "" {
		return "", "", fmt.Errorf("%s is empty", newf)
	}

	return data, newf, nil
}

// replace which next is not in nextExcludes
func replaceWithNextExclude(s string, old byte, new string, nextExcludes []byte) string {
	if s == "" {
		return ""
	}

	if len(nextExcludes) == 0 {
		return strings.Replace(s, string(old), new, -1)
	}

	targetslice := make([]byte, 0, 0)
	nextexclude := false
	totallen := len(s)
	for i := 0; i < totallen; i++ {
		c := s[i]
		if c == old {
			nextexclude = false
			if i < totallen-1 {
				next := s[i+1]
				for _, e := range nextExcludes {
					if next == e {
						nextexclude = true
						break
					}
				}
			}
			if nextexclude {
				targetslice = append(targetslice, c)
				targetslice = append(targetslice, s[i+1])
				i++
			} else {
				targetslice = append(targetslice, []byte(new)...)
			}
		} else {
			targetslice = append(targetslice, c)
		}
	}

	return string(targetslice)
}

// ensure compiler exist in args.
func ensureCompilerRaw(args []string, workdir string) (string, []string, bool, string, string, string, error) {
	responseFile := ""
	sourcedependfile := ""
	objectfile := ""
	pchfile := ""
	showinclude := false
	if len(args) == 0 {
		blog.Warnf("cc: ensure compiler got empty arg")
		return responseFile, nil, showinclude, sourcedependfile, objectfile, pchfile, ErrorMissingOption
	}

	if args[0] == "/" || args[0] == "@" || isSourceFile(args[0]) || isObjectFile(args[0]) {
		return responseFile, append([]string{defaultCompiler}, args...), showinclude, sourcedependfile, objectfile, pchfile, nil
	}

	for _, v := range args {
		if strings.HasPrefix(v, "@") {
			responseFile = strings.Trim(v[1:], "\"")

			allrspfile := []string{}
			options := expandRspFilesRecursively(responseFile, workdir, &allrspfile)
			if len(options) == 0 {
				return responseFile, nil, showinclude, sourcedependfile, objectfile, pchfile, fmt.Errorf("failed to resolve response file")
			}

			args = []string{args[0]}
			args = append(args, options...)

		} else if v == "/showIncludes" {
			showinclude = true
		}
	}

	firstinclude := true
	for i := range args {
		if strings.HasPrefix(args[i], "-MF") {
			if len(args[i]) > 3 {
				sourcedependfile = args[i][3:]
				continue
			}

			i++
			if i >= len(args) {
				blog.Warnf("cc: scan args: no output file found after -MF")
				return responseFile, nil, showinclude, sourcedependfile, objectfile, pchfile, ErrorMissingOption
			}
			sourcedependfile = args[i]
		} else if strings.HasPrefix(args[i], "/clang:-MF") {
			if len(args[i]) > 10 {
				sourcedependfile = strings.Trim(args[i][10:], "\"")
				continue
			}

			i++
			if i >= len(args) {
				blog.Warnf("cc: scan args: no output file found after /clang:-MF")
				continue
			}
			sourcedependfile = strings.Trim(args[i], "\"")
		} else if strings.HasPrefix(args[i], "-o") {
			// if -o just a prefix, the output file is also in this index, then skip the -o.
			if len(args[i]) > 2 {
				objectfile = args[i][2:]
				blog.Infof("cc: got objectfile file:%s", objectfile)
				continue
			}

			i++
			if i >= len(args) {
				blog.Warnf("cc: scan args: no output file found after -o")
				return responseFile, nil, showinclude, sourcedependfile, objectfile, pchfile, ErrorMissingOption
			}
			objectfile = args[i]
			blog.Infof("cc: got objectfile file:%s", objectfile)
		} else if strings.HasPrefix(args[i], "/Fo") {
			// if /Fo just a prefix, the output file is also in this index, then skip the /Fo.
			if len(args[i]) > 3 {
				objectfile = strings.Trim(args[i][3:], "\"")
				blog.Infof("cc: got objectfile file:%s", objectfile)
				continue
			}

			// if file name is in the next index, then take it.
			// Whatever follows must be the output file
			i++
			if i >= len(args) {
				blog.Warnf("cc: scan args: no output file found after /Fo")
				return responseFile, nil, showinclude, sourcedependfile, objectfile, pchfile, ErrorMissingOption
			}
			objectfile = strings.Trim(args[i], "\"")
			blog.Infof("cc: got objectfile file:%s", objectfile)
		} else if strings.HasPrefix(args[i], "-include-pch") {
			firstinclude = false
			if len(args[i]) > 12 {
				pchfile = args[i][12:]
				continue
			}

			i++
			if i >= len(args) {
				blog.Warnf("cc: scan args: no output file found after -include-pch")
				return responseFile, nil, showinclude, sourcedependfile, objectfile, pchfile, ErrorMissingOption
			}
			pchfile = args[i]
		} else if firstinclude && strings.HasPrefix(args[i], "-include") {
			firstinclude = false
			i++
			if i >= len(args) {
				blog.Warnf("cc: scan args: no output file found after -include")
				return responseFile, nil, showinclude, sourcedependfile, objectfile, pchfile, ErrorMissingOption
			}
			pchfile = args[i] + ".gch"
			blog.Infof("cc: ready check gch file of %s", pchfile)
		}
	}

	if responseFile != "" && !filepath.IsAbs(responseFile) {
		responseFile, _ = filepath.Abs(filepath.Join(workdir, responseFile))
	}

	if sourcedependfile != "" && !filepath.IsAbs(sourcedependfile) {
		sourcedependfile, _ = filepath.Abs(filepath.Join(workdir, sourcedependfile))
	}

	if objectfile != "" && !filepath.IsAbs(objectfile) {
		objectfile, _ = filepath.Abs(filepath.Join(workdir, objectfile))
	}

	if pchfile != "" && !filepath.IsAbs(pchfile) {
		pchfile, _ = filepath.Abs(filepath.Join(workdir, pchfile))
		if !dcFile.Stat(pchfile).Exist() {
			pchfile = ""
		}
	}

	return responseFile, args, showinclude, sourcedependfile, objectfile, pchfile, nil
}

func expandRspFilesRecursively(responseFile, workdir string, rspfiles *[]string) []string {
	data := ""
	fullrsppath := ""
	if responseFile != "" {
		var err error
		data, fullrsppath, err = readResponse(responseFile, workdir)
		if err != nil {
			blog.Infof("cc: failed to read response file:%s,err:%v", responseFile, err)
			return nil
		}
	}

	options, err := shlex.Split(replaceWithNextExclude(string(data), '\\', "\\\\", []byte{'"'}))
	if err != nil {
		blog.Infof("cc: failed to parse response file:%s,err:%v", responseFile, err)
		return nil
	}

	for i := 0; i < len(options); i++ {
		if strings.HasPrefix(options[i], "@") && len(options[i]) > 1 {
			newoptions := expandRspFilesRecursively(strings.Trim(options[i][1:], "\""), workdir, rspfiles)
			if len(newoptions) > 0 {
				options = append(options[:i], append(newoptions, options[i+1:]...)...)
				i += len(newoptions) - 1
			}
		}
	}

	*rspfiles = append(*rspfiles, fullrsppath)

	return options
}

// ensure compiler exist in args.
// change "executor -c foo.c" -> "cc -c foo.c"
func ensureCompiler(args []string, workdir string) (string, []string, []string, error) {
	responseFile := ""
	if len(args) == 0 {
		blog.Warnf("cc: ensure compiler got empty arg")
		return responseFile, nil, nil, ErrorMissingOption
	}

	if args[0] == "-" || isSourceFile(args[0]) || isObjectFile(args[0]) {
		return responseFile, append([]string{defaultCompiler}, args...), nil, nil
	}

	allrspfile := []string{}
	for _, v := range args {
		if strings.HasPrefix(v, "@") {
			responseFile = strings.Trim(v[1:], "\"")
			options := expandRspFilesRecursively(responseFile, workdir, &allrspfile)
			if len(options) == 0 {
				return responseFile, nil, nil, fmt.Errorf("failed to resolve response file")
			}

			for i := range options {
				if options[i] == "/we4668" {
					options[i] = "/wd4668" // for ue4
					break
				}
			}

			args = []string{args[0]}
			args = append(args, options...)
		}
	}

	return responseFile, args, allrspfile, nil
}

var (
	sourceFileExt = map[string]bool{
		// begin with i
		".i":  true,
		".ii": true,

		// begin with c
		".c":   true,
		".cc":  true,
		".cpp": true,
		".cxx": true,
		".cp":  true,
		".c++": true,

		// begin with C
		".C": true,

		// begin with m
		".m":   true,
		".mm":  true,
		".mi":  true,
		".mii": true,

		// begin with M
		".M": true,
	}

	preprocessedFileExt = map[string]bool{
		// begin with i
		".i":  true,
		".ii": true,

		// begin with m
		".mi":  true,
		".mii": true,
	}

	preprocessedExtensionMap = map[string]string{
		// extension .i
		".i": ".i",
		".c": ".i",

		// extension .ii
		".cc":  ".ii",
		".cpp": ".ii",
		".cxx": ".ii",
		".cp":  ".ii",
		".c++": ".ii",
		".C":   ".ii",
		".ii":  ".ii",

		// extension .mi
		".mi": ".mi",
		".m":  ".mi",

		// extension .mii
		".mii": ".mii",
		".mm":  ".mii",

		// extension .s
		".s": ".s",
		".S": ".s",
	}

	// skip options and skip its value in the next index
	skipLocalOptionsWithValue = map[string]bool{
		"-D":                 true,
		"-I":                 true,
		"-U":                 true,
		"-L":                 true,
		"-l":                 true,
		"-MF":                true,
		"-MT":                true,
		"-MQ":                true,
		"-include":           true,
		"-imacros":           true,
		"-iprefix":           true,
		"-iwithprefix":       true,
		"-isystem":           true,
		"-iwithprefixbefore": true,
		"-idirafter":         true,
		"-include-pch":       true,
		// ++ for ue 4.26 mac compile
		"-isysroot": true,
		// --
		"/I":         true,
		"/imsvc":     true,
		"/FI":        true,
		"/clang:-MF": true,
	}

	// skip options without value
	skipLocalOptions = map[string]bool{
		"-undef":      true,
		"-nostdinc":   true,
		"-nostdinc++": true,
		"-MD":         true,
		"-MMD":        true,
		"-MG":         true,
		"-MP":         true,
		// ++ for ue4.25.0 linux clang++
		"-Werror": true,
		// --
		"/clang:-MD": true,
	}

	// skip options start with flags
	skipLocalOptionStartWith = map[string]bool{
		"-Wp,":            true,
		"-Wl,":            true,
		"-D":              true,
		"-I":              true,
		"-U":              true,
		"-L":              true,
		"-l":              true,
		"-MF":             true,
		"-MT":             true,
		"-MQ":             true,
		"-isystem":        true,
		"@":               true, // such as @"..\XXX\XXX.rsp"
		"--gcc-toolchain": true,
		"--sysroot":       true,
		"/I":              true,
		"/imsvc":          true,
		"/FI":             true,
		"/clang:-MF":      true,
	}
)

func isSourceFile(filename string) bool {
	if _, ok := sourceFileExt[filepath.Ext(filename)]; ok {
		return true
	}

	return false
}

func isObjectFile(filename string) bool {
	return filepath.Ext(filename) == ".o"
}

// check if the given file is already preprocessed
func isPreprocessedFile(filename string) bool {
	if _, ok := preprocessedFileExt[filepath.Ext(filename)]; ok {
		return true
	}

	return false
}

// get a input file's extension and return the extension should be after preprocessed.
func getPreprocessedExt(inputFile string) string {
	inputExt := filepath.Ext(inputFile)
	if _, ok := preprocessedExtensionMap[inputExt]; !ok {
		return ""
	}

	return preprocessedExtensionMap[inputExt]
}

// Strip arguments like -D and -I from a command line, because they do
// not need to be passed across the wire.  This covers options for
// both the preprocess and link phases, since they should never happen
// remotely.
//
// In the case where we inadvertently do cause preprocessing to happen
// remotely, it is possible that omitting these options will make
// failure more obvious and avoid false success.
//
// Giving -L on a compile-only command line is a bit wierd, but it is
// observed to happen in Makefiles that are not strict about CFLAGS vs
// LDFLAGS, etc.
//
// NOTE: gcc-3.2's manual in the "preprocessor options" section
// describes some options, such as -d, that only take effect when
// passed directly to cpp.  When given to gcc they have different
// meanings.
//
// The value stored in '*out_argv' is malloc'd, but the arguments that
// are pointed to by that array are aliased with the values pointed
// to by 'from'.  The caller is responsible for calling free() on
// '*out_argv'.
func stripLocalArgs(args []string, env *env.Sandbox) []string {
	r := make([]string, 0, len(args))

	// skip through argv, copying all arguments but skipping ones that ought to be omitted
	for index := 0; index < len(args); index++ {
		arg := args[index]

		// skip the options and its value in next index.
		if _, ok := skipLocalOptionsWithValue[arg]; ok {
			index++
			continue
		}

		// skip the options that with value together
		if func() bool {
			for key := range skipLocalOptionStartWith {
				if strings.HasPrefix(arg, key) {
					return true
				}
			}
			return false
		}() {
			continue
		}

		// skip the options without value
		if _, ok := skipLocalOptions[arg]; ok {
			continue
		}

		r = append(r, arg)
	}

	if !dcSyscall.NeedSearchToolchain(env) {
		r[0] = filepath.Base(r[0])
	}
	return r
}

// convert any -Wp options into regular gcc options.
func expandPreprocessorOptions(args []string) ([]string, error) {
	r := make([]string, 0, len(args)*2)
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-Wp,") {
			r = append(r, arg)
			continue
		}

		options, err := copyExtraArgs(arg)
		if err != nil {
			return nil, err
		}

		r = append(r, options...)
	}

	return r, nil
}

// Convert a "-Wp,..." option into one or more regular gcc options.
// Copy the resulting gcc options to dest_argv, which should be pre-allocated by the caller.
func copyExtraArgs(option string) ([]string, error) {
	args := strings.Split(option, ",")
	if len(args) == 0 || args[0] != "Wp" {
		return nil, fmt.Errorf("handle copy extra args(%s) failed: %v", option, ErrorMissingOption)
	}

	r := make([]string, 0, len(args))

	// skip first arg "-Wp"
	for index := 1; index < len(args); index++ {
		arg := args[index]

		r = append(r, arg)
		if arg == "-MD" || arg == "-MMD" {
			// if there is no args after -MD or -MMD, the args must be missing filename.
			if index == len(args)-1 {
				return nil, fmt.Errorf("'-Wp,-MD' or '-Wp,-MMD' option is missing filename argument")
			}

			r = append(r, "-MF")
		}
	}

	return r, nil
}

type ccArgs struct {
	inputFile       string
	outputFile      string
	mfOutputFile    []string
	args            []string
	includeRspFiles []string // with @ in response file
	includePaths    []string // with -I
	includeFiles    []string // with -include
	hasDependencies bool
}

// scanArgs receive the complete compiling args, and the first item should always be a compiler name.
func scanArgs(args []string) (*ccArgs, error) {
	blog.Infof("cc: scanning arguments: %v", args)

	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		blog.Warnf("cc: scan args: unrecognized option: %s", args[0])
		return nil, ErrorUnrecognizedOption
	}

	r := new(ccArgs)
	seenOptionS := false
	seenOptionC := false
	seenOptionO := false
	seenInputFile := false
	for index := 0; index < len(args); index++ {
		arg := args[index]

		if strings.HasPrefix(arg, "-") {
			switch arg {
			case "-E":
				// pre-process should be run locally.
				blog.Warnf("cc: scan args: -E call for cpp must be local")
				return nil, ErrorNotSupportE

			case "-MD", "-MMD":
				// These two generate dependencies as a side effect.  They
				// should work with the way we call cpp.
				r.hasDependencies = true
				continue

			case "-MG", "-MP":
				// These just modify the behaviour of other -M* options and do
				// nothing by themselves.
				continue

			case "-MF", "-MT", "-MQ":
				// As above but with extra argument.
				index++
				if arg == "-MF" && index < len(args) {
					r.mfOutputFile = append(r.mfOutputFile, strings.Trim(arg, "\""))
				}
				continue

			case "-march=native":
				blog.Warnf("cc: scan args: -march=native generates code for local machine; must be local")
				return nil, ErrorNotSupportMarchNative

			case "-mtune=native":
				blog.Warnf("cc: scan args: -mtune=native optimizes for local machine; must be local")
				return nil, ErrorNotSupportMtuneNative

			case "-fprofile-arcs", "-ftest-coverage", "--coverage":
				blog.Warnf("cc: scan args: compiler will emit profile info; must be local")
				return nil, ErrorNotSupportCoverage

			case "-frepo":
				blog.Warnf("cc: scan args: compiler will emit .rpo files; must be local")
				return nil, ErrorNotSupportFrepo

			case "-S":
				seenOptionS = true
				continue

			case "-c":
				seenOptionC = true
				continue
			}

			// ++ by tomtian 20201127,for example: -MF/data/.../XX.cpp.d
			if strings.HasPrefix(arg, "-MF") && len(arg) > 3 {
				r.mfOutputFile = append(r.mfOutputFile, strings.Trim(args[index][3:], "\""))
				continue
			}
			// --

			// -M(anything else) causes the preprocessor to
			// produce a list of make-style dependencies on
			// header files, either to stdout or to a local file.
			// It implies -E, so only the preprocessor is run,
			// not the compiler. There would be no point trying
			// to distribute it even if we could.
			if strings.HasPrefix(arg, "-M") {
				blog.Warnf("cc: scan args: %s implies -E (maybe) and must be local", arg)
				return nil, ErrorNotSupportM
			}

			// Look for assembler options that would produce output
			// files and must be local.
			// Writing listings to stdout could be supported but it might
			// be hard to parse reliably.
			if strings.HasPrefix(arg, "-Wa") {
				if strings.Contains(arg, ",-a") || strings.Contains(arg, "--MD") {
					blog.Warnf("cc: scan args: %s must be local", arg)
					return nil, ErrorNotSupportWa
				}
				continue
			}

			if strings.HasPrefix(arg, "-specs=") {
				blog.Warnf("cc: scan args: %s must be local", arg)
				return nil, ErrorNotSupportSpecs
			}

			// ++ by tomtian 20250408, to support -Xclang
			if strings.HasPrefix(arg, "-Xclang") {
				// skip -Xclang and next arg
				index++
				index++
			}
			//--

			if strings.HasPrefix(arg, "-x") {
				index++
				if index >= len(args) {
					continue
				}
				arg = args[index]

				if strings.HasPrefix(arg, "c") ||
					strings.HasPrefix(arg, "c++") ||
					strings.HasPrefix(arg, "objective-c") ||
					strings.HasPrefix(arg, "objective-c++") ||
					strings.HasPrefix(arg, "go") {
					continue
				}

				blog.Warnf("cc: scan args: gcc's -x handling is complex; running locally for %s", arg)
				return nil, ErrorNotSupportX
			}

			if strings.HasPrefix(arg, "-dr") {
				blog.Warnf("cc: scan args: gcc's debug option %s may write extra files; running locally", arg)
				return nil, ErrorNotSupportDr
			}

			// ++ by tomtian 2021-05-18
			if strings.HasPrefix(arg, "-fsanitize") {
				blog.Warnf("cc: scan args: clang option %s need read origin source file; running locally", arg)
				return nil, ErrorNotSupportFsanitize
			}
			// --

			if strings.HasPrefix(arg, "-I") {
				// if -I just a prefix, save the remain of this line.
				if len(arg) > 2 {
					r.includePaths = append(r.includePaths, strings.Trim(arg[2:], "\""))
					continue
				}

				// if file name is in the next index, then take it.
				index++
				if index >= len(args) {
					blog.Warnf("cc: scan args: no file found after -I")
					return nil, ErrorMissingOption
				}
				r.includePaths = append(r.includePaths, strings.Trim(args[index], "\""))
				continue
			}

			if strings.HasPrefix(arg, "-include") {
				keylen := 8
				if arg == "-include-pch" {
					keylen = 12
				}

				// if -include just a prefix, save the remain of this line.
				if len(arg) > keylen {
					r.includeFiles = append(r.includeFiles, strings.Trim(arg[keylen:], "\""))
					continue
				}

				// if file name is in the next index, then take it.
				index++
				if index >= len(args) {
					blog.Warnf("cc: scan args: no file found after -include or -include-pch")
					return nil, ErrorMissingOption
				}
				r.includeFiles = append(r.includeFiles, strings.Trim(args[index], "\""))
				continue
			}

			if strings.HasPrefix(arg, "-o") {
				// -o should always appear once.
				if seenOptionO {
					blog.Warnf("cc: scan args: multi -o found in args")
					return nil, ErrorInvalidOption
				}
				seenOptionO = true

				// if -o just a prefix, the output file is also in this index, then skip the -o.
				if len(arg) > 2 {
					r.outputFile = arg[2:]
					continue
				}

				// if file name is in the next index, then take it.
				// Whatever follows must be the output file
				index++
				if index >= len(args) {
					blog.Warnf("cc: scan args: no output file found after -o")
					return nil, ErrorMissingOption
				}
				r.outputFile = args[index]
				continue
			}
			continue
		} else if strings.HasPrefix(arg, "@") {
			r.includeRspFiles = append(r.includeRspFiles, arg[1:])
		} else if strings.HasPrefix(arg, "/") { // support clang-cl.exe
			switch arg {
			case "/c":
				seenOptionC = true
				continue
			case "/Yc":
				// Creates a precompiled header file, should be run locally
				blog.Warnf("cc: scan args: /Yc call for cpp must be local")
				return nil, ErrorNotSupportYc
			}

			if strings.HasPrefix(arg, "/Yc") {
				// Creates a precompiled header file, should be run locally
				blog.Warnf("cc: scan args: /Yc call for cpp must be local")
				return nil, ErrorNotSupportYc
			}

			if strings.HasPrefix(arg, "/Fo") {
				// /Fo should always appear once.
				if seenOptionO {
					blog.Warnf("cc: scan args: multi /FO found in args")
					return nil, ErrorInvalidOption
				}
				seenOptionO = true

				// if /Fo just a prefix, the output file is also in this index, then skip the /Fo.
				if len(arg) > 3 {
					r.outputFile = strings.Trim(arg[3:], "\"")
					continue
				}

				// if file name is in the next index, then take it.
				// Whatever follows must be the output file
				index++
				if index >= len(args) {
					blog.Warnf("cc: scan args: no output file found after /Fo")
					return nil, ErrorMissingOption
				}
				r.outputFile = strings.Trim(arg, "\"")
				continue
			}

			if strings.HasPrefix(arg, "/clang:-MF") {
				if len(arg) > 10 {
					r.mfOutputFile = append(r.mfOutputFile, strings.Trim(arg[10:], "\""))
					continue
				}

				index++
				if index >= len(args) {
					blog.Warnf("cc: scan args: no output file found after /clang:-MF")
					continue
				}
				r.mfOutputFile = append(r.mfOutputFile, strings.Trim(arg, "\""))
			}

			continue
		}

		// if this is not start with -, then it maybe a file.
		if isSourceFile(arg) {
			if seenInputFile {
				blog.Warnf("cc: scan args: multi input file found in args")
				return nil, ErrorInvalidOption
			}
			seenInputFile = true

			r.inputFile = arg
			continue
		} else {
			blog.Debugf("cc: arg[%s] is not source file", arg)
		}

		// if this file is end with .o, it must be the output file.
		if strings.HasSuffix(arg, ".o") {
			if seenOptionO {
				blog.Warnf("cc: scan args: multi -o found in args")
				return nil, ErrorInvalidOption
			}
			seenOptionO = true
			r.outputFile = args[index]
		}
	}

	if !seenOptionC && !seenOptionS {
		blog.Warnf("cc: scan args: no -c or -s found, compiler apparently called not for compile")
		return nil, ErrorMissingOption
	}

	if !seenInputFile {
		blog.Warnf("cc: scan args: no visible input file")
		return nil, ErrorMissingOption
	}

	// in some cases, input file can not run remotely.
	if base := filepath.Base(r.inputFile); strings.HasPrefix(base, "conftest.") ||
		strings.HasSuffix(base, "tmp.conftest.") {
		blog.Warnf("cc: scan args: autoconf tests are run locally: %s", r.inputFile)
		return nil, ErrorNotSupportConftest
	}

	// This is a commandline like "gcc -c hello.c".  They want
	// hello.o, but they don't say so.  For example, the Ethereal
	// makefile does this.
	//
	// Note: this doesn't handle a.out, the other implied
	// filename, but that doesn't matter because it would already
	// be excluded by not having -c or -S.
	if !seenOptionO {
		var outputFile string
		var err error

		// -S takes precedence over -c, because it means "stop after
		// preprocessing" rather than "stop after compilation."
		if seenOptionS {
			if outputFile, err = outputFromSource(r.inputFile, ".s"); err != nil {
				return nil, err
			}
		} else if seenOptionC {
			if outputFile, err = outputFromSource(r.inputFile, ".o"); err != nil {
				return nil, err
			}
		}

		blog.Infof("cc: no visible output file, going to add \"-o %s\" at end", outputFile)
		args = append(args, "-o", outputFile)
		r.outputFile = outputFile
	}

	if r.outputFile == "-" {
		blog.Warnf("cc: output to stdout, running locally")
		return nil, ErrorNotSupportOutputStdout
	}

	if strings.HasSuffix(r.outputFile, ".gch") {
		blog.Warnf("cc: output file is gch, running locally")
		return nil, ErrorNotSupportGch
	}

	r.args = args
	blog.Infof("cc: success to scan arguments: %s, input file %s, output file %s",
		r.args, r.inputFile, r.outputFile)
	return r, nil
}

// Work out the default object file name the compiler would use if -o
// was not specified.  We don't need to worry about "a.out" because
// we've already determined that -c or -S was specified.
//
// However, the compiler does put the output file in the current
// directory even if the source file is elsewhere, so we need to strip
// off all leading directories.
//
// @param sfile Source filename.  Assumed to match one of the
// recognized patterns, otherwise bad things might happen.
func outputFromSource(filename, ext string) (string, error) {
	if len(filepath.Base(filename)) < 3 {
		blog.Warnf("cc: outputFromSource failed: source file %s is bogus", filename)
		return "", ErrorInvalidOption
	}

	return strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename)) + ext, nil
}

// rewrite "cc" to directly call gcc or clang
func rewriteGenericCompiler(args []string) ([]string, error) {
	if len(args) == 0 {
		blog.Warnf("cc: rewrite generic compiler got empty arg")
		return nil, ErrorMissingOption
	}

	cpp := false
	switch args[0] {
	case "cc":
	case "c++":
		cpp = true
	default:
		return args, nil
	}

	// TODO: finish the rewrite-generic-compiler
	_ = cpp
	return args, nil
}

// Clang is a native cross-compiler, but needs to be told to what target it is
// building.
func addClangTarget(args []string) ([]string, error) {
	if len(args) == 0 {
		blog.Warnf("cc: add clang target got empty arg")
		return nil, ErrorMissingOption
	}

	// if it is not about clang, just return.
	if args[0] == "clang" ||
		args[0] == "clang++" ||
		strings.HasPrefix(args[0], "clang-") ||
		strings.HasPrefix(args[0], "clang++-") {

	} else {
		return args, nil
	}

	// already has -target
	if hasOptions(args, "-target") {
		return args, nil
	}

	target := gnuHost
	blog.Infof("cc: adding '-target %s' to support clang cross-compilation", target)
	return append(args, "-target", target), nil
}

// Cross compilation for gcc
func gccRewriteFqn(args []string) ([]string, error) {
	if len(args) == 0 {
		blog.Warnf("cc: gcc rewrite fqn got empty arg")
		return nil, ErrorMissingOption
	}

	// if it is not about gcc, just return.
	if args[0] == "gcc" ||
		args[0] == "g++" ||
		strings.HasPrefix(args[0], "gcc-") ||
		strings.HasPrefix(args[0], "g++-") {

	} else {
		return args, nil
	}

	target := gnuHost
	newCmd := target + "-" + args[0]
	if strings.Contains(newCmd, "-pc-") {
		piece := strings.Split(newCmd, "-pc-")
		newCmd = piece[0] + "-" + piece[1]
	}

	path := getEnv("PATH")
	for _, p := range strings.Split(path, ":") {
		f, err := os.Stat(strings.TrimRight(p, "/") + "/" + newCmd)
		if err != nil {
			continue
		}

		// if the target exist and is executable
		if f.Mode()&0111 != 0 {
			blog.Infof("cc: re-writing call from '%s' to '%s' to support cross-compilation.", args[0], newCmd)
			return append([]string{newCmd}, args[1:]...), nil
		}
	}

	blog.Debugf("cc: gcc rewrite no found executable compiler from '%s' to '%s'", args[0], newCmd)
	return args, nil
}

func hasOptions(r []string, s string) bool {
	for _, i := range r {
		if i == s {
			return true
		}
	}

	return false
}

// Create a file inside the temporary directory and register it for
// later cleanup, and return its name.
//
// The file will be reopened later, possibly in a child.  But we know
// that it exists with appropriately tight permissions.
func makeTmpFile(tmpDir, prefix, ext string) (string, error) {
	stat, err := os.Stat(tmpDir)
	if err != nil {
		blog.Errorf("cc: can not access tmp dir \"%s\": %s", tmpDir, err)
		return "", err
	}
	if !stat.IsDir() || stat.Mode()&0555 == 0 {
		blog.Errorf("cc: can not access tmp dir \"%s\": is not a dir or could not be write or execute.", tmpDir)
		return "", ErrorFileInvalid
	}

	var target string
	pid := os.Getpid()
	for i := 0; i < 3; i++ {
		target = filepath.Join(tmpDir,
			fmt.Sprintf("%s_%d_%s_%d%s",
				prefix, pid, commonUtil.RandomString(8), time.Now().UnixNano(), ext))

		f, err := os.Create(target)
		if err != nil {
			blog.Errorf("cc: failed to create tmp file \"%s\": %s", target, err)
			continue
		}

		if err = f.Close(); err != nil {
			blog.Errorf("cc: failed to close tmp file \"%s\": %s", target, err)
			return "", err
		}

		blog.Infof("cc: success to make tmp file \"%s\"", target)
		return target, nil
	}

	return "", fmt.Errorf("cc: create tmp file failed: %s", target)
}

func getPumpIncludeFile(tmpDir, prefix, ext string, args []string, workdir string) (string, error) {
	fullarg := strings.Join(args, " ")
	md5str := md5.Sum([]byte((fullarg + workdir)))
	target := filepath.Join(tmpDir, fmt.Sprintf("%s_%x%s", prefix, md5str, ext))

	return target, nil
}

func createFile(target string) error {
	for i := 0; i < 3; i++ {
		f, err := os.Create(target)
		if err != nil {
			blog.Errorf("cl: failed to create tmp file \"%s\": %s", target, err)
			continue
		}

		if err = f.Close(); err != nil {
			blog.Errorf("cl: failed to close tmp file \"%s\": %s", target, err)
			return err
		}

		blog.Infof("cl: success to make tmp file \"%s\"", target)
		return nil
	}

	return fmt.Errorf("cl: create tmp file failed: %s", target)
}

// only genegerate file name, do not create really
func makeTmpFileName(tmpDir, prefix, ext string) string {
	pid := os.Getpid()

	return filepath.Join(tmpDir,
		fmt.Sprintf("%s_%d_%s_%d%s",
			prefix, pid, commonUtil.RandomString(8), time.Now().UnixNano(), ext))
}

// Remove "-o" options from argument list.
//
// This is used when running the preprocessor, when we just want it to write
// to stdout, which is the default when no -o option is specified.
//
// Structurally similar to dcc_strip_local_args()
func stripDashO(args []string) []string {
	r := make([]string, 0, len(args))

	for index := 0; index < len(args); index++ {
		arg := args[index]

		if arg == "-o" {
			index++
			continue
		}

		if strings.HasPrefix(arg, "-o") {
			continue
		}

		r = append(r, arg)
	}

	return r
}

// Used to change "-c" or "-S" to "-E", so that we get preprocessed
// source.
func setActionOptionE(args []string) ([]string, error) {
	r := make([]string, 0, len(args))

	found := false
	for _, arg := range args {
		if arg == "-c" || arg == "-S" {
			found = true
			r = append(r, "-E")
			continue
		} else if arg == "/c" {
			found = true
			r = append(r, "/E")
			continue
		}

		r = append(r, arg)
	}

	if !found {
		blog.Warnf("cc: failed to find -c or -S")
		return nil, ErrorMissingOption
	}

	return r, nil
}

func getPreloadConfig(configPath string) (*dcSDK.PreloadConfig, error) {
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = f.Close()
	}()

	var pConfig dcSDK.PreloadConfig
	if err = codec.DecJSONReader(f, &pConfig); err != nil {
		return nil, err
	}

	return &pConfig, nil
}

func getFirstIncludeFile(args []string) string {
	for index := 0; index < len(args); index++ {
		if args[index] == "-include" {
			if index+1 < len(args) {
				return args[index+1]
			}
			return ""
		}
	}
	return ""
}

func saveResultFile(rf *dcSDK.FileDesc, dir string) error {
	fp := rf.FilePath
	data := rf.Buffer
	blog.Debugf("cc: ready save file [%s]", fp)
	if fp == "" {
		blog.Warnf("cc: file [%s] path is empty!", fp)
		return fmt.Errorf("file path is empty")
	}

	if !filepath.IsAbs(fp) {
		fp = filepath.Join(dir, fp)
	}

	if rf.CompressedSize > 0 {
		switch rf.Compresstype {
		case protocol.CompressNone:

			f, err := os.Create(fp)
			if err != nil {
				if !filepath.IsAbs(fp) && dir != "" {
					newfp, _ := filepath.Abs(filepath.Join(dir, fp))
					f, err = os.Create(newfp)
					if err != nil {
						blog.Errorf("cc: create file %s or %s error: [%s]", fp, newfp, err.Error())
						return err
					}
				} else {
					blog.Errorf("cc: create file %s error: [%s]", fp, err.Error())
					return err
				}
			}
			defer f.Close()

			_, err = f.Write(data)
			if err != nil {
				blog.Errorf("save file [%s] error: [%s]", fp, err.Error())
				return err
			}
			break

		case protocol.CompressLZ4:
			// decompress with lz4 firstly
			dst := make([]byte, rf.FileSize)
			if dst == nil {
				err := fmt.Errorf("failed to alloc [%d] size buffer", rf.FileSize)
				blog.Errorf("%v", err)
				return err
			}

			// allocTime = time.Now().Local().UnixNano()
			outdata, err := dcUtil.Lz4Uncompress(data, dst)
			if err != nil {
				blog.Errorf("cc: decompress [%s] error: [%s], data len:[%d], buffer len:[%d], filesize:[%d]",
					fp, err.Error(), len(data), len(dst), rf.FileSize)
				return err
			}
			// compressTime = time.Now().Local().UnixNano()
			// outlen := len(string(outdata))
			outlen := len(outdata)
			blog.Debugf("cc: decompressed file %s with lz4, from [%d] to [%d]", fp, rf.CompressedSize, outlen)
			if outlen != int(rf.FileSize) {
				err := fmt.Errorf("decompressed size %d, expected size %d", outlen, rf.FileSize)
				blog.Errorf("cc: decompress error: [%v]", err)
				return err
			}

			f, err := os.Create(fp)
			if err != nil {
				if !filepath.IsAbs(fp) && dir != "" {
					newfp, _ := filepath.Abs(filepath.Join(dir, fp))
					f, err = os.Create(newfp)
					if err != nil {
						blog.Errorf("cc: create file %s or %s error: [%s]", fp, newfp, err.Error())
						return err
					}
				} else {
					blog.Errorf("cc: create file %s error: [%s]", fp, err.Error())
					return err
				}
			}
			defer f.Close()

			_, err = f.Write(outdata)
			if err != nil {
				blog.Errorf("cc: save file [%s] error: [%v]", fp, err)
				return err
			}
			blog.Infof("cc: succeed save file %s size [%d]", fp, outlen)
			break
		default:
			return fmt.Errorf("cc: unknown compress type [%s]", rf.Compresstype)
		}
	}

	blog.Debugf("cc: succeed to save file [%s]", fp)
	return nil
}

// EscapeArg and MakeCmdLine copied from exec_windows.go

// EscapeArg rewrites command line argument s as prescribed
// in https://msdn.microsoft.com/en-us/library/ms880421.
// This function returns "" (2 double quotes) if s is empty.
// Alternatively, these transformations are done:
//   - every back slash (\) is doubled, but only if immediately
//     followed by double quote (");
//   - every double quote (") is escaped by back slash (\);
//   - finally, s is wrapped with double quotes (arg -> "arg"),
//     but only if there is space or tab inside s.
func EscapeArg(s string) string {
	if len(s) == 0 {
		return "\"\""
	}
	n := len(s)
	hasSpace := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			n++
		case ' ', '\t':
			hasSpace = true
		}
	}
	if hasSpace {
		n += 2
	}
	if n == len(s) {
		return s
	}

	qs := make([]byte, n)
	j := 0
	if hasSpace {
		qs[j] = '"'
		j++
	}
	slashes := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		default:
			slashes = 0
			qs[j] = s[i]
		case '\\':
			slashes++
			qs[j] = s[i]
		case '"':
			for ; slashes > 0; slashes-- {
				qs[j] = '\\'
				j++
			}
			qs[j] = '\\'
			j++
			qs[j] = s[i]
		}
		j++
	}
	if hasSpace {
		for ; slashes > 0; slashes-- {
			qs[j] = '\\'
			j++
		}
		qs[j] = '"'
		j++
	}
	return string(qs[:j])
}

// EscapeArg and MakeCmdLine copied from exec_windows.go

// MakeCmdLine builds a command line out of args by escaping "special"
// characters and joining the arguments with spaces.
func MakeCmdLine(args []string) string {
	var s string
	for _, v := range args {
		if s != "" {
			s += " "
		}
		s += EscapeArg(v)
	}
	return s
}

// 根据 clang 命令，获取相应的 resource-dir
type clangResourceDirInfo struct {
	clangcommandfullpath string
	clangResourceDirpath string
}

var (
	clangResourceDirlock sync.RWMutex
	clangResourceDirs    []clangResourceDirInfo
)

func getResourceDir(cmd string) (string, error) {
	var err error
	exepfullath := cmd
	if !filepath.IsAbs(cmd) {
		exepfullath, err = dcUtil.CheckExecutable(cmd)
		if err != nil {
			return "", err
		}
	}

	// search from cache
	clangResourceDirlock.RLock()
	resourcedir := ""
	for _, v := range clangResourceDirs {
		if exepfullath == v.clangcommandfullpath {
			resourcedir = v.clangResourceDirpath
			clangResourceDirlock.RUnlock()
			return resourcedir, nil
		}
	}
	clangResourceDirlock.RUnlock()

	// try get resource-dir with clang exe path
	clangResourceDirlock.Lock()
	maxversion := ""
	appended := false
	defer func() {
		// append to cache if not
		if !appended {
			clangResourceDirs = append(clangResourceDirs, clangResourceDirInfo{
				clangcommandfullpath: exepfullath,
				clangResourceDirpath: maxversion,
			})
		}

		clangResourceDirlock.Unlock()
	}()

	// search from cache again, maybe append by others
	for _, v := range clangResourceDirs {
		if exepfullath == v.clangcommandfullpath {
			resourcedir = v.clangResourceDirpath
			appended = true
			return resourcedir, nil
		}
	}

	// real compute resource-dir now
	exedir := filepath.Dir(exepfullath)
	exeparentdir := filepath.Dir(exedir)
	foundclangdir := false
	target := filepath.Join(exeparentdir, "lib", "clang")
	if dcFile.Stat(target).Exist() {
		blog.Infof("cc: found clang dir:%s by exe dir:%s", target, exepfullath)
		foundclangdir = true
	} else {
		target = filepath.Join(exeparentdir, "lib64", "clang")
		if dcFile.Stat(target).Exist() {
			blog.Infof("cc: found clang dir:%s by exe dir:%s", target, exepfullath)
			foundclangdir = true
		}
	}

	if !foundclangdir {
		return resourcedir, fmt.Errorf("not found clang dir")
	}

	// get all version dirs, and select the max
	files, err := ioutil.ReadDir(target)
	if err != nil {
		blog.Warnf("failed to get version dirs from dir:%s", target)
		return resourcedir, err
	}

	versiondirs := []string{}
	for _, file := range files {
		if file.IsDir() {
			nums := strings.Split(file.Name(), ".")
			if len(nums) > 1 {
				versiondirs = append(versiondirs, filepath.Join(target, file.Name()))
			}
		}
	}
	blog.Infof("cc: found all clang version dir:%v", versiondirs)

	if len(versiondirs) == 0 {
		return resourcedir, fmt.Errorf("not found any clang's version dir")
	}

	maxversion = versiondirs[0]
	for _, v := range versiondirs {
		if v > maxversion {
			maxversion = v
		}
	}

	blog.Infof("cc: found final resource dir:%s by exe dir:%s", maxversion, exepfullath)
	return maxversion, nil
}

var (
	XcodeIncludeLinkFileslock sync.RWMutex
	XcodeIncludeReal2link     = make(map[string]string, 0)
	XcodeIncludeLink2real     = make(map[string]string, 0)
	XcodeIncludeLinkResolved  = false
)

func getIncludeLinks(env *env.Sandbox, uniqlines []string) ([]string, error) {
	if !dcPump.SupportPumpSearchLink(env) {
		return nil, nil
	}

	if !XcodeIncludeLinkResolved {
		XcodeIncludeLinkFileslock.Lock()

		if !XcodeIncludeLinkResolved {
			XcodeIncludeLinkResolved = true

			var err error
			resultfile := dcPump.LinkResultFile(env)
			XcodeIncludeLink2real, XcodeIncludeReal2link, err = dcPump.ResolveLinkData(resultfile)
			if err != nil {
				blog.Infof("cc: resolve link file %s with error:%v", resultfile, err)
			}
		}

		XcodeIncludeLinkFileslock.Unlock()
	}

	if XcodeIncludeLink2real != nil {
		temparr := make([]string, 0, 10)
		for _, l := range uniqlines {
			if v, ok := XcodeIncludeLink2real[l]; ok {
				temparr = append(temparr, v)
			}
			if v, ok := XcodeIncludeReal2link[l]; ok {
				temparr = append(temparr, v)
			}
		}
		return temparr, nil
	}

	return nil, nil

}

// scanRspFiles 类似scanArgs，递归解析包含的rsp文件，得到依赖列表，包括路径/文件/新的rsp列表
func scanRspFilesRecursively(
	newrspfile string,
	workdir string,
	resultIncludePaths *[]string,
	resultIncludeFiles *[]string,
	checkedRspFiles *[]string) {
	blog.Infof("cc: ready resolve recursively rsp file: %s", newrspfile)

	for _, f := range *checkedRspFiles {
		if f == newrspfile {
			blog.Errorf("cc: found dead loop include response file %s", newrspfile)
			return
		}
	}

	*checkedRspFiles = append(*checkedRspFiles, newrspfile)

	if !filepath.IsAbs(newrspfile) {
		newrspfile, _ = filepath.Abs(filepath.Join(workdir, newrspfile))
	}

	blog.Infof("cc: ready resolve recursively rsp file with full path: %s", newrspfile)

	data := ""
	var err error
	data, _, err = readResponse(newrspfile, workdir)
	if err != nil {
		blog.Infof("cc: failed to read response file:%s,err:%v", newrspfile, err)
		return
	}

	// options, sources, err := parseArgument(data)
	args, err := shlex.Split(replaceWithNextExclude(string(data), '\\', "\\\\", []byte{'"'}))
	if err != nil {
		blog.Infof("cc: failed to parse response file:%s,err:%v", newrspfile, err)
		return
	}

	// for debug
	blog.Infof("cc: response file:%s,args:%+v", newrspfile, args)

	// 只关心包含的依赖，其它选项忽略
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "-") {
			if strings.HasPrefix(arg, "-I") {
				// if -I just a prefix, save the remain of this line.
				if len(arg) > 2 {
					*resultIncludePaths = append(*resultIncludePaths, strings.Trim(arg[2:], "\""))
					// for debug
					blog.Debugf("cc: response file:%s,got include path:%s", newrspfile, strings.Trim(arg[2:], "\""))
					continue
				}

				// if file name is in the next index, then take it.
				index++
				if index >= len(args) {
					blog.Warnf("cc: scan args: no file found after -I")
					return
				}
				*resultIncludePaths = append(*resultIncludePaths, strings.Trim(args[index], "\""))
				// for debug
				blog.Debugf("cc: response file:%s,got include path:%s", newrspfile, strings.Trim(args[index], "\""))
				continue
			}

			if strings.HasPrefix(arg, "-include") {
				keylen := 8
				if arg == "-include-pch" {
					keylen = 12
				}

				// if -include just a prefix, save the remain of this line.
				if len(arg) > keylen {
					*resultIncludeFiles = append(*resultIncludeFiles, strings.Trim(arg[keylen:], "\""))
					// for debug
					blog.Debugf("cc: response file:%s,got include file:%s", newrspfile, strings.Trim(arg[keylen:], "\""))
					continue
				}

				// if file name is in the next index, then take it.
				index++
				if index >= len(args) {
					blog.Warnf("cc: scan args: no file found after -include or -include-pch")
					return
				}
				*resultIncludeFiles = append(*resultIncludeFiles, strings.Trim(args[index], "\""))
				// for debug
				blog.Debugf("cc: response file:%s,got include file:%s", newrspfile, strings.Trim(args[index], "\""))
				continue
			}
			continue
		} else if strings.HasPrefix(arg, "@") {
			// 递归调用
			scanRspFilesRecursively(
				arg[1:],
				workdir,
				resultIncludePaths,
				resultIncludeFiles,
				checkedRspFiles)
		}
	}

	// for debug
	blog.Debugf("cc: response file:%s,resultIncludePaths:%+v,resultIncludeFiles:%+v",
		newrspfile, *resultIncludePaths, *resultIncludeFiles)
}
