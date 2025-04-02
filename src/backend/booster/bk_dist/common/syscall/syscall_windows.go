//go:build windows
// +build windows

/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package syscall

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

const (
	ExitErrorCode            = 99
	DevOPSProcessTreeKillKey = "DEVOPS_DONT_KILL_PROCESS_TREE"
)

// RunServer run the detached server
func RunServer(command string) error {
	argv := syscall.StringToUTF16Ptr(command)
	var sI syscall.StartupInfo
	var pI syscall.ProcessInformation

	blog.Infof("syscall: ready to run cmd [%s]", command)
	err := syscall.CreateProcess(
		nil,
		argv,
		nil,
		nil,
		false,
		0x00000008|0x00000200|0x08000000|syscall.CREATE_UNICODE_ENVIRONMENT, // https://docs.microsoft.com/en-us/windows/win32/procthread/process-creation-flags
		createEnvBlock(append(os.Environ(), fmt.Sprintf("%s=%s", DevOPSProcessTreeKillKey, "true"))),
		nil,
		&sI,
		&pI)
	if err != nil {
		blog.Errorf("syscall: run server error: %v", err)
	}
	return err
}

func createEnvBlock(envv []string) *uint16 {
	if len(envv) == 0 {
		return &utf16.Encode([]rune("\x00\x00"))[0]
	}
	length := 0
	for _, s := range envv {
		length += len(s) + 1
	}
	length += 1

	b := make([]byte, length)
	i := 0
	for _, s := range envv {
		l := len(s)
		copy(b[i:i+l], []byte(s))
		copy(b[i+l:i+l+1], []byte{0})
		i = i + l + 1
	}
	copy(b[i:i+1], []byte{0})

	return &utf16.Encode([]rune(string(b)))[0]
}

// GetSysProcAttr return an empty syscall.SysProcAttr
func GetSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// GetCallerAndOptions return the caller and options in windows
func GetCallerAndOptions() (string, string) {
	fullcmd := "C:\\Windows\\System32\\cmd.exe"
	if dcFile.Stat(fullcmd).Exist() {
		return fullcmd, "/C"
	}

	return "cmd", "/C"
}

// GetDir return the running dir
func (s *Sandbox) GetDir() string {
	if s.Dir != "" {
		return s.Dir
	}

	p, _ := os.Getwd()
	return p
}

// GetAbsPath return the abs path related to current running dir
func (s *Sandbox) GetAbsPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}

	return filepath.Join(s.GetDir(), path)
}

func formatArg(arg string) string {
	if arg != "" && strings.HasPrefix(arg, "\"") && strings.HasSuffix(arg, "\"") {
		return strings.Trim(arg, "\"")
	}

	return arg
}

// Sandbox describe the handler to build up an isolated execution environment
type Sandbox struct {
	Ctx    context.Context
	Env    *env.Sandbox
	Dir    string
	User   user.User
	Stdout io.Writer
	Stderr io.Writer
	spa    *syscall.SysProcAttr
}

// Fork return a new sandbox which inherits from current one
func (s *Sandbox) Fork() *Sandbox {
	return &Sandbox{
		Ctx:    s.Ctx,
		Env:    s.Env,
		Dir:    s.Dir,
		Stdout: s.Stdout,
		Stderr: s.Stderr,
	}
}

// ExecScripts run the scripts
func (s *Sandbox) ExecScripts(src string) (int, error) {
	blog.Infof("sanbox:ready exec script:%s", src)

	caller, options := GetCallerAndOptions()

	s.spa = &syscall.SysProcAttr{
		CmdLine:    fmt.Sprintf("%s %s", options, src),
		HideWindow: true,
	}
	return s.execCommand(caller)
}

func (s *Sandbox) ExecScriptsRaw(src string) (int, error) {
	blog.Infof("sanbox:ready exec raw script:%s", src)

	caller, _ := GetCallerAndOptions()

	s.spa = &syscall.SysProcAttr{
		CmdLine:    src,
		HideWindow: true,
	}
	return s.execCommand(caller)
}

