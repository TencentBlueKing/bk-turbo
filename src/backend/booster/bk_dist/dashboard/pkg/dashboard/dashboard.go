/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package dashboard

import (
	"net/http"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/dashboard/static"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/http/httpserver"
)

// RegisterStaticServer register the static server into router
func RegisterStaticServer(svr *httpserver.HTTPServer) error {
	statsFS, err := static.StatsFS()
	if err != nil {
		return err
	}

	svr.GetWebContainer().Handle("/", http.FileServer(http.FS(statsFS)))
	return nil
}
