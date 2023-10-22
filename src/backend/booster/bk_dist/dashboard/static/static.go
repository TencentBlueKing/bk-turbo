/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

// Package static embed static resource and index.html
package static

import (
	"embed"
	"io/fs"
)

//go:embed controller stats
var assets embed.FS

// StatsFS stats 静态资源
func StatsFS() (fs.FS, error) {
	stats, err := fs.Sub(assets, "stats")
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// ControllerFS controller 静态资源
func ControllerFS() (fs.FS, error) {
	controller, err := fs.Sub(assets, "controller")
	if err != nil {
		return nil, err
	}

	return controller, nil
}
