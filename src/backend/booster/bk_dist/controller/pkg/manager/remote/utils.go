/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package remote

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	dcProtocol "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

func getFileDetailsFromExecuteRequest(req *types.RemoteTaskExecuteRequest) []*types.FilesDetails {
	fd := make([]*types.FilesDetails, 0, len(req.Req.Commands[0].Inputfiles))
	for _, c := range req.Req.Commands {
		for _, f := range c.Inputfiles {
			fd = append(fd, &types.FilesDetails{
				Servers: []*dcProtocol.Host{req.Server},
				File:    f,
			})
		}
	}
	return fd
}

func getMaxSizeFile(req *types.RemoteTaskExecuteRequest, threshold int64) (string, int64) {
	var maxsize int64
	fpath := ""
	for _, c := range req.Req.Commands {
		for _, v := range c.Inputfiles {
			if v.FileSize > maxsize {
				fpath = v.FilePath
				maxsize = v.FileSize
			}
		}
	}

	if maxsize > threshold {
		return fpath, maxsize
	}

	return "", 0
}

// updateTaskRequestInputFilesReady 根据给定的baseDirs, 标记request中对应index的文件为"已经发送", 可以直接使用
func updateTaskRequestInputFilesReady(req *types.RemoteTaskExecuteRequest, baseDirs []string) error {
	index := 0
	for i, c := range req.Req.Commands {
		for j := range c.Inputfiles {
			if index >= len(baseDirs) {
				return fmt.Errorf("baseDirs length not equals to input files")
			}

			baseDir := baseDirs[index]
			if baseDir == "" {
				index++
				continue
			}

			req.Req.Commands[i].Inputfiles[j].FileSize = -1
			req.Req.Commands[i].Inputfiles[j].CompressedSize = -1
			req.Req.Commands[i].Inputfiles[j].Targetrelativepath = baseDir
			index++
		}
	}
	return nil
}

func getFileDetailsFromSendFileRequest(req *types.RemoteTaskSendFileRequest) []*types.FilesDetails {
	fd := make([]*types.FilesDetails, 0, 100)
	for _, f := range req.Req {
		fd = append(fd, &types.FilesDetails{
			Servers: []*dcProtocol.Host{req.Server},
			File:    f,
		})
	}
	return fd
}

func getIPFromServer(server string) string {
	if strings.Count(server, ":") >= 2 { //ipv6
		// The port starts after the last colon.
		i := strings.LastIndex(server, ":")
		return server[:i]
	}
	return strings.Split(server, ":")[0]
}

func workerSideCache(sandbox *dcSyscall.Sandbox) bool {
	if sandbox == nil || sandbox.Env == nil {
		return false
	}
	return sandbox.Env.GetEnv(env.KeyExecutorWorkerSideCache) != ""
}

func isCaredNetError(err error) bool {
	netErr, ok := err.(net.Error)
	if !ok {
		blog.Infof("remote util: error[%v] is not net.Error", err)
		return false
	}

	if netErr.Timeout() {
		blog.Infof("remote util: error[%v] is Timeout()", err)
		return true
	}

	opErr, ok := netErr.(*net.OpError)
	if !ok {
		blog.Infof("remote util: error[%v] is not net.OpError", netErr)
		return false
	} else {
		blog.Infof("remote util: error[%v] is net.OpError[%+v]", err, opErr)
	}

	switch t := opErr.Err.(type) {
	case *net.DNSError:
		blog.Infof("remote util: error[%v] is net.DNSError", opErr.Err)
		return true
	case *os.SyscallError:
		if errno, ok := t.Err.(syscall.Errno); ok {
			blog.Infof("remote util: error[%v] got syscall.Errno[%d]", err, errno)
			switch errno {
			// syscall.WSAECONNABORTED(10053) syscall.WSAECONNRESET(10054)
			case syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED, 10053, 10054:
				return true
			case syscall.ETIMEDOUT:
				return true
			}
		}
	}

	return false
}

// calculateDependencies 计算依赖关系
func calculateDependencies(fileDetails []*types.FilesDetails) [][]int {
	// 初始化依赖列表
	dependencies := make([][]int, 0, len(fileDetails))
	for range fileDetails {
		dependencies = append(dependencies, make([]int, 0, 0))
	}

	// 记录需要排除的依赖关系，比如
	// /a/b/xx1.h -> /a/b/c/xx.h   /a/b/xx2.h -> /a/b/c/xx.h
	// 如果有循环依赖，则链接关系优先，去掉目录依赖
	excludedepend := make(map[int]*[]int)

	// 遍历字符串数组，计算依赖关系
	for i, s1 := range fileDetails {
		for j, s2 := range fileDetails {
			if i != j {
				if dirDepend(s1, s2) {
					dependencies[i] = append(dependencies[i], j)
				} else if linkDepend(s1, s2) {
					dependencies[i] = append(dependencies[i], j)
					if dirDepend(s2, s1) {
						v, ok := excludedepend[j]
						if ok {
							*v = append(*v, i)
						} else {
							excludedepend[j] = &[]int{i}
						}
					}
				}
			}
		}
	}

	// 清掉排除的依赖
	if len(excludedepend) > 0 {
		for k, arrv := range excludedepend {
			for _, v := range *arrv {
				blog.Infof("remote util: clean depend:%s to %s",
					fileDetails[k].File.FilePath,
					fileDetails[v].File.FilePath)
				if len(dependencies[k]) > 0 {
					for i := range dependencies[k] {
						if dependencies[k][i] == v {
							dependencies[k][i] = -1
							blog.Infof("remote util: set depend:%s %d to -1",
								fileDetails[k].File.FilePath, i)
						}
					}
				}
			}
		}
	}

	return dependencies
}

