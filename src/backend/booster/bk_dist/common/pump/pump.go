/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package pump

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcEnv "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

const (
	hex = "0123456789abcdef"
)

// New get a new pump client via provided socketed
func New(socketAddr string) (*Client, error) {
	conn, err := net.Dial("unix", socketAddr)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn: conn,
	}, nil
}

// Client provide a handler to do request to distcc-pump include-server
type Client struct {
	conn net.Conn
}

// Analyze do the analyze via include-server
func (c *Client) Analyze(dir string, args []string) ([]string, error) {
	if err := c.writeCwd(dir); err != nil {
		return nil, err
	}

	if err := c.write(args); err != nil {
		return nil, err
	}

	return c.read()
}

func (c *Client) writeCwd(dir string) error {
	if realDir, err := os.Readlink(dir); err == nil {
		dir = realDir
	}

	return c.writeString("CDIR", dir)
}

func (c *Client) writeInt(token string, num int) error {
	buf := []byte(token)

	for i := 28; i >= 0; i -= 4 {
		buf = append(buf, hex[num>>i&0xf])
	}

	_, err := c.conn.Write(buf)
	return err
}

func (c *Client) writeString(token string, args string) error {
	if err := c.writeInt(token, len(args)); err != nil {
		return err
	}

	_, err := c.conn.Write([]byte(args))
	return err
}

func (c *Client) write(args []string) error {
	if err := c.writeInt("ARGC", len(args)); err != nil {
		return err
	}

	for _, arg := range args {
		if err := c.writeString("ARGV", arg); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) readInt(token string) (int, error) {
	data := make([]byte, 12)
	if _, err := c.conn.Read(data); err != nil {
		return 0, err
	}

	if len(data) != 12 {
		return 0, fmt.Errorf("error data: %s", string(data))
	}

	if token != string(data[:4]) {
		return 0, fmt.Errorf("error token from data: %s", string(data))
	}

	length, err := strconv.ParseInt(string(data[4:]), 16, 32)
	return int(length), err
}

func (c *Client) readString(token string) (string, error) {
	length, err := c.readInt(token)
	if err != nil {
		return "", err
	}

	data := make([]byte, length)
	_, err = c.conn.Read(data)

	return string(data), err
}

func (c *Client) read() ([]string, error) {
	length, err := c.readInt("ARGC")
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, 100)
	for i := 0; i < length; i++ {
		r, err := c.readString("ARGV")
		if err != nil {
			return nil, err
		}

		files = append(files, r)
	}

	return files, nil
}

func IsPump(env *env.Sandbox) bool {
	return env.GetEnv(dcEnv.KeyExecutorPump) != ""
}

func SupportPump(env *env.Sandbox) bool {
	return IsPump(env) && (runtime.GOOS == "windows" || runtime.GOOS == "darwin")
	// return IsPump(env) && runtime.GOOS == "windows"
}

func IsPumpCache(env *env.Sandbox) bool {
	return env.GetEnv(dcEnv.KeyExecutorPumpCache) != ""
}

func PumpCorrectCap(env *env.Sandbox) bool {
	return env.GetEnv(dcEnv.KeyExecutorPumpCorrectCap) != ""
}

func PumpCacheDir(env *env.Sandbox) string {
	return env.GetEnv(dcEnv.KeyExecutorPumpCacheDir)
}

func PumpCacheSizeMaxMB(env *env.Sandbox) int32 {
	strsize := env.GetEnv(dcEnv.KeyExecutorPumpCacheSizeMaxMB)
	if strsize != "" {
		size, err := strconv.Atoi(strsize)
		if err != nil {
			return -1
		} else {
			return int32(size)
		}
	}

	return -1
}

func PumpMinActionNum(env *env.Sandbox) int32 {
	strsize := env.GetEnv(dcEnv.KeyExecutorPumpMinActionNum)
	if strsize != "" {
		size, err := strconv.Atoi(strsize)
		if err != nil {
			return 0
		} else {
			return int32(size)
		}
	}

	return 0
}

// 是否支持依赖文件的stat信息的缓存
func SupportPumpStatCache(env *env.Sandbox) bool {
	return env.GetEnv(dcEnv.KeyExecutorPumpDisableStatCache) == ""
}

// 是否支持添加xcode的头文件中的链接文件
func SupportPumpSearchLink(env *env.Sandbox) bool {
	return runtime.GOOS == "darwin" && env.GetEnv(dcEnv.KeyExecutorPumpSearchLink) != ""
}

func SaveLinkData(data map[string]string, f string) error {
	temparr := make([]string, 0, len(data))
	for k, v := range data {
		temparr = append(temparr, fmt.Sprintf("%s->%s", k, v))
	}

	newdata := strings.Join(temparr, "\n")
	return ioutil.WriteFile(f, []byte(newdata), os.ModePerm)
}

