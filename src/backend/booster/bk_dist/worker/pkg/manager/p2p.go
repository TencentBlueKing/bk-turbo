/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package manager

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/codec"
	commonHttp "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/http"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/http/httpclient"
	commonTypes "github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/version"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	dcSyscall "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/syscall"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/worker/pkg/types"
)

// define const strings
const (
	LabelKeyGOOS = "os"

	LabelValueGOOSWindows = "windows"
	LabelValueGOOSDarwin  = "darwin"

	reportIntervalTime = 5 * time.Second

	restartCheckIntervalTime = 300 * time.Second
	upgradeCheckIntervalTime = 300 * time.Second

	defaultRestartThresholdSecs = 21600
)

var (
	agent *types.AgentInfo = &types.AgentInfo{}

	reportURL       = ""
	reportURLFormat = "http://%s/api/v1/build/reportresource"

	currentP2PUserNumber int = 0

	processStartTime = ""

	overRestart = false

	upgradeLock      sync.Mutex
	upgradeURL       = ""
	upgradeURLFormat = "http://%s/api/v2/upgrade/worker"
)

func (o *tcpManager) checkP2PConf() error {
	blog.Infof("[p2p] check p2p conf with:%+v", *o.conf)

	if o.conf.P2P {
		processStartTime = time.Now().String()

		if o.conf.P2PServer == "" {
			return fmt.Errorf("p2p server can't be empty when p2p enabled")
		}
		reportURL = fmt.Sprintf(reportURLFormat, o.conf.P2PServer)
		blog.Infof("[p2p] got report url %s", reportURL)

		if o.conf.P2PGroupLabel == "" {
			return fmt.Errorf("p2p group label can't be empty when p2p enabled")
		}

		// add p2p tag in group label
		if !strings.Contains(o.conf.P2PGroupLabel, commonTypes.LabelValueModeP2P) {
			o.conf.P2PGroupLabel = commonTypes.LabelValueModeP2P + "_" + o.conf.P2PGroupLabel
		}

		agent.Total.CPU = totalSlotCPU
		agent.Total.Mem = totalSlotMemory

		if o.conf.RestartThresholdSecs <= 0 {
			o.conf.RestartThresholdSecs = defaultRestartThresholdSecs
		}
	}

	return nil
}

func (o *tcpManager) startP2P() error {
	if o.conf.P2P {
		err := o.checkP2PConf()
		if err != nil {
			blog.Warnf("[p2p] check p2p conf failed with error:%v", err)
			return err
		}

		err = o.initP2PBaseInfo()
		if err != nil {
			blog.Warnf("[p2p] init p2p base info failed with error:%v", err)
			return err
		}

		err = o.initHttpClient()
		if err != nil {
			blog.Warnf("[p2p] init http client failed with error:%v", err)
			return err
		}

		go o.reportTimer()
	}

	return nil
}

func (o *tcpManager) reportTimer() {
	blog.Infof("[p2p] start p2p report timer")
	resourceReportTick := time.NewTicker(reportIntervalTime)
	defer resourceReportTick.Stop()

	for {
		select {
		case <-resourceReportTick.C:
			o.reportResource()
		}
	}
}

func (o *tcpManager) getLocalIP() (string, error) {
	blog.Infof("[p2p] initLocalIp...")
	ips := dcUtil.GetIPAddress()
	if len(ips) <= 0 {
		err := fmt.Errorf("get none local ip")
		blog.Infof("[p2p] get none local ip")
		return "", err
	}
	ip := ips[0]

	blog.Infof("[p2p] got localip[%s]", ip)
	o.conf.LocalConfig.LocalIP = ip
	return ip, nil
}

