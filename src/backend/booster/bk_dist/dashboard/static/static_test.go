/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package static

import (
	"io"
	"testing"
)

func BenchmarkStatsFS(b *testing.B) {
	staticFS, err := StatsFS()
	if err != nil {
		panic(err)
	}

	for i := 0; i < b.N; i++ {
		f, err := staticFS.Open("index.html")
		if err != nil {
			panic(err)
		}

		_, err = io.Copy(io.Discard, f)
		if err != nil {
			panic(err)
		}
	}
}
