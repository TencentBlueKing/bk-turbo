/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package sdk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"

	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// type ToolChainKey string

// const (
// 	ToolChainKeyUE4Shader  ToolChainKey = "toolchain_key_ue4_shader"
// 	ToolChainKeyUE4Compile ToolChainKey = "toolchain_key_ue4_compile"
// )

// func (t ToolChainKey) String() string {
// 	return string(t)
// }

// GetJsonToolChainKey get key from original full exe path, fit json format
func GetJsonToolChainKey(origiralfullexepath string) string {
	return strings.Replace(origiralfullexepath, "\\", "/", -1)
}

// ToolFile describe tool file target
type ToolFile struct {
	LocalFullPath      string `json:"local_full_path"`
	RemoteRelativePath string `json:"remote_relative_path"`
}

// OneToolChain describe the single tool chain info
type OneToolChain struct {
	ToolKey                string     `json:"tool_key"`
	ToolName               string     `json:"tool_name"`
	ToolLocalFullPath      string     `json:"tool_local_full_path"`
	ToolRemoteRelativePath string     `json:"tool_remote_relative_path"`
	Files                  []ToolFile `json:"files"`
}

// Toolchain describe the toolchains
type Toolchain struct {
	Toolchains []OneToolChain `json:"toolchains"`
}

// GetToolchainEnvValue get the generated env value
func (t *Toolchain) GetToolchainEnvValue() (string, error) {
	if t == nil {
		return "", fmt.Errorf("tool chain is nil")
	}

	envvalue := ""
	isfirst := true
	for _, v := range t.Toolchains {
		if !isfirst {
			envvalue += ";"
		}

		envvalue += fmt.Sprintf("%s|%s", v.ToolName, v.ToolRemoteRelativePath)

		if isfirst {
			isfirst = false
		}
	}

	return envvalue, nil
}

// ResolveToolchainEnvValue receive generated env value and return the k-v map
func ResolveToolchainEnvValue(value string) (map[string]string, error) {
	if value == "" {
		return nil, fmt.Errorf("env value is empty")
	}

	outmap := map[string]string{}
	envs := strings.Split(value, ";")
	for _, v := range envs {
		kv := strings.Split(v, "|")
		if len(kv) == 2 {
			outmap[kv[0]] = kv[1]
		}
	}
	return outmap, nil
}

func checkAndAdd(i *dcFile.Info, remotepath string, files *[]FileDesc) error {
	f := FileDesc{
		FilePath:           i.Path(),
		Compresstype:       protocol.CompressLZ4,
		FileSize:           i.Size(),
		Lastmodifytime:     i.ModifyTime64(),
		Md5:                "",
		Targetrelativepath: remotepath,
		Filemode:           i.Mode32(),
		LinkTarget:         i.LinkTarget,
		Priority:           GetPriority(i),
	}

	if i.LinkTarget == "" {
		*files = append(*files, f)
		return nil
	}

	// 检查链接是否存在循环
	for _, v := range *files {
		if v.FilePath == i.Path() {
			return fmt.Errorf("found loop link file:%s", v.FilePath)
		}
	}

	*files = append(*files, f)
	return nil
}

// 判断符号链接是否指向网络地址
func isNetworkLink(target string) (bool, error) {
	// 1. 转换为小写便于检查（保留原始值用于UNC检查）
	lowerTarget := strings.ToLower(target)

	// 2. 检查网络标识特征
	switch {
	case strings.Contains(lowerTarget, "://"): // 包含协议标识符
		return true, nil
	case strings.HasPrefix(target, "\\\\"): // Windows UNC路径 (原始大小写)
		return true, nil
	}

	return false, nil
}

