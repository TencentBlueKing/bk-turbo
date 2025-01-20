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

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/resultcache"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

func main() {
	c := config.NewConfig()
	if c == nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("failed to get config file"))
		os.Exit(1)
	}

	c.Parse()
	if !filepath.IsAbs(c.ResultCacheDir) {
		newabs, err := filepath.Abs(c.ResultCacheDir)
		if err == nil {
			c.ResultCacheDir = newabs
		}
	}
	config.GlobalResultCacheDir = c.ResultCacheDir
	config.GlobalMaxFileNumber = c.MaxResultFileNumber
	config.GlobalMaxIndexNumber = c.MaxResultIndexNumber
	if c.ResultCache {
		resultcache.GetInstance(config.GlobalResultCacheDir,
			config.GlobalMaxFileNumber,
			config.GlobalMaxIndexNumber)
	}

	if !filepath.IsAbs(c.LogConfig.LogDir) {
		newabs, err := filepath.Abs(c.LogConfig.LogDir)
		if err == nil {
			c.LogConfig.LogDir = newabs
		}
	}

	fmt.Printf("ready init log with dir:%s\n", c.LogConfig.LogDir)
	blog.InitLogs(c.LogConfig)
	defer blog.CloseLogs()

	if err := pkg.Run(c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
