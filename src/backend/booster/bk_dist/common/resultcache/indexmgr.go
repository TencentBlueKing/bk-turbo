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
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

type IndexMgr interface {
	Init()
	GetRecordGroup(key string) ([]byte, error)
	PutRecord(record Record) error
	DeleteRecord(record Record) error
}

const (
	GroupKey             = "group_key"
	CommandKey           = "command_key"
	ResultKey            = "result_key"
	RemoteExecuteTimeKey = "remote_execute_time_key"
	MachineIDKey         = "machine_id_key"
	UserKey              = "user_key"
	IPKey                = "ip_key"
	HitResultKey         = "hit_result_key"
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

	DefaultMaxIndexNumber = 50000

	keySeperator = "<|>"
)

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

func (r Record) GetARCKey() string {
	var gkey, ckey string
	gkey, _ = r[GroupKey]
	ckey, _ = r[CommandKey]

	return fmt.Sprintf("%s%s%s", gkey, keySeperator, ckey)
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

func (r Record) GreaterEqualIntByKey(another Record, key string) bool {
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

	return myvalue >= anothervalue
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
	blog.Infof("RecordGroup: ready put record:%v", record)
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
		// TODO : 增加上限控制
		rg.Group = append(rg.Group, &record)
	}

	rg.LastStatus = StatusModified
	blog.Infof("RecordGroup: finish put record:%v", record)
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
		blog.Warnf("RecordGroup: convert to json with error:%v", err)
		return nil, err
	}

	return jsonData, nil
}