// ExecCommand run the origin commands by file
func (s *Sandbox) ExecRawByFile(bt, name string, arg ...string) (int, error) {
	fullArgs := strings.Join(arg, " ")
	argsFile, err := os.CreateTemp(GetHandlerTmpDir(s, bt), "args-*.txt")
	if err != nil {
		blog.Errorf("sanbox: exec raw script in file failed to create tmp file, error:%s", err.Error())
		return -1, err
	}
	blog.Infof("sanbox:ready exec raw script in file %s with arg len %d", argsFile.Name(), len(arg))
	err = os.WriteFile(argsFile.Name(), []byte(fullArgs), os.ModePerm)
	if err != nil {
		argsFile.Close() // 关闭文件
		blog.Errorf("sasanbox: exec raw script in file failed to write tmp file %s, error:%s", argsFile.Name(), err.Error())
		return -1, err
	}
	argsFile.Close() // 关闭文件
	code, err := s.execCommand(name, "@"+argsFile.Name())
	if err != nil {
		blog.Errorf("sanbox: exec raw script in file failed to exec command [%s] %s in file (%s) , error:%s", name, fullArgs, argsFile.Name(), err.Error())
		return code, err
	}
	blog.Infof("sanbox: success to exec raw script in file %s, delete the arg file now", argsFile.Name())
	os.Remove(argsFile.Name())
	return code, err
}

// GetHandlerTmpDir get temp dir by booster type
func GetHandlerTmpDir(sandBox *Sandbox, bt string) string {
	var baseTmpDir string
	if sandBox == nil {
		baseTmpDir = os.TempDir()
	} else {
		if baseTmpDir = sandBox.Env.GetOriginEnv("TMPDIR"); baseTmpDir == "" {
			baseTmpDir = os.TempDir()
		}
	}

	if baseTmpDir != "" {
		fullTmpDir := path.Join(baseTmpDir, protocol.BKDistDir, bt)
		if !dcFile.Stat(fullTmpDir).Exist() {
			if err := os.MkdirAll(fullTmpDir, os.ModePerm); err != nil {
				blog.Warnf("common util: create tmp dir failed with error:%v", err)
				return ""
			}
		}
		return fullTmpDir
	}

	return ""
}

// ExecScriptsWithMessage run the scripts and return the output
func (s *Sandbox) ExecScriptsWithMessage(src string) (int, []byte, []byte, error) {
	caller, options := GetCallerAndOptions()

	s.spa = &syscall.SysProcAttr{
		CmdLine:    fmt.Sprintf("%s %s", options, src),
		HideWindow: true,
	}

	var outBuf, errBuf bytes.Buffer
	s.Stdout = &outBuf
	s.Stderr = &errBuf

	code, err := s.execCommand(caller)
	if err != nil && code != ExitErrorCode && len(errBuf.Bytes()) == 0 {
		return code, outBuf.Bytes(), []byte(err.Error()), err
	}

	return code, outBuf.Bytes(), errBuf.Bytes(), err
}

// StartScripts start the scripts, not wait
func (s *Sandbox) StartScripts(src string) (*exec.Cmd, error) {
	caller, options := GetCallerAndOptions()

	s.spa = &syscall.SysProcAttr{
		CmdLine:    fmt.Sprintf("%s %s", options, src),
		HideWindow: true,
	}
	return s.startCommand(caller)
}