// 判断s2是否是s1的子串
func isSubString(s1, s2 string) bool {
	return len(s1) > len(s2) &&
		strings.HasPrefix(s1, s2) &&
		s2 != "/"
}

// dirDepend 检查 s1的目录 是否依赖 s2的目录
// 优化规则：只有s2是指向目录的链接时，才需要判断依赖关系；
// 因为普通目录（非链接）不影响远端路径的生成；而指向目录的链接，会在保存依赖时提炼出来
func dirDepend(s1, s2 *types.FilesDetails) bool {
	if s2.File.Priority != sdk.LinkDirPriority {
		return false
	}

	if isSubString(s1.File.FilePath, s2.File.FilePath) {
		return true
	}

	return false

	// is1File := s1.File.Priority == sdk.RealFilePriority || s1.File.Priority == sdk.LinkFilePriority
	// is2File := s2.File.Priority == sdk.RealFilePriority || s2.File.Priority == sdk.LinkFilePriority

	// // 如果s1是文件，s2是目录
	// if is1File {
	// 	if is2File { // 如果s1是文件，s2是文件
	// 		if isSubString(s1.File.Targetrelativepath, s2.File.Targetrelativepath) {
	// 			return true
	// 		}
	// 	} else { // 如果s1是文件，s2是目录
	// 		if isSubString(s1.File.FilePath, s2.File.FilePath) {
	// 			return true
	// 		}
	// 	}
	// } else {
	// 	if is2File { // 如果s1是目录，s2是文件
	// 		if isSubString(s1.File.FilePath, s2.File.Targetrelativepath) {
	// 			return true
	// 		}
	// 	} else { // 如果s1是目录，s2是目录
	// 		if isSubString(s1.File.FilePath, s2.File.FilePath) {
	// 			return true
	// 		}
	// 	}
	// }

	// return false
}

// linkDepend 检查 s1 是否链接到了 s2
func linkDepend(s1, s2 *types.FilesDetails) bool {
	if s1.File.Priority != sdk.LinkDirPriority && s1.File.Priority != sdk.LinkFilePriority {
		return false
	}

	if s1.File.LinkTarget != "" && s1.File.LinkTarget == s2.File.FilePath {
		return true
	}

	return false
}

func freshPriority(fileDetails []*types.FilesDetails) error {
	// 得到路径的依赖关系
	dependencies := calculateDependencies(fileDetails)

	// 重置优先级为-1
	for _, v := range fileDetails {
		v.File.Priority = -1
	}

	// 计算权重
	maxPriority := 0
	maxTry := 30
	tryNum := 0
	for {
		tryNum++
		if tryNum >= maxTry {
			printLeftDepend(fileDetails, dependencies)
		}

		allok := true
		for i := range fileDetails {
			// 如果超过遍历次数上限，则剩余的全部赋值，避免死循环
			if tryNum >= maxTry {
				if fileDetails[i].File.Priority < 0 {
					fileDetails[i].File.Priority = sdk.FileDescPriority(maxPriority + 1)
				}
				continue
			}

			// 已经设置了权重
			if fileDetails[i].File.Priority >= 0 {
				continue
			}

			if len(dependencies[i]) == 0 {
				if fileDetails[i].File.Priority < 0 {
					fileDetails[i].File.Priority = 0
				}
			} else {
				maxDependPriority := -1
				alldependok := true
				for j, v := range dependencies[i] {
					if v < 0 {
						continue
					}
					dependPriority := int(fileDetails[v].File.Priority)
					if dependPriority >= 0 {
						dependencies[i][j] = -1
						if dependPriority > maxDependPriority {
							maxDependPriority = dependPriority
						}
						if dependPriority > maxPriority {
							maxPriority = dependPriority
						}
					} else {
						blog.Debugf("remote util: %s wait %s",
							fileDetails[i].File.FilePath, fileDetails[v].File.FilePath)
						alldependok = false
					}
				}
				if alldependok {
					dependencies[i] = nil
					fileDetails[i].File.Priority = sdk.FileDescPriority(maxDependPriority + 1)
					blog.Debugf("remote util: %s set Priority to %d",
						fileDetails[i].File.FilePath, maxDependPriority+1)
				} else {
					allok = false
				}
			}
		}

		if allok || tryNum >= maxTry {
			blog.Infof("remote util: finished set Priority after %d try", tryNum)
			break
		}
	}

	return nil
}

func printLeftDepend(fileDetails []*types.FilesDetails, dependencies [][]int) {
	for i := range fileDetails {
		if fileDetails[i].File.Priority < 0 {
			dependfiles := []string{}
			for _, v := range dependencies[i] {
				if v >= 0 {
					dependPriority := int(fileDetails[v].File.Priority)
					if dependPriority < 0 {
						dependfiles = append(dependfiles, fileDetails[v].File.FilePath)
					}
				}
			}
			blog.Warnf("remote util: after max try, %s wait %s",
				fileDetails[i].File.FilePath,
				strings.Join(dependfiles, " "))
		}
	}
}
