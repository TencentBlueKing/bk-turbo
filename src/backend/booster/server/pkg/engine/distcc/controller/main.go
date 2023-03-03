package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/engine/distcc/controller/config"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/server/pkg/engine/distcc/controller/pkg"
)

func main() {
	c := config.NewConfig()
	c.Parse()
	blog.InitLogs(c.LogConfig)
	log.SetOutput(ioutil.Discard)
	defer blog.CloseLogs()

	if err := pkg.Run(c); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
