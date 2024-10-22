/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package websocket

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
	"github.com/emicklei/go-restful"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

var (
	ErrorContextCanceled       = fmt.Errorf("session canceled by context")
	ErrorConnectionInvalid     = fmt.Errorf("connection is invalid")
	ErrorResponseLengthInvalid = fmt.Errorf("response data length is invalid")
	ErrorAllConnectionInvalid  = fmt.Errorf("all connections are invalid")
)

const (
	DefaultIP   = "127.0.0.1"
	DefaultPort = 30117

	UniqIDLength = 32
)

// 固定长度UniqIDLength，用于识别命令
type MessageID string

type MessageResult struct {
	Err    error
	UniqID MessageID // 固定长度，用于识别命令
	Data   []byte
}

// 约束条件：
// 返回结果的 UniqID 需要保持不变，方便收到结果后，找到对应的chan
type Message struct {
	UniqID MessageID // 固定长度，用于识别命令
	// op           ws.OpCode  // 先用固定的 Binary
	Data         []byte
	WaitResponse bool // 发送成功后，是否还需要等待对方返回结果
	RetChan      chan *MessageResult
}

type autoInc struct {
	sync.Mutex
	id uint64
}

type Session struct {
	ctx    context.Context
	cancel context.CancelFunc

	req *restful.Request

	ip      string // 远端的ip
	port    int32  // 远端的port
	conn    net.Conn
	connKey string
	valid   bool // 连接是否可用

	// 收到数据后的回调函数
	callback WebSocketFunc

	// 发送
	sendQueue      []*Message
	sendMutex      sync.RWMutex
	sendNotifyChan chan bool // 唤醒发送协程

	// 等待返回结果
	waitMap   map[MessageID]*Message
	waitMutex sync.RWMutex

	// 记录出现的error
	errorChan chan error

	// 生成唯一id
	idMutext sync.Mutex
	id       uint64
}

// 处理收到的消息，一般是流程是将data转成需要的格式，然后业务逻辑处理，处理完，再通过 Session发送回去
type WebSocketFunc func(r *restful.Request, id MessageID, data []byte, s *Session) error

