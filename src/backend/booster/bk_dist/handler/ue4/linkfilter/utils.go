/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package linkfilter

import (
	"strings"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"

	"github.com/google/shlex"
)

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

// E:\tbs\Engine\Build\Windows\link-filter\link-filter.exe -- C:\Program Files (x86)\Microsoft Visual Studio\2019\Community\VC\Tools\MSVC\14.28.29333\bin\HostX64\x64\link.exe @e:\tbs\Engine\Intermediate\Build\Win64\ShaderCompileWorker\Development\Core\ShaderCompileWorker-Core.dll.response
// ensure compiler exist in args.
func ensureCompiler(args []string) ([]string, error) {
	if len(args) < 2 {
		return nil, ErrorInvalidParam
	}

	if len(args) > 2 && args[1] == "--" {
		return args[2:], nil
	}

	options, err := shlex.Split(replaceWithNextExclude(args[1], '\\', "\\\\", []byte{'"'}))
	if err != nil {
		return nil, err
	}
	blog.Infof("lf: got args:%v", options)

	for i := range options {
		if options[i] == "--" {
			if i == len(options)-1 {
				return nil, ErrorInvalidParam
			}

			return options[i+1:], nil
		}
	}

	return nil, ErrorInvalidParam
}
