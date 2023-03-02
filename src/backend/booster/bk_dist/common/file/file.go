/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package file

import (
	"crypto/md5"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tencent/bk-ci/src/booster/common/blog"
)

// Stat do the os.Stat and return an Info
func Stat(fp string) *Info {
	info, err := os.Stat(fp)
	return &Info{
		filePath: fp,
		info:     info,
		err:      err,
	}
}

// Lstat do the os.Lstat and return an Info
func Lstat(fp string) *Info {
	info, err := os.Lstat(fp)
	return &Info{
		filePath: fp,
		info:     info,
		err:      err,
	}
}

// get file info by enum dir
// TODO : not ok for linux, do not call in linux
func GetFileInfoByEnumDir(fp string) *Info {
	fis, err := ioutil.ReadDir(filepath.Dir(fp))
	if err != nil {
		return &Info{
			filePath: fp,
			info:     nil,
			err:      err,
		}
	} else {
		for _, fi := range fis {
			if strings.EqualFold(fi.Name(), filepath.Base(fp)) {
				newfp := fp
				if !strings.HasSuffix(fp, fi.Name()) {
					old := filepath.Base(fp)
					new := fi.Name()
					i := strings.LastIndex(fp, old)
					if i >= 0 {
						newfp = fp[:i] + new + fp[i+len(old):]
						blog.Infof("common file: from [%s] to [%s]", fp, newfp)
					}
				}

				return &Info{
					filePath: newfp,
					info:     fi,
					err:      err,
				}
			}
		}

		return &Info{
			filePath: fp,
			info:     nil,
			err:      fmt.Errorf("%s not existed", fp),
		}
	}
}

// Info describe the os.FileInfo and handle some actions
type Info struct {
	filePath   string
	LinkTarget string

	// info and err are return from os.Stat
	info os.FileInfo
	err  error
}

// Path return the file path
func (i *Info) Path() string {
	return i.filePath
}

// Basic return the origin os.FileInfo
func (i *Info) Basic() os.FileInfo {
	return i.info
}

// Error return the os.Stat/os.Lstat error
func (i *Info) Error() error {
	return i.err
}

// Exist check if the file is exist
func (i *Info) Exist() bool {
	return i.info != nil && (i.err == nil || os.IsExist(i.err))
}

// ModifyTime return the nano-second of this file's mod time
func (i *Info) ModifyTime() int64 {
	if i.info == nil {
		return 0
	}
	return i.info.ModTime().UnixNano()
}

// ModifyTime64 return the ModifyTime as int64
func (i *Info) ModifyTime64() int64 {
	return int64(i.ModifyTime())
}

// Mode32 return the file mode
func (i *Info) Mode32() uint32 {
	if i.info == nil {
		return 0
	}
	return uint32(i.info.Mode())
}

// Size return the file size
func (i *Info) Size() int64 {
	if i.info == nil {
		return 0
	}
	return i.info.Size()
}

// Batch return Exist, Size, ModifyTime64, Mode32 in one call.
func (i *Info) Batch() (bool, int64, int64, uint32) {
	return i.Exist(), i.Size(), i.ModifyTime64(), i.Mode32()
}

// Md5 return the md5 of this file
func (i *Info) Md5() (string, error) {
	if i.err != nil {
		return "", i.err
	}

	f, err := os.Open(i.filePath)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = f.Close()
	}()

	md5hash := md5.New()
	if _, err := io.Copy(md5hash, f); err != nil {
		return "", err
	}

	md5string := fmt.Sprintf("%x", md5hash.Sum(nil))
	return md5string, nil
}
