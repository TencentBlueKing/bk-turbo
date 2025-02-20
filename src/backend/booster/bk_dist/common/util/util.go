/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package util

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/denisbrodbeck/machineid"

	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"

	"github.com/pierrec/lz4"
	"github.com/shirou/gopsutil/process"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	commonUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/util"
)

// GetCaller return the current caller functions
func GetCaller() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "???"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}

	return fmt.Sprintf("%s:%d", file, line)
}

// GetHomeDir get home dir by system env
func GetHomeDir() string {
	homeDir, _ := os.UserHomeDir()

	return homeDir
}

// GetGlobalDir
func GetGlobalDir() string {
	if runtime.GOOS == "windows" {
		return "C:\\bk_dist"
	}

	if runtime.GOOS == "darwin" {
		return GetRuntimeDir()
	}

	return "/etc/bk_dist"
}

// GetLogsDir get the runtime log dir
func GetLogsDir() string {
	dir := path.Join(GetRuntimeDir(), "logs")
	_ = os.MkdirAll(dir, os.ModePerm)
	return dir
}

// GetPumpCacheDir get the runtime pump cache dir
func GetPumpCacheDir() string {
	dir := path.Join(GetRuntimeDir(), "pump_cache")
	_ = os.MkdirAll(dir, os.ModePerm)
	return dir
}

// GetRecordDir get the record dir
func GetRecordDir() string {
	dir := filepath.Join(GetGlobalDir(), "record")
	_ = os.MkdirAll(dir, os.ModePerm)
	return dir
}

// GetRuntimeDir get the runtime tmp dir
func GetRuntimeDir() string {
	dir := path.Join(GetHomeDir(), protocol.BKDistDir)
	_ = os.MkdirAll(dir, os.ModePerm)
	return dir
}

// PrintLevel define log level
type PrintLevel int32

// define log levels
const (
	PrintFatal PrintLevel = iota
	PrintError
	PrintWarn
	PrintInfo
	PrintDebug
	PrintNothing
)

var (
	printlevel2StringMap = map[PrintLevel]string{
		PrintFatal:   "fatal",
		PrintError:   "error",
		PrintWarn:    "warn",
		PrintInfo:    "info",
		PrintDebug:   "debug",
		PrintNothing: "nothing",
	}
)

// String return the string of PrintLevel
func (p PrintLevel) String() string {
	if v, ok := printlevel2StringMap[p]; ok {
		return v
	}

	return "unknown"
}

// Lz4Compress to implement lz4 compress
func Lz4Compress(src []byte) ([]byte, error) {
	if src == nil || len(src) == 0 {
		return nil, fmt.Errorf("src is invalid")
	}

	dst := make([]byte, lz4.CompressBlockBound(len(src)))
	if dst == nil || len(dst) == 0 {
		return nil, fmt.Errorf("failed to allocate memory for compress dst")
	}

	compressedsize, err := lz4.CompressBlock(src, dst, nil)
	if err != nil {
		return nil, err
	}

	return dst[:compressedsize], nil
}

// Lz4Uncompress to implement lz4 uncompress
func Lz4Uncompress(src []byte, dst []byte) ([]byte, error) {
	if src == nil || len(src) == 0 {
		return nil, fmt.Errorf("src is empty")
	}

	if dst == nil || len(dst) == 0 {
		return nil, fmt.Errorf("dst is empty")
	}

	uncompressedsize, err := lz4.UncompressBlock(src, dst)
	if err != nil {
		return nil, err
	}

	return dst[:uncompressedsize], nil
}

// CheckExecutable check executable is exist
func CheckExecutable(target string) (string, error) {
	return CheckExecutableWithPath(target, "", "")
}

// CheckExecutableWithPath check executable is exist
// if path is set, then will look from it,
// if path is empty, then will look from env PATH
// pathExt define the PATHEXT in windows
func CheckExecutableWithPath(target, path, pathExt string) (string, error) {
	absPath, err := LookPath(target, path, pathExt)
	if err != nil {
		return "", err
	}

	return absPath, nil
}