// first map  : symlink->realfile
// second map : realfile->symlink
func ResolveLinkData(f string) (map[string]string, map[string]string, error) {
	data, err := ioutil.ReadFile(f)
	if err != nil {
		blog.Warnf("pump: read link file %s with err:%v", f, err)
		return nil, nil, err
	}

	lines := strings.Split(string(data), "\n")
	link2real := make(map[string]string, len(lines))
	real2link := make(map[string]string, len(lines))
	for _, l := range lines {
		l = strings.Trim(l, " \r\n")
		fields := strings.Split(l, "->")
		if len(fields) == 2 {
			link2real[fields[0]] = fields[1]
			real2link[fields[1]] = fields[0]
		}
	}

	return link2real, real2link, nil
}

func LinkResultFile(env *env.Sandbox) string {
	return env.GetEnv(dcEnv.KeyExecutorPumpSearchLinkResult)
}

// 是否支持通过搜索目录来获取文件的stat信息
func SupportPumpLstatByDir(env *env.Sandbox) bool {
	return env.GetEnv(dcEnv.KeyExecutorPumpLstatByDir) != "" && (runtime.GOOS == "windows" || runtime.GOOS == "darwin")
}

// -----------------------------------------------------------------------

var (
	pchDependLock       sync.RWMutex
	pchDependResultFile string
	pchDepend           map[string]*[]string = make(map[string]*[]string, 1000)
)

func init() {
	pchDependResultFile = ""
	pchDependResultFile = getPchDependResultFile()
	if pchDependResultFile != "" {
		resolvePchDepend(pchDependResultFile)
		return
	}

	// blog.Warnf("pump: not found pch depend result file")
}

func getPchDependResultFile() string {
	pumpdir := env.GetEnv(dcEnv.KeyExecutorPumpCacheDir)
	if pumpdir == "" {
		pumpdir = dcUtil.GetPumpCacheDir()
	}

	if pumpdir != "" {
		return filepath.Join(pumpdir, fmt.Sprintf("pch_depends.txt"))
	}

	return ""
}

// 保存预编译头文件之间的依赖关系
func savePchDepend(data map[string]*[]string, f string) error {
	temparr := make([]string, 0, len(data))
	for k, v := range data {
		for _, v1 := range *v {
			temparr = append(temparr, fmt.Sprintf("%s->%s", k, v1))
		}
	}

	newdata := strings.Join(temparr, "\n")
	return os.WriteFile(f, []byte(newdata), os.ModePerm)
}

// a.pch->b.pch means a depend b
// a.pch->c.pch means a depend c too
func resolvePchDepend(f string) error {
	data, err := os.ReadFile(f)
	if err != nil {
		// blog.Warnf("pump: read pch depend file %s with err:%v", f, err)
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, l := range lines {
		l = strings.Trim(l, " \r\n")
		fields := strings.Split(l, "->")
		if len(fields) == 2 {
			// blog.Infof("pump: resolve get pch depend %s->%s", fields[0], fields[1])
			arr, ok := pchDepend[fields[0]]
			if !ok {
				arr := []string{fields[1]}
				pchDepend[fields[0]] = &arr
			} else {
				*arr = append(*arr, fields[1])
			}
		}
	}

	return nil
}

func SetPchDepend(f string, depends []string) {
	pchDependLock.Lock()
	defer pchDependLock.Unlock()

	changed := false

	arr, ok := pchDepend[f]
	if !ok {
		changed = true
		pchDepend[f] = &depends
	} else {
		for _, v := range depends {
			found := false
			for _, v1 := range *arr {
				if v == v1 {
					found = true
					break
				}
			}
			if !found {
				changed = true
				*arr = append(*arr, v)
			}
		}
	}

	if changed && pchDependResultFile != "" {
		savePchDepend(pchDepend, pchDependResultFile)
	}
}

// 返回依赖列表，需要注意有嵌套关系
func GetPchDepend(f string) []string {
	pchDependLock.RLock()
	defer pchDependLock.RUnlock()

	arr, ok := pchDepend[f]
	if ok {
		result := make([]string, len(*arr))
		copy(result, *arr)
		for _, v := range *arr {
			getPchDepend(v, &result)
		}

		blog.Infof("pump: got pch depend %s->%v", f, result)
		return result
	}

	return nil
}

// 递归获取所有依赖列表
func getPchDepend(f string, result *[]string) {
	arr, ok := pchDepend[f]
	if ok {
		for _, v := range *arr {
			newfile := true
			for _, v1 := range *result {
				if v == v1 {
					newfile = true
					break
				}
			}

			if newfile {
				*result = append(*result, v)
				getPchDepend(v, result)
			}
		}
	}

	return
}