func (o *tcpManager) initP2PBaseInfo() error {
	specifiedIP := false
	var ip string
	var err error
	if o.conf.LocalConfig.LocalIP != "" {
		blog.Infof("[p2p] user specified localip:[%s]", o.conf.LocalConfig.LocalIP)
		specifiedIP = true
		ip = o.conf.LocalConfig.LocalIP
	} else {
		ip, err = o.getLocalIP()
		if err != nil {
			return err
		}
	}

	username := "unknown"
	usr, err := user.Current()
	if err == nil {
		username = usr.Username
	}

	agent.Base = types.AgentBase{
		IP:      ip,
		Port:    int(o.conf.Port),
		Message: "",
		Cluster: o.conf.P2PGroupLabel,
		Labels: map[string]string{
			LabelKeyGOOS:                            runtime.GOOS,
			commonTypes.LabelKeyMode:                commonTypes.LabelValueModeP2P,
			commonTypes.LabelKeyP2PUserNumber:       strconv.Itoa(currentP2PUserNumber),
			commonTypes.LabelKeyP2PProcessStartTime: processStartTime,
			commonTypes.LabelKeySupportAbsPath:      strconv.FormatBool(o.conf.SupportAbsPath),
			commonTypes.LabelKeyP2PSpecifiedIP:      strconv.FormatBool(specifiedIP),
			commonTypes.LabelKeyVersion:             version.Version,
			commonTypes.LabelKeyUser:                username,
		},
	}

	return nil
}

func (o *tcpManager) initHttpClient() error {
	cli := httpclient.NewHTTPClient()
	cli.SetHeader("Content-Type", "application/json")
	cli.SetHeader("Accept", "application/json")

	o.client = cli

	return nil
}

func (o *tcpManager) reportResource() error {
	blog.Infof("[p2p] ReportResource for group[%s] ip[%s]...", o.conf.P2PGroupLabel, o.conf.LocalConfig.LocalIP)

	// o.updateAvailable()
	agent.Free.CPU = currentAvailableSlotCPU
	agent.Free.Mem = currentAvailableSlotMemory

	var reportobj = types.ReportAgentResource{
		AgentInfo: *agent,
	}

	var data []byte
	_ = codec.EncJSON(reportobj, &data)

	if _, _, err := o.post(reportURL, nil, data); err != nil {
		blog.Errorf("[p2p] report resource: report failed: %v", err)
		return err
	}

	blog.Infof("[p2p] report resource with url: %s data: [%s]", reportURL, (string)(data))
	return nil
}

func (o *tcpManager) query(uri string, header http.Header) (resp *commonHttp.APIResponse, raw []byte, err error) {
	return o.request("GET", uri, header, nil)
}

func (o *tcpManager) post(uri string, header http.Header, data []byte) (
	resp *commonHttp.APIResponse, raw []byte, err error) {
	return o.request("POST", uri, header, data)
}

func (o *tcpManager) delete(uri string, header http.Header, data []byte) (
	resp *commonHttp.APIResponse, raw []byte, err error) {
	return o.request("DELETE", uri, header, data)
}

func (o *tcpManager) request(method, uri string, header http.Header, data []byte) (
	resp *commonHttp.APIResponse, raw []byte, err error) {
	var r *httpclient.HttpResponse

	switch strings.ToUpper(method) {
	case "GET":
		if r, err = o.client.Get(uri, header, data); err != nil {
			return
		}
	case "POST":
		if r, err = o.client.Post(uri, header, data); err != nil {
			return
		}
	case "PUT":
		if r, err = o.client.Put(uri, header, data); err != nil {
			return
		}
	case "DELETE":
		if r, err = o.client.Delete(uri, header, data); err != nil {
			return
		}
	}

	if r.StatusCode != http.StatusOK {
		err = fmt.Errorf("failed to request, http(%d)%s: %s", r.StatusCode, r.Status, uri)
		return
	}

	if err = codec.DecJSON(r.Reply, &resp); err != nil {
		err = fmt.Errorf("%v: %s", err, string(r.Reply))
		return
	}

	if resp.Code != common.RestSuccess {
		err = fmt.Errorf("failed to request, resp(%d)%s: %s", resp.Code, resp.Message, uri)
		return
	}

	if err = codec.EncJSON(resp.Data, &raw); err != nil {
		return
	}
	return
}