// try search file in caller path
func CheckFileWithCallerPath(target string) (string, error) {
	callpath := GetExcPath()
	newtarget := filepath.Join(callpath, target)
	if !dcFile.Stat(newtarget).Exist() {
		return "", fmt.Errorf("not found controller[%s]", newtarget)
	}

	absPath, err := filepath.Abs(newtarget)
	if err != nil {
		return "", err
	}

	return absPath, nil
}

// Now get the current time
func Now(t *time.Time) {
	*t = now()
}

func now() time.Time {
	return time.Now().Local()
}

// Environ return the current environment variables in map
func Environ() map[string]string {
	items := make(map[string]string)
	for _, i := range os.Environ() {
		key, val := func(item string) (key, val string) {
			splits := strings.Split(item, "=")
			key = splits[0]
			val = splits[1]
			return
		}(i)
		items[key] = val
	}
	return items
}

// GetExcPath get current Exec caller and return the dir
func GetExcPath() string {
	file, _ := exec.LookPath(os.Args[0])
	p, err := filepath.Abs(file)
	if err != nil {
		return ""
	}

	return filepath.Dir(p)
}

// ProjectSetting describe the project setting
type ProjectSetting struct {
	ProjectID string `json:"project_id"`
}

// SearchProjectID try get project id
func SearchProjectID() string {
	exepath := GetExcPath()
	if exepath != "" {
		jsonfile := filepath.Join(exepath, "bk_project_setting.json")
		if dcFile.Stat(jsonfile).Exist() {
			data, err := ioutil.ReadFile(jsonfile)
			if err != nil {
				blog.Debugf("util: failed to read file[%s]", jsonfile)
				return ""
			}

			var setting ProjectSetting
			if err = codec.DecJSON(data, &setting); err != nil {
				blog.Debugf("util: failed to decode file[%s]", jsonfile)
				return ""
			}

			return setting.ProjectID
		}
	}
	return ""
}

// Utf8ToGbk transfer utf-8 to gbk
func Utf8ToGbk(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), simplifiedchinese.GBK.NewEncoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

// ProcessExist check if target process exsit
func ProcessExist(target string) bool {
	processes, err := process.Processes()
	if err != nil {
		return false
	}

	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			continue
		}

		if target == name {
			return true
		}
	}

	return false
}

// ListProcess list process by process name
func ListProcess(target string) ([]*process.Process, error) {
	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	procs := []*process.Process{}
	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			continue
		}

		if target == name {
			procs = append(procs, p)
		}
	}

	return procs, nil
}

// ProcessExistTimeoutAndKill check target process with name, and kill it after timeout
func ProcessExistTimeoutAndKill(target string, timeout time.Duration) bool {
	processes, err := process.Processes()
	if err != nil {
		return false
	}

	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			continue
		}

		createTime, err := p.CreateTime()
		if err != nil {
			continue
		}

		if target == name && time.Unix(createTime/1000, 0).Local().Add(timeout).Before(time.Now().Local()) {
			if err = p.Kill(); err != nil {
				blog.Warnf("util: process %s by pid %d killed failed: %v", name, p.Pid, err)
			}

			blog.Infof("util: process %s by pid %d trying to kill", name, p.Pid)
			return true
		}
	}

	return false
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

// QuoteSpacePath quote path if include space
func QuoteSpacePath(s string) string {
	if s == "" {
		return ""
	}

	if !hasSpace(s) {
		return s
	}

	if s[0] == '"' || s[0] == '\'' {
		return s
	}

	return "\"" + s + "\""
}

func UniqID() string {
	id, err := machineid.ID()
	if err == nil {
		return id
	}

	ips := util.GetIPAddress()
	if len(ips) > 0 {
		return ips[0]
	}

	return fmt.Sprintf("uniq_%s_%d",
		commonUtil.RandomString(5),
		time.Now().Local().UnixNano())
}

