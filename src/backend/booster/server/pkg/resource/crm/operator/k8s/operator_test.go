/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package k8s

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/net"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/config"
)

func TestGenerateNativeClient(t *testing.T) {
	wd, _ := os.Getwd()
	tplPath, _ := filepath.Abs(filepath.Join(wd, "../../../../../template/k8s_container.yaml.template"))

	conf := &config.ContainerResourceConfig{
		BcsAppTemplate: tplPath,
		KubeConfigPath: "/root/.kube/config",
	}

	conf.BcsAPIPool = net.NewConnectPool(strings.Split(conf.BcsAPIAddress, ","))
	conf.BcsAPIPool.Start()

	operator, err := NewOperator(conf)
	if err != nil {
		t.Fatal(err)
	}

	node, err := operator.GetResource("Fake-Cluster-ID")
	if err != nil {
		t.Fatal(err)
	}

	if len(node) == 0 {
		t.Fatalf("have no node")
	}

	t.Log(node[0])
}
