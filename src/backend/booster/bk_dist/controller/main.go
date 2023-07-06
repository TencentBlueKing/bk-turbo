/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

func main() {
	c := config.NewConfig()
	c.Parse()
	blog.InitLogs(c.LogConfig)

	panicfile := filepath.Join(c.LogConfig.LogDir, "bk-dist-controller-panic.log")
	syscall.RedirectStderror(panicfile)

	if err := pkg.Run(c); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		blog.Errorf("start controller failed: %v", err)
		blog.CloseLogs()
		os.Exit(1)
	}
}
