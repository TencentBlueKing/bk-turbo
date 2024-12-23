/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package local

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/env"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/resultcache"
	dcSDK "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/sdk"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/controller/pkg/types"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

const (
	LongExecutionSeconds = 300
)

func (e *executor) initResultCacheInfo() {
	e.cacheType = e.handler.SupportResultCache(e.req.Commands)

	e.cacheGroupKey = ""
	if str := e.sandbox.Env.GetEnv(env.ProjectID); str != "" {
		e.cacheGroupKey = str
	}

	e.commandKey = strings.Join(e.req.Commands, " ")

	record := resultcache.Record{
		resultcache.CommandKey: e.commandKey,
	}
	if e.localCacheEnabled() {
		e.hasLocalIndex, _ = e.mgr.hasLocalIndex(record)
	}
	if e.remoteCacheEnabled() {
		e.hasRemoteIndex, _ = e.mgr.hasRemoteIndex(record)
	}

	blog.Infof("executor: got cache type:%d,cache group key:%s,command:[%s],"+
		"haslocalindex:%v,hasremoteindex:%v",
		e.cacheType, e.cacheGroupKey, e.commandKey,
		e.hasLocalIndex, e.hasRemoteIndex)
}

func (e *executor) localCacheEnabled() bool {
	return e.cacheType&resultcache.CacheTypeLocal == 1
}

func (e *executor) remoteCacheEnabled() bool {
	return e.cacheType&resultcache.CacheTypeRemote == 1
}

func (e *executor) cacheEnabled() bool {
	return e.localCacheEnabled() || e.remoteCacheEnabled()
}

func (e *executor) getCacheResult(c *dcSDK.BKDistCommand) *types.LocalTaskExecuteResult {
	if len(c.Commands) != 1 {
		return nil
	}

	if e.cacheEnabled() {
		e.preprocessResultKey = e.handler.GetResultCacheKey(e.req.Commands)
		if e.preprocessResultKey == "" {
			return nil
		}

		if e.localCacheEnabled() {
			result := e.getLocalResultFiles(c)
			if result != nil {
				return result
			}
		}

		// TODO : get result from remote
		if e.remoteCacheEnabled() {
			result := e.getRemoteResultFiles(c)
			if result != nil {
				return result
			}
		}
	}

	return nil
}

func (e *executor) getLocalResultFiles(c *dcSDK.BKDistCommand) *types.LocalTaskExecuteResult {
	rs, err := resultcache.GetInstance("").GetResult(e.preprocessResultKey, true)
	if err != nil {
		return nil
	}

	// TODO : 先加到远程统计数据里面，后续可能需要完善，比如单独加个统计
	defer e.mgr.work.Basic().UpdateJobStats(e.stats)

	dcSDK.StatsTimeNow(&e.stats.RemoteWorkEnterTime)
	defer dcSDK.StatsTimeNow(&e.stats.RemoteWorkLeaveTime)
	e.mgr.work.Basic().UpdateJobStats(e.stats)

	// 匹配和保存，简单的用文件名或者后缀来匹配
	resultmap := make(map[string]int)
	for _, r := range c.Commands[0].ResultFiles {
		found := false
		// 先用文件名匹配
		rbase := filepath.Base(r)
		for index, rc := range rs {
			if rbase == filepath.Base(rc.FileName) {
				if !filepath.IsAbs(r) {
					r = filepath.Join(c.Commands[0].WorkDir, r)
				}
				resultmap[r] = index
				found = true
			}
		}

		// 再用后缀匹配；如果同一后缀有多个结果文件，可能匹配错误！！
		if !found {
			rsuffix := filepath.Ext(r)
			for index, rc := range rs {
				if rsuffix == filepath.Ext(rc.FileName) {
					if !filepath.IsAbs(r) {
						r = filepath.Join(c.Commands[0].WorkDir, r)
					}
					resultmap[r] = index
					found = true
				}
			}
		}

		if !found {
			blog.Warnf("executor: not found cache for file %s", r)
			return nil
		}
	}

	for k, v := range resultmap {
		f, err := os.Create(k)
		if err != nil {
			blog.Errorf("executor: create file %s with error: %v", k, err)
			return nil
		}

		_, err = f.Write(rs[v].CompressDataBuf)
		if err != nil {
			f.Close()
			blog.Errorf("executor: save file %s with error: %v", k, err)
			return nil
		}
		f.Close()
		blog.Infof("executor: got cache result file %s with key:%s", k, e.preprocessResultKey)
	}

	e.stats.PostWorkSuccess = true
	e.stats.RemoteWorkSuccess = true
	e.stats.Success = true
	e.mgr.work.Basic().UpdateJobStats(e.stats)

	return &types.LocalTaskExecuteResult{
		Result: &dcSDK.LocalTaskResult{
			ExitCode: 0,
			Stdout:   nil,
			Stderr:   nil,
		},
	}
}