// support restart automatic
func (o *tcpManager) restartCheckTimer() {
	blog.Infof("[restart] start restart check timer")
	tick := time.NewTicker(restartCheckIntervalTime)
	defer tick.Stop()
	startTime := time.Now()

	for {
		select {
		case <-tick.C:
			o.restartCheck(startTime)
		}
	}
}

func (o *tcpManager) restartCheck(t time.Time) {
	blog.Infof("[restart] do restart check now")

	// check running over time
	if !overRestart {
		nowsecs := time.Now().Unix()
		startsecs := t.Unix()
		if nowsecs-startsecs > int64(o.conf.RestartThresholdSecs) {
			overRestart = true
		} else {
			blog.Infof("[restart] only run %d seconds less than %d,do nothing now",
				nowsecs-startsecs, o.conf.RestartThresholdSecs)
			return
		}
	}

	// check if idle now
	if o.curjobs <= 0 && len(o.buffedcmds) == 0 {
		// 避免升级时重启，导致可执行程序异常
		upgradeLock.Lock()
		defer upgradeLock.Unlock()

		blog.Infof("[restart] ready restart worker now")

		exePath, err := os.Executable()
		if err != nil {
			blog.Warnf("[restart] get execute path failed with error:%v", err)
			return
		}

		exeDir := filepath.Dir(exePath)
		// TODO : support for linux and mac later
		restartScript := filepath.Join(exeDir, "restart.bat")

		blog.Infof("[restart] ready restart with run [%s] at dir:%s", restartScript, exeDir)
		sandbox := dcSyscall.Sandbox{}
		sandbox.Dir = exeDir
		sandbox.ExecScripts(restartScript)
	}
}

// 将下载的文件放到目标位置，如果有则覆盖（考虑先备份）
type upgradeOneRule struct {
	URL        string `json:"url"`
	MD5        string `json:"md5"`
	TargetPath string `json:"target_path"`
}

type upgradeOneVersion struct {
	Version string           `json:"version"`
	Rules   []upgradeOneRule `json:"rules"`
}

type upgradeConf struct {
	Versions []upgradeOneVersion `json:"versions"`
}

// support upgrade automatic
func (o *tcpManager) upgradeCheckTimer() {
	blog.Infof("[upgrade] start upgrade check timer")
	tick := time.NewTicker(upgradeCheckIntervalTime)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			o.upgradeCheck()
		}
	}
}

func (o *tcpManager) queryUpgradeJson() (*upgradeConf, error) {
	blog.Infof("[upgrade] do query upgrade json with version:%s", version.Version)

	if o.conf.UpgradeServer == "" {
		blog.Warnf("[upgrade] do upgrade check failed for UpgradeServer empty")
		return nil, fmt.Errorf("upgrade server is empty")
	}

	if upgradeURL == "" {
		upgradeURL = fmt.Sprintf(upgradeURLFormat, o.conf.UpgradeServer)
		blog.Infof("[upgrade] got upgrade url %s", upgradeURL)
	}

	var err error
	var rawbase64 []byte
	if _, rawbase64, err = o.query(upgradeURL, nil); err != nil {
		blog.Warnf("[upgrade] query upgrade failed: %v", err)
		return nil, err
	}
	blog.Infof("[upgrade] url:%s,raw:%s", upgradeURL, string(rawbase64))

	rawbase64 = bytes.Trim(rawbase64, "\"")
	rawbytes, err := base64.StdEncoding.DecodeString(string(rawbase64))
	if err != nil {
		blog.Warnf("[upgrade] decode %s failed with error:%v", string(rawbase64), err)
		return nil, err
	}

	var conf upgradeConf
	if err = codec.DecJSON(rawbytes, &conf); err != nil {
		blog.Warnf("[upgrade] decode %s failed with error:%v", string(rawbytes), err)
	}

	blog.Infof("[upgrade] got upgrade json:%+v", conf)
	return &conf, nil
}

