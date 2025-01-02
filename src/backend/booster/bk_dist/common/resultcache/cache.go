/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package resultcache

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	dcFile "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/file"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/cespare/xxhash/v2"
)

type ResultCacheMgr interface {
	GetResult(groupkey, resultkey string, needUnzip bool) ([]*Result, error)
	PutResult(groupkey, resultkey string, rs []*Result) error

	GetRecordGroup(key string) ([]byte, error)
	PutRecord(record Record) error
	DeleteRecord(record Record) error
}

const (
	CacheTypeNone = iota
	CacheTypeLocal
	CacheTypeRemote
	CacheTypeBothLocalAndRemote = CacheTypeLocal & CacheTypeRemote
)

type Result struct {
	FileName        string
	CompressDataBuf []byte
	CompressType    protocol.CompressType
	RealSize        uint64
	HashStr         string
}

func (r *Result) generateFileName() string {
	if r.HashStr == "" {
		hashbyte := xxhash.Sum64(r.CompressDataBuf)
		r.HashStr = fmt.Sprintf("%x", hashbyte)
	}
	return fmt.Sprintf("%s_%d_%s_%s", r.HashStr, r.RealSize, r.CompressType.String(), r.FileName)
}

func getFileNameWithoutExtension(fullPath string) string {
	fileNameWithExt := filepath.Base(fullPath)
	fileName := strings.TrimSuffix(fileNameWithExt, filepath.Ext(fileNameWithExt))
	return fileName
}

func ToResult(f string, needUnzip bool) *Result {
	blog.Infof("resultcache: ready load file %s to result", f)

	r := &Result{}
	r.FileName = filepath.Base(f)

	fname := getFileNameWithoutExtension(f)
	fields := strings.Split(fname, "_")
	if len(fields) >= 3 {
		hashstrfromname, realsizestr, compresstypestr := fields[0], fields[1], fields[2]

		realsize, err := strconv.ParseUint(realsizestr, 10, 64)
		if err != nil {
			blog.Warnf("resultcache: file %s name resolve with error:%v", f, err)
			return nil
		}
		r.RealSize = realsize

		compressType := protocol.CompressNone
		if compresstypestr != "" {
			if compresstypestr == "lz4" {
				compressType = protocol.CompressLZ4
			}

			file, err := os.Open(f)
			if err != nil {
				blog.Warnf("resultcache: failed to open file:%s", f)
				return nil
			}
			defer file.Close()

			data, err := io.ReadAll(file)
			if err != nil {
				blog.Warnf("resultcache: file %s read with error:%v", f, err)
				return nil
			}

			hashbyte := xxhash.Sum64(data)
			nowhashstr := fmt.Sprintf("%x", hashbyte)

			if hashstrfromname != nowhashstr {
				blog.Warnf("resultcache: file %s hash %s not equal newly hash %s", f, hashstrfromname, nowhashstr)
				return nil
			}

			// 返回前先解压
			if needUnzip && compressType == protocol.CompressLZ4 {
				dst := make([]byte, realsize)
				outdata, err := dcUtil.Lz4Uncompress(data, dst)
				if err != nil {
					blog.Warnf("resultcache: decompress [%s] with error:%v", f, err)
					return nil
				}
				outlen := len(outdata)
				if uint64(outlen) != realsize {
					blog.Warnf("resultcache: file %s decompress size %d not equal realsize %d", f, outlen, realsize)
					return nil
				}

				data = outdata
				compressType = protocol.CompressNone
			}

			r.CompressDataBuf = data
			r.CompressType = compressType

			return r
		}
	}

	blog.Warnf("resultcache: file name %s is invalid", f)
	return nil
}

func (r *Result) save(dir string) error {
	fp := filepath.Join(dir, r.generateFileName())
	f, err := os.Create(fp)
	if err != nil {
		blog.Errorf("resultcache: create file %s error: %v", fp, err)
		return err
	}
	defer f.Close()

	_, err = f.Write(r.CompressDataBuf)
	if err != nil {
		blog.Errorf("resultcache: save file [%s] error: %v", fp, err)
		return err
	}

	blog.Infof("resultcache: succeed save file %s", fp)
	return nil
}

var (
	instance *ResultCache
	once     sync.Once
)

type ResultCache struct {
	FileDir string

	table *Table
}