// 得到所有关联文件；如果是链接，则递归搜索，直到找到非链接为止
// 如果发现链接循环，则报错
func getRecursiveFiles(f string, remotepath string, files *[]FileDesc) error {
	// 如果远端路径和本地不一致，则需要将链接替换为真实文件发送过去
	localdir := filepath.Dir(f)
	if localdir != remotepath {
		i := dcFile.Stat(f)
		if !i.Exist() {
			err := fmt.Errorf("file %s not existed", f)
			blog.Errorf("%v", err)
			return err
		}

		return checkAndAdd(i, remotepath, files)
	}

	i := dcFile.Lstat(f)
	if !i.Exist() {
		err := fmt.Errorf("file %s not existed", f)
		blog.Errorf("%v", err)
		return err
	}

	// 如果远端路径和本地一致，则将链接相关的文件都包含进来
	if i.Basic().Mode()&os.ModeSymlink != 0 {
		originFile, err := os.Readlink(f)
		if err == nil {
			isNetwork, err := isNetworkLink(originFile)
			if !isNetwork {
				if !filepath.IsAbs(originFile) {
					originFile, err = filepath.Abs(filepath.Join(filepath.Dir(f), originFile))
					if err == nil {
						i.LinkTarget = originFile
						blog.Infof("toolchain: symlink %s to %s", f, originFile)
					} else {
						blog.Infof("toolchain: symlink %s origin %s, got abs path error:%s",
							f, originFile, err)
						return err
					}
				} else {
					i.LinkTarget = originFile
					blog.Infof("toolchain: symlink %s to %s", f, originFile)
				}

				err = checkAndAdd(i, remotepath, files)
				if err != nil {
					return err
				}

				// 递归查找
				return getRecursiveFiles(originFile, filepath.Dir(originFile), files)
			} else {
				blog.Infof("toolchain: symlink %s is network link, skip", originFile)
			}
		} else {
			blog.Infof("toolchain: symlink %s Readlink error:%s", f, err)
			return err
		}
	}

	return checkAndAdd(i, remotepath, files)
}

// 得到所有关联文件
func getAssociatedFiles(f string, remotepath string) (*[]FileDesc, error) {
	files := make([]FileDesc, 0, 0)
	err := getRecursiveFiles(f, remotepath, &files)

	return &files, err
}

// ToFileDesc parse toolchains to file targets
func (t *Toolchain) ToFileDesc() ([]FileDesc, error) {
	if t == nil {
		return nil, fmt.Errorf("tool chain is nil")
	}

	// TODO : 将链接展开，直到得到所有相关文件，比如 a->b,b->c，则需要将a/b/c都包含进来
	toolfiles := make([]FileDesc, 0, 0)
	for _, v := range t.Toolchains {
		files, err := getAssociatedFiles(v.ToolLocalFullPath, v.ToolRemoteRelativePath)
		if err != nil {
			return nil, err
		}
		// 倒序添加，保证创建链接成功
		size := len(*files)
		if size > 0 {
			for i := size - 1; i >= 0; i-- {
				toolfiles = append(toolfiles, (*files)[i])
			}
		}

		for _, f := range v.Files {
			files, err := getAssociatedFiles(f.LocalFullPath, f.RemoteRelativePath)
			if err != nil {
				return nil, err
			}
			// 倒序添加，保证创建链接成功
			size := len(*files)
			if size > 0 {
				for i := size - 1; i >= 0; i-- {
					toolfiles = append(toolfiles, (*files)[i])
				}
			}
		}
	}

	// 将文件集合中涉及到目录的链接列出来，这种需要提前发送
	getAllLinkDirs(&toolfiles)

	blog.Infof("toolchain: get all files:%+v", toolfiles)

	return toolfiles, nil
}

func getAllLinkDirs(files *[]FileDesc) ([]FileDesc, error) {
	lines := make([]string, 0, len(*files))
	for _, v := range *files {
		lines = append(lines, v.FilePath)
	}

	uniqlines := dcUtil.UniqArr(lines)
	linkdirs := dcUtil.GetAllLinkDir(uniqlines)
	if len(linkdirs) > 0 {
		for _, v := range linkdirs {
			getRecursiveFiles(v, filepath.Dir(v), files)
		}
	}

	return nil, nil
}

func GetAdditionFileKey() string {
	return "addition\\file|key"
}
