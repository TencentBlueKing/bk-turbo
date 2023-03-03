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

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/gateway/pkg"
)

func main() {
	c := config.NewConfig()
	c.Parse()
	blog.InitLogs(c.LogConfig)
	defer blog.CloseLogs()

	if err := pkg.Run(c); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