func GetInstance(localdir string) ResultCacheMgr {
	once.Do(func() {
		if localdir == "" {
			localdir = dcUtil.GetResultCacheDir()
		}

		if !file.Stat(localdir).Exist() {
			os.MkdirAll(localdir, os.ModePerm)
		}

		filedir := filepath.Join(localdir, "file")
		if !file.Stat(filedir).Exist() {
			os.MkdirAll(filedir, os.ModePerm)
		}

		indexdir := filepath.Join(localdir, "index")
		if !file.Stat(indexdir).Exist() {
			os.MkdirAll(indexdir, os.ModePerm)
		}

		instance = &ResultCache{
			FileDir: filedir,
			table:   NewTable(indexdir),
		}

		instance.Init()
	})
	return instance
}

func (rc *ResultCache) GetResult(groupkey, resultkey string, needUnzip bool) ([]*Result, error) {
	return rc.getLocal(groupkey, resultkey, needUnzip)
}

func (rc *ResultCache) PutResult(groupkey, resultkey string, rs []*Result) error {
	if len(rs) == 0 {
		return nil
	}

	return rc.putLocal(groupkey, resultkey, rs)
}

func (rc *ResultCache) GetRecordGroup(key string) ([]byte, error) {
	return rc.table.GetRecordGroup(key)
}

func (rc *ResultCache) PutRecord(record Record) error {
	return rc.table.PutRecord(record)
}

func (rc *ResultCache) DeleteRecord(record Record) error {
	return rc.table.DeleteRecord(record)
}

func (rc *ResultCache) getDir(groupkey, resultkey string) (string, error) {
	if len(resultkey) < 3 || groupkey == "" {
		err := fmt.Errorf("result key %s too short or group key[%s] invalid",
			resultkey, groupkey)
		blog.Warnf("resultcache: get dir with error:%v", err)
		return "", err
	}

	dir := filepath.Join(rc.FileDir, groupkey, string(resultkey[0]), string(resultkey[1]), resultkey)
	return dir, nil
}

func (rc *ResultCache) getLocal(groupkey, resultkey string, needUnzip bool) ([]*Result, error) {
	dir, err := rc.getDir(groupkey, resultkey)
	if err != nil {
		blog.Warnf("resultcache: get dir by groupkey %s resultkey %s with error:%v",
			groupkey, resultkey, err)
		return nil, err
	}

	f, err := os.Open(dir)
	if err != nil {
		blog.Warnf("resultcache: failed to open dir %s with error:%v", dir, err)
		return nil, err
	}
	fis, _ := f.Readdir(-1)
	f.Close()

	rs := make([]*Result, 0, len(fis))
	for _, fi := range fis {
		f := filepath.Join(dir, fi.Name())
		r := ToResult(f, needUnzip)
		if r != nil {
			rs = append(rs, r)
		}
	}

	return rs, nil
}

func (rc *ResultCache) putLocal(groupkey, resultkey string, rs []*Result) error {
	dir, err := rc.getDir(groupkey, resultkey)
	if err != nil {
		blog.Warnf("resultcache: get dir by group key %s result key: %s with error:%v",
			groupkey, resultkey, err)
		return err
	}

	os.RemoveAll(dir)
	if !dcFile.Stat(dir).Exist() {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			blog.Warnf("resultcache: create dir %s with error:%v", dir, err)
			return err
		}
	}

	for _, r := range rs {
		err = r.save(dir)
		if err != nil {
			return err
		}
	}

	return nil
}

func (rc *ResultCache) Init() {
	rc.table.Init()
}

// ---------------cache index-----------------------------
const (
	GroupKey             = "group_key"
	CommandKey           = "command_key"
	ResultKey            = "result_key"
	RemoteExecuteTimeKey = "remote_execute_time_key"
)

const (
	StatusSaved = iota
	StatusModified
)

var (
	ErrorNotFound      = fmt.Errorf("not found")
	ErrorRecordInvalid = fmt.Errorf("record invalid")
)

const (
	syncTick = 5 * time.Second
)

// 存储result cache的index
type Record map[string]string

func (r Record) Valid() bool {
	_, ok := r[GroupKey]
	if !ok {
		return false
	}

	_, ok = r[CommandKey]
	if !ok {
		return false
	}

	return true
}

func (r Record) GetStringByKey(key string) string {
	v, ok := r[key]
	if ok {
		return v
	}

	return ""
}