//-----------------------------------------------------------------------

var (
	commonTargetPathSep = string(filepath.Separator)
	commonInitPathSep1  = ""
	commonInitPathSep2  = ""
)

var (
	pathmapLock sync.RWMutex
	pathmap     map[string]string = make(map[string]string, 10000)
)

func init() {
	if runtime.GOOS == "windows" {
		commonInitPathSep1 = "/"
		commonInitPathSep2 = "\\\\"
	} else {
		commonInitPathSep1 = "\\"
		commonInitPathSep2 = "//"
	}
}

// 在指定目录下找到正确的文件名（大小写）
func getWindowsRealName(inputdir, inputname string) (string, error) {
	files, err := os.ReadDir(inputdir)
	if err != nil {
		fmt.Printf("check dir:%s with error:%v\r\n", inputdir, err)
		return "", err
	}
	for _, file := range files {
		if strings.EqualFold(file.Name(), inputname) {
			return FormatFilePath(filepath.Join(inputdir, file.Name())), nil
		}
	}

	return "", fmt.Errorf("%s not exist", inputname)
}

func getPath(inputPath string) (string, bool) {
	pathmapLock.RLock()
	newpath, ok := pathmap[inputPath]
	pathmapLock.RUnlock()

	return newpath, ok
}

func putPath(oldPath, newPath string) {
	pathmapLock.Lock()
	pathmap[oldPath] = newPath
	pathmapLock.Unlock()

	return
}

// 根据指定的完整路径得到正确的大小写的完整路径
func getWindowsFullRealPath(inputPath string) (string, error) {
	newpath, ok := getPath(inputPath)
	if ok {
		return newpath, nil
	}

	// 先检查目录是否在缓存，如果在，只需要检查文件名
	realPath := inputPath
	inputdir, inputfile := filepath.Split(inputPath)
	inputdir = strings.TrimRight(inputdir, commonTargetPathSep)

	newpath, ok = getPath(inputdir)
	if ok {
		newPath, err := getWindowsRealName(newpath, inputfile)
		if err == nil {
			realPath = newPath
			// 将结果记录下来
			putPath(inputPath, realPath)
			return realPath, nil
		} else {
			// 将结果记录下来
			putPath(inputPath, inputPath)
			return inputPath, err
		}
	}

	// 完整目录逐级检查，并将逐级目录放入缓存
	parts := strings.Split(inputPath, commonTargetPathSep)
	oldpath := []string{}
	if len(parts) > 0 {
		oldpath = append(oldpath, parts[0])

		realPath = parts[0]
		if strings.HasSuffix(realPath, ":") {
			realPath = strings.ToUpper(realPath + commonTargetPathSep)
		}

		for i, part := range parts {
			if i == 0 {
				continue
			}
			if part == "" {
				continue
			}

			oldpath = append(oldpath, part)

			files, err := os.ReadDir(realPath)
			if err != nil {
				fmt.Printf("check dir:%s with error:%v\r\n", realPath, err)
				// 将结果记录下来
				putPath(inputPath, inputPath)
				return inputPath, err
			}

			for _, file := range files {
				if strings.EqualFold(file.Name(), part) {
					realPath = filepath.Join(realPath, file.Name())

					// 将过程中的路径记录下来
					putPath(strings.Join(oldpath, commonTargetPathSep), FormatFilePath(realPath))

					break
				}
			}
		}
	}

	// 将结果记录下来
	realPath = FormatFilePath(realPath)
	putPath(inputPath, realPath)

	return realPath, nil
}

func CorrectPathCap(inputPaths []string) ([]string, error) {
	newpaths := make([]string, 0, len(inputPaths))
	for _, oldpath := range inputPaths {
		newpath, err := getWindowsFullRealPath(oldpath)
		if err == nil {
			newpaths = append(newpaths, newpath)
		} else {
			newpaths = append(newpaths, oldpath)
		}
	}

	return newpaths, nil
}