// ExecCommandWithMessage run the commands and get the stdout and stderr
func (s *Sandbox) ExecCommandWithMessage(name string, arg ...string) (int, []byte, []byte, error) {
	var outBuf, errBuf bytes.Buffer
	s.Stdout = &outBuf
	s.Stderr = &errBuf

	// if has space, quoto name
	name4CmdLine := name
	if !strings.HasPrefix(name, "\"") {
		hasspace := false
		for _, v := range name {
			if v == ' ' {
				hasspace = true
			}
		}
		if hasspace {
			name4CmdLine = "\"" + name + "\""
		}
	}

	s.spa = &syscall.SysProcAttr{
		CmdLine:    fmt.Sprintf("%s %s", name4CmdLine, strings.Join(arg, " ")),
		HideWindow: true,
	}

	code, err := s.execCommand(name, arg...)
	if err != nil && code != ExitErrorCode && len(errBuf.Bytes()) == 0 {
		return code, outBuf.Bytes(), []byte(err.Error()), err
	}

	return code, outBuf.Bytes(), errBuf.Bytes(), err
}

// ExecCommand run the origin commands
func (s *Sandbox) ExecCommand(name string, arg ...string) (int, error) {
	fArg := make([]string, 0, 100)
	for _, v := range arg {
		fArg = append(fArg, formatArg(v))
	}
	return s.execCommand(name, fArg...)
}

func (s *Sandbox) execCommand(name string, arg ...string) (int, error) {
	blog.Infof("sanbox:ready run cmd:%s %v", name, arg)

	if s.Env == nil {
		s.Env = env.NewSandbox(os.Environ())
	}

	if s.User.Username == "" {
		if u, _ := user.Current(); u != nil {
			s.User = *u
		}
	}

	if s.Stdout == nil {
		s.Stdout = os.Stdout
	}
	if s.Stderr == nil {
		s.Stderr = os.Stderr
	}

	if s.spa == nil {
		s.spa = &syscall.SysProcAttr{
			HideWindow: true,
		}
	}

	var err error
	// if not relative path find the command in PATH
	if !strings.HasPrefix(name, ".") {
		name, err = s.LookPath(name)

		if err != nil {
			blog.Infof("sanbox:LookPath %s with error:%v", name, err)
		}
	}

	var cmd *exec.Cmd
	if s.Ctx != nil {
		cmd = exec.CommandContext(s.Ctx, name, arg...)
	} else {
		cmd = exec.Command(name, arg...)
	}

	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	cmd.Env = s.Env.Source()
	cmd.Dir = s.Dir
	cmd.SysProcAttr = s.spa

	// 错误等到stdout和stderr都初始化完, 再处理
	if err != nil {
		_, _ = s.Stderr.Write([]byte(fmt.Sprintf("run command failed: %v\n", err.Error())))
		return -1, err
	}

	if err := cmd.Run(); err != nil {
		// blog.Infof("sanbox:run cmd:%+v with error:%v", cmd, err)
		blog.Infof("sanbox:run cmd:%+v with error:%v,spa:%+v\n", *cmd, err, *cmd.SysProcAttr)

		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				return status.ExitStatus(), err
			}
		}
		return ExitErrorCode, err
	}

	return 0, nil
}

func (s *Sandbox) startCommand(name string, arg ...string) (*exec.Cmd, error) {
	if s.Env == nil {
		s.Env = env.NewSandbox(os.Environ())
	}

	if s.User.Username == "" {
		if u, _ := user.Current(); u != nil {
			s.User = *u
		}
	}

	if s.Stdout == nil {
		s.Stdout = os.Stdout
	}
	if s.Stderr == nil {
		s.Stderr = os.Stderr
	}

	if s.spa == nil {
		s.spa = &syscall.SysProcAttr{
			HideWindow: true,
		}
	}

	var err error
	// if not relative path find the command in PATH
	if !strings.HasPrefix(name, ".") {
		name, err = s.LookPath(name)
	}

	var cmd *exec.Cmd
	if s.Ctx != nil {
		cmd = exec.CommandContext(s.Ctx, name, arg...)
	} else {
		cmd = exec.Command(name, arg...)
	}

	cmd.Stdout = s.Stdout
	cmd.Stderr = s.Stderr
	cmd.Env = s.Env.Source()
	cmd.Dir = s.Dir
	cmd.SysProcAttr = s.spa

	// 错误等到stdout和stderr都初始化完, 再处理
	if err != nil {
		_, _ = s.Stderr.Write([]byte(fmt.Sprintf("run command failed: %v\n", err.Error())))
		return cmd, err
	}

	if err := cmd.Start(); err != nil {
		//if _, ok := err.(*exec.ExitError); ok {
		//if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
		//return cmd, err
		//}
		//}
		blog.Errorf("syscall: failed to start cmd with error: %v", err)
		return cmd, err
	}

	return cmd, nil
}