func (e *executor) getRemoteResultFiles(c *dcSDK.BKDistCommand) *types.LocalTaskExecuteResult {
	rs, err := e.mgr.getRemoteResultCacheFile(e.preprocessResultKey)
	if err != nil {
		return nil
	}

	// TODO : 先加到远程统计数据里面，后续可能需要完善，比如单独加个统计
	defer e.mgr.work.Basic().UpdateJobStats(e.stats)

	dcSDK.StatsTimeNow(&e.stats.RemoteWorkEnterTime)
	defer dcSDK.StatsTimeNow(&e.stats.RemoteWorkLeaveTime)
	e.mgr.work.Basic().UpdateJobStats(e.stats)

	// 匹配和保存，简单的用文件名或者后缀来匹配
	resultmap := make(map[string]int)
	for _, r := range c.Commands[0].ResultFiles {
		found := false
		// 先用文件名匹配
		rbase := filepath.Base(r)
		for index, rc := range rs.Resultfiles {
			if rbase == filepath.Base(rc.FilePath) {
				if !filepath.IsAbs(r) {
					r = filepath.Join(c.Commands[0].WorkDir, r)
				}
				resultmap[r] = index
				found = true
			}
		}

		// 再用后缀匹配；如果同一后缀有多个结果文件，可能匹配错误！！
		if !found {
			rsuffix := filepath.Ext(r)
			for index, rc := range rs.Resultfiles {
				if rsuffix == filepath.Ext(rc.FilePath) {
					if !filepath.IsAbs(r) {
						r = filepath.Join(c.Commands[0].WorkDir, r)
					}
					resultmap[r] = index
					found = true
				}
			}
		}

		if !found {
			blog.Warnf("executor: not found cache for file %s", r)
			return nil
		}
	}

	for k, v := range resultmap {
		f, err := os.Create(k)
		if err != nil {
			blog.Errorf("executor: create file %s with error: %v", k, err)
			return nil
		}

		_, err = f.Write(rs.Resultfiles[v].Buffer)
		if err != nil {
			f.Close()
			blog.Errorf("executor: save file %s with error: %v", k, err)
			return nil
		}
		f.Close()
		blog.Infof("executor: got cache result file %s with key:%s", k, e.preprocessResultKey)
	}

	e.stats.PostWorkSuccess = true
	e.stats.RemoteWorkSuccess = true
	e.stats.Success = true
	e.mgr.work.Basic().UpdateJobStats(e.stats)

	return &types.LocalTaskExecuteResult{
		Result: &dcSDK.LocalTaskResult{
			ExitCode: 0,
			Stdout:   nil,
			Stderr:   nil,
		},
	}
}

// 1. 如果在index里面，则一定需要上报到cache
// 2. 如果不在index里面，则判断是否符合上报条件（比如远程执行时间）
// 3. 如果result key为空，则无需上报结果文件；否则，需要上报index和结果文件
func (e *executor) putCacheResult(r *dcSDK.BKDistResult, stat *dcSDK.ControllerJobStats) error {
	if e.cacheEnabled() {
		if e.preprocessResultKey == "" {
			e.preprocessResultKey = e.handler.GetResultCacheKey(e.req.Commands)
		}

		remoteTooLong := false
		if stat != nil {
			dura := stat.RemoteWorkProcessEndTime.Unix() - stat.RemoteWorkProcessStartTime.Unix()
			e.remoteExecuteSecs = int(dura)
			blog.Infof("executor: remote executed %d seconds for this command", e.remoteExecuteSecs)
		}
		if e.remoteExecuteSecs > 0 {
			remoteTooLong = e.remoteExecuteSecs >= LongExecutionSeconds
		}

		record := resultcache.Record{}

		// local cache
		if e.localCacheEnabled() && (e.hasLocalIndex || remoteTooLong) {
			// report result files
			if e.preprocessResultKey != "" {
				err := e.putLocalResultFiles(r)
				if err != nil {
					blog.Infof("executor: put result file to local with error:%v", err)
				}
			}

			// report result index
			record[resultcache.GroupKey] = e.cacheGroupKey
			record[resultcache.CommandKey] = e.commandKey
			record[resultcache.RemoteExecuteTimeKey] = strconv.Itoa(e.remoteExecuteSecs)
			if e.preprocessResultKey != "" {
				record[resultcache.ResultKey] = e.preprocessResultKey
			}
			err := resultcache.GetInstance("").PutRecord(record)
			if err != nil {
				blog.Infof("executor: put result index to local with error:%v", err)
			}
		}

		// remote cache
		if e.remoteCacheEnabled() && (e.hasRemoteIndex || remoteTooLong) {
			if len(record) == 0 {
				record[resultcache.GroupKey] = e.cacheGroupKey
				record[resultcache.CommandKey] = e.commandKey
				record[resultcache.RemoteExecuteTimeKey] = strconv.Itoa(e.remoteExecuteSecs)
				if e.preprocessResultKey != "" {
					record[resultcache.ResultKey] = e.preprocessResultKey
				}
			}

			err := e.putRemoteResult(r, record)
			if err != nil {
				blog.Infof("executor: put result file to remote with error:%v", err)
			}
		}
	}

	return nil
}

func (e *executor) putLocalResultFiles(r *dcSDK.BKDistResult) error {
	if len(r.Results) != 1 {
		return nil
	}

	// 保存结果数据到cache
	resultlen := len(r.Results[0].ResultFiles)
	rs := make([]*resultcache.Result, 0, resultlen)
	for _, v := range r.Results[0].ResultFiles {
		rs = append(rs, &resultcache.Result{
			FileName:        filepath.Base(v.FilePath),
			CompressDataBuf: v.Buffer,
			CompressType:    v.Compresstype,
			RealSize:        uint64(v.FileSize),
			HashStr:         "",
		})
	}

	err := resultcache.GetInstance("").PutResult(e.preprocessResultKey, rs)
	return err
}

func (e *executor) putRemoteResult(r *dcSDK.BKDistResult, record resultcache.Record) error {
	if len(r.Results) != 1 {
		return nil
	}

	var err error
	if e.preprocessResultKey != "" {
		// 保存结果数据到cache
		resultlen := len(r.Results[0].ResultFiles)
		rs := make([]*dcSDK.FileDesc, 0, resultlen)
		for _, v := range r.Results[0].ResultFiles {
			rs = append(rs, &v)
		}

		_, err = e.mgr.reportRemoteResultCache(record, rs)
	} else {
		_, err = e.mgr.reportRemoteResultCache(record, nil)
	}

	return err
}