func (r Record) EqualByKey(another Record, key string) bool {
	if another == nil {
		return false
	}

	myvalue := ""
	if v, ok := r[key]; ok {
		myvalue = v
	} else {
		return false
	}

	anothervalue := ""
	if v, ok := another[key]; ok {
		anothervalue = v
	} else {
		return false
	}

	return myvalue == anothervalue
}

func (r Record) GreaterIntByKey(another Record, key string) bool {
	if another == nil {
		return false
	}

	var err error
	myvalue := 0
	if v, ok := r[key]; ok {
		myvalue, err = strconv.Atoi(v)
		if err != nil {
			return false
		}
	} else {
		return false
	}

	anothervalue := 0
	if v, ok := another[key]; ok {
		anothervalue, err = strconv.Atoi(v)
		if err != nil {
			return false
		}
	} else {
		return false
	}

	return myvalue > anothervalue
}

func (r *Record) ToString() string {
	jsonStr, err := json.Marshal(*r)
	if err == nil {
		return string(jsonStr)
	}

	return ""
}

type RecordGroup struct {
	lock       sync.RWMutex
	Key        string
	Group      []*Record
	LastStatus int32
}

func (rg *RecordGroup) PutRecord(record Record) {
	blog.Infof("resultcache: ready put record:%v", record)
	rg.lock.Lock()
	defer rg.lock.Unlock()

	// 要不要考虑去重?
	existed := false
	for i, v := range rg.Group {
		if v.EqualByKey(record, CommandKey) {
			existed = true
			rg.Group[i] = &record
		}
	}
	if !existed {
		rg.Group = append(rg.Group, &record)
	}

	rg.LastStatus = StatusModified
	blog.Infof("resultcache: finish put record:%v", record)
}

func (rg *RecordGroup) DeleteRecord(record Record) {
	rg.lock.Lock()
	defer rg.lock.Unlock()

	deleted := false
	// 先只考虑删除匹配上的第一个，后面看看要不要全部删除
	for index, v := range rg.Group {
		if record.EqualByKey(*v, CommandKey) {
			rg.Group = append(rg.Group[:index], rg.Group[index+1:]...)
			deleted = true
			break
		}
	}

	if deleted {
		rg.LastStatus = StatusModified
	}
}

func (rg *RecordGroup) ToBytes() ([]byte, error) {
	rg.lock.RLock()
	defer rg.lock.RUnlock()

	// Convert []*Record to JSON string
	jsonData, err := json.MarshalIndent(rg.Group, "", "  ")
	if err != nil {
		blog.Warnf("resultcache: convert to json with error:%v", err)
		return nil, err
	}

	return jsonData, nil
}

func (rg *RecordGroup) HitIndex(record Record) (bool, error) {
	rg.lock.RLock()
	defer rg.lock.RUnlock()

	for _, v := range rg.Group {
		if record.EqualByKey(*v, CommandKey) {
			if v.GreaterIntByKey(record, RemoteExecuteTimeKey) {
				return true, nil
			}
			break
		}
	}

	return false, nil
}

func (rg *RecordGroup) HitResult(record Record) (bool, error) {
	rg.lock.RLock()
	defer rg.lock.RUnlock()

	for _, v := range rg.Group {
		if record.EqualByKey(*v, ResultKey) {
			return true, nil
		}
	}

	return false, nil
}

func (rg *RecordGroup) ToString() string {
	str := ""
	for _, v := range rg.Group {
		str += v.ToString() + "\n"
	}
	return str
}

func ToRecordGroup(data []byte) (*RecordGroup, error) {
	var records []*Record
	err := json.Unmarshal(data, &records)
	if err != nil {
		blog.Warnf("resultcache: convert to record group with error:%v", err)
		return nil, err
	}

	if len(records) > 0 {
		if records[0].Valid() {
			key, _ := (*records[0])[GroupKey]
			return &RecordGroup{
				Key:        key,
				Group:      records,
				LastStatus: StatusSaved,
			}, nil
		}

		return nil, ErrorRecordInvalid
	}

	return nil, nil
}

// Table
type Table struct {
	lock     sync.RWMutex
	Data     map[string]*RecordGroup
	IndexDir string
}

func NewTable(dir string) *Table {
	return &Table{IndexDir: dir}
}