func (rg *RecordGroup) HitIndex(record Record) (bool, error) {
	rg.lock.RLock()
	defer rg.lock.RUnlock()

	for _, v := range rg.Group {
		if record.EqualByKey(*v, CommandKey) {
			if v.GreaterEqualIntByKey(record, RemoteExecuteTimeKey) {
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

	return nil, ErrorRecordInvalid
}

// IndexMgrWithARC
type IndexMgrWithARC struct {
	lock     sync.RWMutex
	Data     map[string]*RecordGroup
	IndexDir string

	// 淘汰算法
	mutexARC sync.Mutex
	arc      *ARC
}

func NewIndexMgrWithARC(dir string, maxRecordNum int) IndexMgr {
	if maxRecordNum <= 0 {
		maxRecordNum = DefaultMaxIndexNumber
	}

	return &IndexMgrWithARC{
		IndexDir: dir,
		arc:      NewARC(maxRecordNum),
	}
}

// PutRecord adds a new record to the table.
func (t *IndexMgrWithARC) PutRecord(record Record) error {
	if !record.Valid() {
		blog.Infof("IndexMgrWithARC: record:%v is invalid", record)
		return ErrorRecordInvalid
	}

	// 如果命中了结果，则只刷新淘汰算法，不更新实际内容
	hitresult, ok := record[HitResultKey]
	if ok && hitresult == "true" {
		go t.onPutARC(record.GetARCKey())
		blog.Infof("IndexMgrWithARC: record:%v hit result file,only fresh elimination strategy", record)
		return nil
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

	go t.onPutARC(record.GetARCKey())

	return nil
}

// DeleteRecord delete one record from the table.
func (t *IndexMgrWithARC) DeleteRecord(record Record) error {
	if !record.Valid() {
		blog.Infof("IndexMgrWithARC: record:%v is invalid", record)
		return ErrorRecordInvalid
	}

	v, _ := record[GroupKey]

	t.lock.RLock()
	rg, ok := t.Data[v]
	t.lock.RUnlock()

	if !ok {
		blog.Warnf("IndexMgrWithARC: not found when delete with key:%s record:%v", v, record)
		return ErrorNotFound
	}

	rg.DeleteRecord(record)
	return nil
}

func (t *IndexMgrWithARC) GetRecordGroup(key string) ([]byte, error) {
	t.lock.RLock()
	rg, ok := t.Data[key]
	t.lock.RUnlock()

	if !ok {
		blog.Warnf("IndexMgrWithARC: not found when query with key:%s", key)
		return nil, ErrorNotFound
	}

	return rg.ToBytes()
}

const (
	indexSuffix = ".json"
)

func (t *IndexMgrWithARC) save(rg *RecordGroup) error {
	filename := fmt.Sprintf("%s%s", rg.Key, indexSuffix)
	fullpath := filepath.Join(t.IndexDir, filename)

	data, err := rg.ToBytes()
	if err != nil || len(data) <= 0 {
		blog.Infof("IndexMgrWithARC: save to file %s with error:%v", fullpath, err)
		return err
	}

	tmpfullpath := fullpath + ".tmp"
	err = os.WriteFile(tmpfullpath, data, 0644)
	if err != nil {
		blog.Infof("IndexMgrWithARC: save to file %s with error:%v", fullpath, err)
		return err
	}

	bakfullpath := fullpath + ".bak"
	if file.Stat(fullpath).Exist() {
		err = os.Rename(fullpath, bakfullpath)
		if err != nil {
			blog.Infof("IndexMgrWithARC: save to file %s with error:%v", fullpath, err)
			return err
		}
	}

	err = os.Rename(tmpfullpath, fullpath)
	if err != nil {
		blog.Infof("IndexMgrWithARC: save to file %s with error:%v", fullpath, err)
		return err
	}

	blog.Infof("IndexMgrWithARC: save to file %s with error:%v", fullpath, err)
	return err
}

func (t *IndexMgrWithARC) Init() {
	t.load()

	// start ticker to manage
	go t.ticker()
}

func (t *IndexMgrWithARC) load() error {
	t.Data = make(map[string]*RecordGroup)

	f, err := os.Open(t.IndexDir)
	if err != nil {
		blog.Warnf("IndexMgrWithARC: open dir %s with error:%v", t.IndexDir, err)
		return err
	}
	fis, _ := f.Readdir(-1)
	f.Close()

	for _, fi := range fis {
		suffix := filepath.Ext(fi.Name())
		if suffix != indexSuffix {
			blog.Infof("IndexMgrWithARC: ignore file:%s", fi.Name())
			continue
		}

		fullpath := filepath.Join(t.IndexDir, fi.Name())
		file, err := os.Open(fullpath)
		if err != nil {
			blog.Warnf("IndexMgrWithARC: open file %s with error:%v", fullpath, err)
			continue
		}

		data, err := io.ReadAll(file)
		file.Close()

		if err != nil {
			blog.Warnf("IndexMgrWithARC: read file %s with error:%v", fullpath, err)
			continue
		}

		rg, err := ToRecordGroup(data)
		if err != nil {
			blog.Warnf("IndexMgrWithARC: convert [%s] to group with error:%v", data, err)
			continue
		}

		t.Data[rg.Key] = rg
		blog.Infof("IndexMgrWithARC: succeed load record group with key:%s from file:%s", rg.Key, fullpath)
	}

	t.initArc()

	return nil
}

func (t *IndexMgrWithARC) initArc() {
	t.mutexARC.Lock()
	defer t.mutexARC.Unlock()

	for _, v := range t.Data {
		for _, r := range v.Group {
			_, evict, deleted := t.arc.Put(r.GetARCKey(), nil)
			if evict != nil {
				t.deleteIndex(evict.(string))
			}
			if deleted != nil {
				t.deleteIndex(deleted.(string))
			}
		}
	}
}

func (f *IndexMgrWithARC) onPutARC(key string) {
	blog.Infof("IndexMgrWithARC:on put key %s", key)

	f.mutexARC.Lock()
	defer f.mutexARC.Unlock()

	_, evict, deleted := f.arc.Put(key, nil)
	if evict != nil {
		f.deleteIndex(evict.(string))
	}
	if deleted != nil {
		f.deleteIndex(deleted.(string))
	}
}

func (f *IndexMgrWithARC) deleteIndex(key string) {
	blog.Infof("IndexMgrWithARC:ready delete index %s", key)

	fields := strings.Split(key, keySeperator)
	if len(fields) == 2 {
		tmprecord := Record{
			GroupKey:   fields[0],
			CommandKey: fields[1],
		}
		f.DeleteRecord(tmprecord)
	}
}

func (t *IndexMgrWithARC) ticker() {
	blog.Infof("IndexMgrWithARC: start ticker now")

	ticker := time.NewTicker(syncTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			blog.Debugf("IndexMgrWithARC: on ticker now...")
			t.lock.RLock()
			for _, rg := range t.Data {
				if rg.LastStatus == StatusModified {
					err := t.save(rg)
					if err == nil {
						rg.LastStatus = StatusSaved
					}
				}
			}
			t.lock.RUnlock()
		}
	}
}