// server端创建session，需要指定http处理函数
func NewServerSession(w *restful.Response, r *restful.Request, callback WebSocketFunc) *Session {
	conn, _, _, err := ws.UpgradeHTTP(r.Request, w)
	if err != nil || conn == nil {
		blog.Errorf("[session] UpgradeHTTP failed with error:%v", err)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	remoteaddr := conn.RemoteAddr().String()
	ip := ""
	var port int
	// The port starts after the last colon.
	i := strings.LastIndex(remoteaddr, ":")
	if i > 0 && i < len(remoteaddr)-1 {
		ip = remoteaddr[:i]
		port, _ = strconv.Atoi(remoteaddr[i+1:])
	}

	sendNotifyChan := make(chan bool, 2)
	sendQueue := make([]*Message, 0, 10)
	errorChan := make(chan error, 2)

	s := &Session{
		ctx:            ctx,
		cancel:         cancel,
		req:            r,
		ip:             ip,
		port:           int32(port),
		conn:           conn,
		connKey:        fmt.Sprintf("%s_%d", remoteaddr, time.Now().Nanosecond()),
		sendNotifyChan: sendNotifyChan,
		sendQueue:      sendQueue,
		waitMap:        make(map[MessageID]*Message),
		errorChan:      errorChan,
		valid:          true,
		callback:       callback,
		id:             0,
	}

	s.serverStart()

	return s
}

func (s *Session) GetConnKey() string {
	return s.connKey
}

// client端创建session，需要指定目标server的ip和端口
func NewClientSession(ip string, port int32, url string, callback WebSocketFunc) *Session {
	ctx, cancel := context.WithCancel(context.Background())
	conn, _, _, err := ws.DefaultDialer.Dial(ctx, fmt.Sprintf("ws://%s:%d/%s", ip, port, url))
	if err != nil || conn == nil {
		blog.Errorf("[session] Dial to :%s:%d/%s failed with error:%v", ip, port, url, err)
		cancel()
		return nil
	}

	sendNotifyChan := make(chan bool, 2)
	sendQueue := make([]*Message, 0, 10)
	errorChan := make(chan error, 2)

	s := &Session{
		ctx:            ctx,
		cancel:         cancel,
		ip:             ip,
		port:           port,
		conn:           conn,
		sendNotifyChan: sendNotifyChan,
		sendQueue:      sendQueue,
		waitMap:        make(map[MessageID]*Message),
		errorChan:      errorChan,
		valid:          true,
		callback:       callback,
		id:             0,
	}

	s.clientStart()

	blog.Infof("[session] Dial to :%s:%d/%s succeed", ip, port, url)

	return s
}

func (s *Session) clientSend(wg *sync.WaitGroup) {
	wg.Done()
	// blog.Debugf("[session] start client internal send...")
	for {
		select {
		case <-s.ctx.Done():
			blog.Debugf("[session] internal send canceled by context")
			return

		case <-s.sendNotifyChan:
			s.clientSendReal()
		}
	}
}

func (s *Session) serverSend(wg *sync.WaitGroup) {
	wg.Done()
	blog.Debugf("[session] start server internal send...")
	for {
		select {
		case <-s.ctx.Done():
			blog.Debugf("[session] internal send canceled by context")
			return

		case <-s.sendNotifyChan:
			s.serverSendReal()
		}
	}
}

// copy all from send queue
func (s *Session) copyMessages() []*Message {
	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()

	if len(s.sendQueue) > 0 {
		ret := make([]*Message, len(s.sendQueue), len(s.sendQueue))
		copy(ret, s.sendQueue)
		blog.Debugf("[session] copied %d messages", len(ret))
		s.sendQueue = make([]*Message, 0, 10)
		return ret
	} else {
		return nil
	}
}

// 添加等待的任务
func (s *Session) putWait(msg *Message) error {
	s.waitMutex.Lock()
	defer s.waitMutex.Unlock()

	s.waitMap[msg.UniqID] = msg

	return nil
}

// 删除等待的任务
func (s *Session) removeWait(msg *Message) error {
	s.waitMutex.Lock()
	defer s.waitMutex.Unlock()

	delete(s.waitMap, msg.UniqID)

	return nil
}

// 发送结果
func (s *Session) returnWait(ret *MessageResult) error {
	s.waitMutex.Lock()
	defer s.waitMutex.Unlock()

	if m, ok := s.waitMap[ret.UniqID]; ok {
		m.RetChan <- ret
		delete(s.waitMap, ret.UniqID)
		return nil
	}

	return fmt.Errorf("not found wait message with key %s", ret.UniqID)
}

// 生成唯一id
func (s *Session) uniqid() uint64 {
	s.idMutext.Lock()
	defer s.idMutext.Unlock()

	id := s.id
	s.id++

	return id
}

func formatID(id uint64) MessageID {
	data := fmt.Sprintf("UNIQID%26x", id)
	return MessageID(data)
}

func (s *Session) encData2Message(data []byte, waitresponse bool) *Message {
	return &Message{
		UniqID:       formatID(s.uniqid()),
		Data:         data,
		WaitResponse: waitresponse,
		RetChan:      make(chan *MessageResult, 1),
	}
}

func (s *Session) encData2MessageWithID(id MessageID, data []byte, waitresponse bool) *Message {
	return &Message{
		UniqID:       id,
		Data:         data,
		WaitResponse: waitresponse,
		RetChan:      make(chan *MessageResult, 1),
	}
}

func (s *Session) decData2MessageResult(data []byte) *MessageResult {
	if len(data) < UniqIDLength {
		return &MessageResult{
			Err:    ErrorResponseLengthInvalid,
			UniqID: "",
			Data:   nil,
		}
	}

	return &MessageResult{
		Err:    nil,
		UniqID: MessageID(data[:UniqIDLength]),
		Data:   data[UniqIDLength:],
	}
}

// 取任务并依次发送
func (s *Session) clientSendReal() {
	// blog.Debugf("[session] start real client send now")
	msgs := s.copyMessages()
	// blog.Debugf("[session] start real client send, got %d messages", len(msgs))

	for _, m := range msgs {
		if m.WaitResponse {
			s.putWait(m)
		}

		totallen := len(m.UniqID) + len(m.Data)
		fulldata := make([]byte, totallen, totallen)
		copy(fulldata, m.UniqID)
		copy(fulldata[len(m.UniqID):], m.Data)
		err := wsutil.WriteClientMessage(s.conn, ws.OpBinary, fulldata)
		if err != nil {
			if m.WaitResponse {
				s.removeWait(m)
			}

			m.RetChan <- &MessageResult{
				Err:  err,
				Data: nil,
			}
			continue
		}

		if !m.WaitResponse {
			m.RetChan <- &MessageResult{
				Err:  nil,
				Data: nil,
			}
		}
	}
}

// 取任务并依次发送
func (s *Session) serverSendReal() {
	msgs := s.copyMessages()

	for _, m := range msgs {
		if m.WaitResponse {
			s.putWait(m)
		}

		totallen := len(m.UniqID) + len(m.Data)
		fulldata := make([]byte, totallen, totallen)
		copy(fulldata, m.UniqID)
		copy(fulldata[len(m.UniqID):], m.Data)
		err := wsutil.WriteServerMessage(s.conn, ws.OpBinary, fulldata)
		if err != nil {
			if m.WaitResponse {
				s.removeWait(m)
			}

			m.RetChan <- &MessageResult{
				Err:  err,
				Data: nil,
			}
			continue
		}

		if !m.WaitResponse {
			m.RetChan <- &MessageResult{
				Err:  nil,
				Data: nil,
			}
		}
	}
}

func (s *Session) clientReceive(wg *sync.WaitGroup) {
	wg.Done()
	for {
		msg, _, err := wsutil.ReadServerData(s.conn)
		if err != nil {
			blog.Debugf("[session] receive message failed with error: %v", err)
			s.errorChan <- err
			return
		}

		// blog.Debugf("[session] received data: %s", string(msg))
		ret := s.decData2MessageResult(msg)
		if ret.Err != nil {
			blog.Warnf("[session] received data is invalid with error: %v", ret.Err)
			s.errorChan <- err
			return
		} else {
			blog.Debugf("[session] received response with ID: %s", ret.UniqID)
			if s.callback != nil {
				go s.callback(s.req, ret.UniqID, ret.Data, s)
			} else {
				err = s.returnWait(ret)
				if err != nil {
					blog.Warnf("[session] notify wait message failed with error: %v", err)
				}
			}
		}

		select {
		case <-s.ctx.Done():
			blog.Debugf("[session] internal recieve canceled by context")
			return
		default:
		}
	}
}

func (s *Session) serverReceive(wg *sync.WaitGroup) {
	wg.Done()
	for {
		msg, _, err := wsutil.ReadClientData(s.conn)
		if err != nil {
			blog.Errorf("[session] receive message failed with error: %v", err)
			s.errorChan <- err
			return
		}

		blog.Debugf("[session] received data: %s", string(msg))
		// TODO : decode msg, and call funtions to deal, and return response
		ret := s.decData2MessageResult(msg)
		if ret.Err != nil {
			blog.Errorf("[session] received data is invalid with error: %v", ret.Err)
			s.errorChan <- err
			return
		} else {
			blog.Debugf("[session] received request with ID: %s", ret.UniqID)
			if s.callback != nil {
				go s.callback(s.req, ret.UniqID, ret.Data, s)
			} else {
				err = s.returnWait(ret)
				if err != nil {
					blog.Warnf("[session] notify wait message failed with error: %v", err)
				}
			}
		}

		select {
		case <-s.ctx.Done():
			blog.Debugf("[session] internal recieve canceled by context")
			return
		default:
		}
	}
}

func (s *Session) notifyAndWait(msg *Message) *MessageResult {
	blog.Debugf("[session] notify send and wait for response now...")

	// TOOD : 拿锁并判断 s.Valid，避免这时连接已经失效
	s.sendMutex.Lock()

	if !s.valid {
		s.sendMutex.Unlock()
		return &MessageResult{
			Err:  ErrorConnectionInvalid,
			Data: nil,
		}
	}

	s.sendQueue = append(s.sendQueue, msg)
	s.sendMutex.Unlock()

	blog.Debugf("[session] notify by chan now, total %d messages now", len(s.sendQueue))
	s.sendNotifyChan <- true

	msgresult := <-msg.RetChan
	return msgresult
}

// session 内部将data封装为Message发送，并通过chan接收发送结果，Message的id需要内部生成
// 如果 waitresponse为true，则需要等待返回的结果
func (s *Session) Send(data []byte, waitresponse bool) *MessageResult {
	if !s.valid {
		return &MessageResult{
			Err:  ErrorConnectionInvalid,
			Data: nil,
		}
	}

	// data 转到 message
	msg := s.encData2Message(data, waitresponse)

	return s.notifyAndWait(msg)
}

// session 内部将data封装为Message发送，并通过chan接收发送结果，这儿指定了id，无需自动生成
// 如果 waitresponse为true，则需要等待返回的结果
func (s *Session) SendWithID(id MessageID, data []byte, waitresponse bool) *MessageResult {
	if !s.valid {
		return &MessageResult{
			Err:  ErrorConnectionInvalid,
			Data: nil,
		}
	}

	// data 转到 message
	msg := s.encData2MessageWithID(id, data, waitresponse)

	return s.notifyAndWait(msg)
}

// 启动收发协程，方便协程退出
func (s *Session) clientStart() {
	blog.Debugf("[session] ready start client go routines")

	// 先启动接收协程
	var wg1 = sync.WaitGroup{}
	wg1.Add(1)
	go s.clientReceive(&wg1)
	wg1.Wait()
	blog.Infof("[session] go routine of client receive started!")

	// 再启动发送协程
	var wg2 = sync.WaitGroup{}
	wg2.Add(1)
	go s.clientSend(&wg2)
	wg2.Wait()
	blog.Infof("[session] go routine of client send started!")

	// 最后启动状态检查协程
	var wg3 = sync.WaitGroup{}
	wg3.Add(1)
	go s.check(&wg3)
	wg3.Wait()
	blog.Infof("[session] go routine of client check started!")
}

// 启动收发协程，方便协程退出
func (s *Session) serverStart() {
	blog.Debugf("[session] ready start server go routines")

	// 先启动发送协程
	var wg1 = sync.WaitGroup{}
	wg1.Add(1)
	go s.serverSend(&wg1)
	wg1.Wait()
	blog.Infof("[session] go routine of server send started!")

	// 再启动接收协程
	var wg2 = sync.WaitGroup{}
	wg2.Add(1)
	go s.serverReceive(&wg2)
	wg2.Wait()
	blog.Infof("[session] go routine of server receive started!")

	// 最后启动状态检查协程
	var wg3 = sync.WaitGroup{}
	wg3.Add(1)
	go s.check(&wg3)
	wg3.Wait()
	blog.Infof("[session] go routine of server check started!")
}

func (s *Session) check(wg *sync.WaitGroup) {
	wg.Done()
	for {
		select {
		// 在用户环境上发现s.ctx.Done()异常触发的，不清楚原因，先屏蔽
		// case <-s.ctx.Done():
		// 	blog.Infof("[session] session check canceled by context")
		// 	s.clean(ErrorContextCanceled)
		// 	return
		case err := <-s.errorChan:
			blog.Warnf("[session] session check found error:%v", err)
			s.clean(err)
			return
		}
	}
}

// 清理资源，包括关闭连接，停止协程等
// 在Run中被调用，不对外
func (s *Session) clean(err error) {
	blog.Infof("[session] session clean now")
	s.cancel()
	s.conn.Close()

	// 通知发送队列中的任务
	s.sendMutex.Lock()
	s.valid = false
	for _, m := range s.sendQueue {
		m.RetChan <- &MessageResult{
			Err:  err,
			Data: nil,
		}
	}
	s.sendMutex.Unlock()

	// 通知等待结果的队列中的任务
	s.waitMutex.Lock()
	for _, m := range s.waitMap {
		m.RetChan <- &MessageResult{
			Err:  err,
			Data: nil,
		}
	}
	s.waitMutex.Unlock()
}

func (s *Session) IsValid() bool {
	return s.valid
}

// 返回当前session的任务数
func (s *Session) Size() int {
	return len(s.sendQueue)
}

// ----------------------------------------------------
// 用于客户端的session pool
type ClientSessionPool struct {
	ctx    context.Context
	cancel context.CancelFunc

	ip       string // 远端的ip
	port     int32  // 远端的port
	url      string
	callback WebSocketFunc
	size     int32
	sessions []*Session
	mutex    sync.RWMutex

	checkNotifyChan chan bool
}

const (
	checkSessionInterval = 3
)

var globalmutex sync.RWMutex
var globalSessionPool *ClientSessionPool

// 用于初始化并返回全局客户端的session pool
func GetGlobalSessionPool(ip string, port int32, url string, size int32, callback WebSocketFunc) *ClientSessionPool {
	// 初始化全局pool，并返回一个可用的
	if globalSessionPool != nil {
		return globalSessionPool
	}

	globalmutex.Lock()
	defer globalmutex.Unlock()

	if globalSessionPool != nil {
		return globalSessionPool
	}

	ctx, cancel := context.WithCancel(context.Background())

	tempSessionPool := &ClientSessionPool{
		ctx:             ctx,
		cancel:          cancel,
		ip:              ip,
		port:            port,
		url:             url,
		callback:        callback,
		size:            size,
		sessions:        make([]*Session, size, size),
		checkNotifyChan: make(chan bool, size*2),
	}

	blog.Infof("[session] client pool ready new client sessions")
	for i := 0; i < int(size); i++ {
		client := NewClientSession(ip, port, url, callback)
		if client != nil {
			tempSessionPool.sessions[i] = client
		} else {
			blog.Warnf("[session] client pool new client session failed with %s:%d/%s", ip, port, url)
			tempSessionPool.sessions[i] = nil
		}
	}

	// 启动检查 session的协程，用于检查和恢复
	blog.Infof("[session] client pool ready start check go routine")
	var wg = sync.WaitGroup{}
	wg.Add(1)
	go tempSessionPool.check(&wg)
	wg.Wait()

	globalSessionPool = tempSessionPool
	return globalSessionPool
}

func DestroyGlobalSessionPool() error {
	globalmutex.Lock()
	defer globalmutex.Unlock()

	if globalSessionPool != nil {
		globalSessionPool.Destroy(nil)
		globalSessionPool = nil
	}

	return nil
}

// 获取可用session
func (sp *ClientSessionPool) GetSession() (*Session, error) {
	blog.Debugf("[session] ready get session now")

	sp.mutex.RLock()
	// select most free
	minsize := 99999
	targetindex := -1
	needchecksession := false
	for i, s := range sp.sessions {
		if s != nil && s.IsValid() {
			if s.Size() < minsize {
				targetindex = i
				minsize = s.Size()
			}
		} else {
			needchecksession = true
		}
	}
	sp.mutex.RUnlock()

	// 注意，如果chan满了，则下面通知会阻塞，所以这时不要持锁，避免其它地方需要锁，导致死锁
	if needchecksession {
		sp.checkNotifyChan <- true
	}

	if targetindex >= 0 {
		return sp.sessions[targetindex], nil
	}

	// TODO : whether allowd dynamic growth?
	return nil, ErrorAllConnectionInvalid
}

func (sp *ClientSessionPool) Destroy(err error) error {
	blog.Infof("[session] session pool destroy now")
	sp.cancel()

	sp.mutex.Lock()
	defer sp.mutex.Unlock()

	for _, s := range sp.sessions {
		if s != nil && s.IsValid() {
			s.clean(err)
		}
	}

	return nil
}

func (sp *ClientSessionPool) check(wg *sync.WaitGroup) {
	wg.Done()
	for {
		select {
		case <-sp.ctx.Done():
			blog.Infof("[session] session pool check routine canceled by context")
			return
		case <-sp.checkNotifyChan:
			blog.Debugf("[session] session check triggled")
			sp.checkSessions()
		}
	}
}

func (sp *ClientSessionPool) checkSessions() {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()

	invalidIndex := make([]int, 0)
	for i, s := range sp.sessions {
		if s == nil || !s.IsValid() {
			invalidIndex = append(invalidIndex, i)
		}
	}

	if len(invalidIndex) > 0 {
		blog.Debugf("[session] session check found %d invalid sessions", len(invalidIndex))

		for i := 0; i < len(invalidIndex); i++ {
			client := NewClientSession(sp.ip, sp.port, sp.url, sp.callback)
			if client != nil {
				blog.Debugf("[session] got Lock")
				sessionid := invalidIndex[i]
				if sp.sessions[sessionid] != nil {
					sp.sessions[sessionid].clean(ErrorConnectionInvalid)
				}
				sp.sessions[sessionid] = client
				blog.Debugf("[session] update %dth session to new", sessionid)
			} else {
				// TODO : if failed ,we should try later?
				blog.Debugf("[session] check sessions failed to NewClientSession")
				time.Sleep(time.Duration(checkSessionInterval) * time.Second)
				go func() {
					sp.checkNotifyChan <- true
				}()
				return
			}
		}
	}
}