// PutRecord adds a new record to the table.
func (t *Table) PutRecord(record Record) error {
	if !record.Valid() {
		blog.Infof("resultcache: record:%v is invalid", record)
		return ErrorRecordInvalid
	}

	v, _ := record[GroupKey]

	t.lock.RLock()
	rg, ok := t.Data[v]
	t.lock.RUnlock()

	if !ok {
		t.lock.Lock()
		newg := RecordGroup{
			Key:        v,
			Group:      []*Record{&record},
			LastStatus: StatusModified,
		}
		t.Data[v] = &newg
		t.lock.Unlock()
	} else {
		rg.PutRecord(record)
	}

	return nil
}

// DeleteRecord delete one record from the table.
func (t *Table) DeleteRecord(record Record) error {
	if !record.Valid() {
		blog.Infof("resultcache: record:%v is invalid", record)
		return ErrorRecordInvalid
	}

	v, _ := record[GroupKey]

	t.lock.RLock()
	rg, ok := t.Data[v]
	t.lock.RUnlock()

	if !ok {
		blog.Warnf("resultcache: not found when delete with key:%s record:%v", v, record)
		return ErrorNotFound
	}

	rg.DeleteRecord(record)
	return nil
}

func (t *Table) GetRecordGroup(key string) ([]byte, error) {
	t.lock.RLock()
	rg, ok := t.Data[key]
	t.lock.RUnlock()

	if !ok {
		blog.Warnf("resultcache: not found when query with key:%s", key)
		return nil, ErrorNotFound
	}

	return rg.ToBytes()
}

const (
	indexSuffix = ".txt"
)

func (t *Table) Save(rg *RecordGroup) error {
	filename := fmt.Sprintf("%s%s", rg.Key, indexSuffix)
	fullpath := filepath.Join(t.IndexDir, filename)

	data, err := rg.ToBytes()
	if err != nil || len(data) <= 0 {
		blog.Infof("resultcache: save to file %s with error:%v", fullpath, err)
		return err
	}

	tmpfullpath := fullpath + ".tmp"
	err = os.WriteFile(tmpfullpath, data, 0644)
	if err != nil {
		blog.Infof("resultcache: save to file %s with error:%v", fullpath, err)
		return err
	}

	bakfullpath := fullpath + ".bak"
	if file.Stat(fullpath).Exist() {
		err = os.Rename(fullpath, bakfullpath)
		if err != nil {
			blog.Infof("resultcache: save to file %s with error:%v", fullpath, err)
			return err
		}
	}

	err = os.Rename(tmpfullpath, fullpath)
	if err != nil {
		blog.Infof("resultcache: save to file %s with error:%v", fullpath, err)
		return err
	}

	blog.Infof("resultcache: save to file %s with error:%v", fullpath, err)
	return err
}

func (t *Table) Init() {
	t.Load()

	// start ticker to manage
	go t.Ticker()
}

func (t *Table) Load() error {
	t.Data = make(map[string]*RecordGroup)

	f, err := os.Open(t.IndexDir)
	if err != nil {
		blog.Warnf("resultcache: open dir %s with error:%v", t.IndexDir, err)
		return err
	}
	fis, _ := f.Readdir(-1)
	f.Close()

	for _, fi := range fis {
		suffix := filepath.Ext(fi.Name())
		if suffix != indexSuffix {
			blog.Infof("resultcache: ignore file:%s", fi.Name())
			continue
		}

		fullpath := filepath.Join(t.IndexDir, fi.Name())
		file, err := os.Open(fullpath)
		if err != nil {
			blog.Warnf("resultcache: open file %s with error:%v", fullpath, err)
			continue
		}

		data, err := io.ReadAll(file)
		file.Close()

		if err != nil {
			blog.Warnf("resultcache: read file %s with error:%v", fullpath, err)
			continue
		}

		rg, err := ToRecordGroup(data)
		if err != nil {
			blog.Warnf("resultcache: convert [%s] to group with error:%v", data, err)
			continue
		}

		t.Data[rg.Key] = rg
		blog.Infof("resultcache: succeed load record group with key:%s from file:%s", rg.Key, fullpath)
	}

	return nil
}

func (t *Table) Ticker() {
	blog.Infof("resultcache: start ticker now")

	ticker := time.NewTicker(syncTick)
	defer ticker.Stop()

	// TODO : delete long unused
	for {
		select {
		case <-ticker.C:
			blog.Infof("resultcache: on ticker now...")
			t.lock.RLock()
			for _, rg := range t.Data {
				if rg.LastStatus == StatusModified {
					err := t.Save(rg)
					if err == nil {
						rg.LastStatus = StatusSaved
					}
				}
			}
			t.lock.RUnlock()
		}
	}
}