// LookPath 根据sandbox中的env-PATH, 来取得正确的command-name路径
func (s *Sandbox) LookPath(file string) (string, error) {
	var exts []string
	x := s.Env.GetOriginEnv(`PATHEXT`)
	if x != "" {
		for _, e := range strings.Split(strings.ToLower(x), `;`) {
			if e == "" {
				continue
			}
			if e[0] != '.' {
				e = "." + e
			}
			exts = append(exts, e)
		}
	} else {
		exts = []string{".com", ".exe", ".bat", ".cmd"}
	}

	if strings.ContainsAny(file, `:\/`) {
		if f, err := findExecutable(file, exts); err == nil {
			return f, nil
		} else {
			return "", fmt.Errorf("command %s not found", file)
		}
	}
	if f, err := findExecutable(filepath.Join(".", file), exts); err == nil {
		return f, nil
	}
	path := s.Env.GetOriginEnv("path")
	for _, dir := range filepath.SplitList(path) {
		if f, err := findExecutable(filepath.Join(dir, file), exts); err == nil {
			return f, nil
		}
	}
	return "", fmt.Errorf("command %s not found", file)
}

func findExecutable(file string, exts []string) (string, error) {
	if len(exts) == 0 {
		return file, chkStat(file)
	}
	if hasExt(file) {
		if chkStat(file) == nil {
			return file, nil
		}
	}
	for _, e := range exts {
		if f := file + e; chkStat(f) == nil {
			return f, nil
		}
	}
	return "", os.ErrNotExist
}

func chkStat(file string) error {
	d, err := os.Stat(file)
	if err != nil {
		return err
	}
	if d.IsDir() {
		return os.ErrPermission
	}
	return nil
}

func hasExt(file string) bool {
	i := strings.LastIndex(file, ".")
	if i < 0 {
		return false
	}
	return strings.LastIndexAny(file, `:\/`) < i
}

// GetConsoleCP call GetConsoleCP of windows, 0 means failed
func GetConsoleCP() int {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		fmt.Println(err)
		return 0
	}
	defer syscall.FreeLibrary(kernel32)

	// https://docs.microsoft.com/en-us/windows/console/getconsolecp
	api, err := syscall.GetProcAddress(kernel32, "GetConsoleCP")
	if err != nil {
		fmt.Println(err)
		return 0
	}

	code, _, _ := syscall.Syscall(uintptr(api), 0, 0, 0, 0)
	return int(code)
}

func AddPath2Env(p string) {
	path := os.Getenv("path")
	newpath := fmt.Sprintf("%s;%s", p, path)
	os.Setenv("path", newpath)
}

func SetStdHandle(stdhandle int32, handle syscall.Handle) error {
	kernel32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer syscall.FreeLibrary(kernel32)

	api, err := syscall.GetProcAddress(kernel32, "SetStdHandle")
	if err != nil {
		fmt.Println(err)
		return err
	}

	r0, _, e1 := syscall.Syscall(uintptr(api), 2, uintptr(stdhandle), uintptr(handle), 0)
	if r0 == 0 {
		if e1 != 0 {
			return error(e1)
		}
		return syscall.EINVAL
	}
	return nil
}

func RedirectStderror(f string) error {
	var err error
	file, err := os.OpenFile(f, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println(err)
		return err
	}
	err = SetStdHandle(syscall.STD_ERROR_HANDLE, syscall.Handle(file.Fd()))
	if err != nil {
		fmt.Println(err)
		return err
	}

	return nil
}

func NeedSearchToolchain(input *env.Sandbox) bool {
	return false
}