func FormatFilePath(f string) string {
	f = strings.Replace(f, commonInitPathSep1, commonTargetPathSep, -1)
	f = strings.Replace(f, commonInitPathSep2, commonTargetPathSep, -1)

	// 去掉路径中的..
	if strings.Contains(f, "..") {
		p := strings.Split(f, commonTargetPathSep)

		var newPath []string
		for _, v := range p {
			if v == ".." {
				newPath = newPath[:len(newPath)-1]
			} else {
				newPath = append(newPath, v)
			}
		}
		f = strings.Join(newPath, commonTargetPathSep)
	}

	return f
}

func GetAllLinkFiles(f string) []string {
	fs := []string{}
	tempf := f
	// avoid dead loop
	maxTry := 10
	try := 0
	for {
		loopagain := false
		i := dcFile.Lstat(tempf)
		if i.Exist() && i.Basic().Mode()&os.ModeSymlink != 0 {
			originFile, err := os.Readlink(tempf)
			if err == nil {
				if !filepath.IsAbs(originFile) {
					originFile, _ = filepath.Abs(filepath.Join(filepath.Dir(tempf), originFile))
				}
				fs = append(fs, FormatFilePath(originFile))

				loopagain = true
				tempf = originFile

				try++
				if try >= maxTry {
					loopagain = false
					blog.Infof("common util: symlink %s may be drop in dead loop", tempf)
					break
				}
			}
		}

		if !loopagain {
			break
		}
	}

	return fs
}

func UniqArr(arr []string) []string {
	newarr := make([]string, 0)
	tempMap := make(map[string]bool, len(newarr))
	for _, v := range arr {
		if tempMap[v] == false {
			tempMap[v] = true
			newarr = append(newarr, v)
		}
	}

	return newarr
}

func getSubdirs(path string) []string {
	var subdirs []string

	// 循环获取每级子目录
	for {
		subdirs = append([]string{path}, subdirs...)
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}

	return subdirs
}

// 获取依赖文件的路径中是链接的路径
func GetAllLinkDir(files []string) []string {
	dirs := make([]string, 0, len(files))
	for _, f := range files {
		dirs = append(dirs, filepath.Dir(f))
	}

	uniqdirs := UniqArr(dirs)
	if len(uniqdirs) > 0 {
		subdirs := []string{}
		for _, d := range uniqdirs {
			subdirs = append(subdirs, getSubdirs(d)...)
		}

		uniqsubdirs := UniqArr(subdirs)
		blog.Infof("common util: got all uniq sub dirs:%v", uniqsubdirs)

		linkdirs := []string{}
		for _, d := range uniqsubdirs {
			i := dcFile.Lstat(d)
			if i.Exist() && i.Basic().Mode()&os.ModeSymlink != 0 {
				fs := GetAllLinkFiles(d)
				if len(fs) > 0 {
					for i := len(fs) - 1; i >= 0; i-- {
						linkdirs = append(linkdirs, fs[i])
					}
					linkdirs = append(linkdirs, d)
				}
			}
		}

		blog.Infof("common util: got all link sub dirs:%v", linkdirs)
		return linkdirs
	}

	return nil
}

// GetResultCacheDir get the runtime result cache dir
func GetResultCacheDir() string {
	dir := path.Join(GetRuntimeDir(), "result_cache")
	_ = os.MkdirAll(dir, os.ModePerm)
	return dir
}

func HashFile(f string) (uint64, error) {
	if !dcFile.Stat(f).Exist() {
		return 0, fmt.Errorf("%s not existed", f)
	}

	file, err := os.Open(f)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	d := xxhash.New()
	if _, err := io.Copy(d, file); err != nil {
		return 0, err
	}

	return d.Sum64(), nil
}