// TODO : 后续支持配置文件的更新（需要将用户修改的配置项保存下来）
func (o *tcpManager) upgradeCheck() error {
	blog.Infof("[upgrade] do upgrade check with version:%s", version.Version)
	upgradeLock.Lock()
	defer upgradeLock.Unlock()

	conf, err := o.queryUpgradeJson()
	if err != nil {
		return err
	}

	// 下载，校验和更新
	for _, v := range conf.Versions {
		if v.Version == version.Version {
			filemap := make(map[string]string)
			exePath, err1 := os.Executable()
			exeDir := ""
			if err1 != nil {
				blog.Warnf("[upgrade] get execute path failed with error:%v", err1)
			} else {
				exeDir = filepath.Dir(exePath)
			}

			tmpdir := filepath.Join(exeDir, "tmp")
			os.MkdirAll(tmpdir, os.ModePerm)

			// 下载所有目标文件到临时目录
			for _, r := range v.Rules {
				tempf := filepath.Join(tmpdir, filepath.Base(r.URL))
				targetf := r.TargetPath
				if !filepath.IsAbs(r.TargetPath) {
					if exeDir == "" {
						blog.Warnf("[upgrade] exe path is empty and target is relative,do nothing")
						return nil
					} else {
						targetf = filepath.Join(exeDir, r.TargetPath)
					}
				}

				// 如果target存在且md5一样，则忽略
				md5, err := file.Stat(targetf).Md5()
				if err == nil && md5 == r.MD5 {
					blog.Infof("[upgrade] target [%s] existed and md5 same,ignore this file", targetf)
					continue
				}

				filemap[tempf] = targetf

				// 如果临时文件已经存在（可能之前下载的），则跳过下载
				md5, err = file.Stat(tempf).Md5()
				if err == nil && md5 == r.MD5 {
					blog.Infof("[upgrade] temp file[%s] existed and md5 same, do not need download", tempf)
					continue
				}

				err = downloadFile(tempf, r.URL)
				if err != nil {
					blog.Warnf("[upgrade] download %s failed with error:%v", r.URL, err)
					return err
				}
				blog.Infof("[upgrade] downloaded file:%s,target is:%s", tempf, targetf)

				os.Chmod(tempf, os.ModePerm)
				md5, err = file.Stat(tempf).Md5()
				if err != nil {
					blog.Warnf("[upgrade] md5 file %s failed with error:%v", tempf, err)
					return err
				}
				if md5 != r.MD5 {
					blog.Warnf("[upgrade] local md5 %s not equal remote md5 %s", md5, r.MD5)
					return fmt.Errorf("local md5 %s not equal remote md5 %s", md5, r.MD5)
				}
			}

			// rename所有下载文件
			bakSuffix := fmt.Sprintf("%d.bak", time.Now().Unix())
			for k, v := range filemap {
				if file.Stat(v).Exist() {
					bakfile := fmt.Sprintf("%s_%s", v, bakSuffix)
					// TODO : 现在如果这个地方失败了，则没办法回退
					err = os.Rename(v, bakfile)
					if err != nil {
						blog.Errorf("[upgrade] rename %s to %s failed with error:%v", v, bakfile, err)
					} else {
						blog.Errorf("[upgrade] rename %s to %s succeed", v, bakfile)
					}
				}
				// TODO : 现在如果这个地方失败了，则没办法回退
				err = os.Rename(k, v)
				if err != nil {
					blog.Errorf("[upgrade] rename %s to %s failed with error:%v", k, v, err)
				} else {
					blog.Errorf("[upgrade] rename %s to %s succeed", k, v)
				}
			}

			break
		}
	}

	return nil

}

func downloadFile(f string, urlstr string) error {
	if f == "" {
		dir, _ := os.Getwd()
		f = filepath.Join(dir, filepath.Base(urlstr))
		blog.Infof("save target file:%s\n", f)
	}

	// Create the file
	out, err := os.Create(f)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	// Get the data
	resp, err := http.Get(urlstr)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
