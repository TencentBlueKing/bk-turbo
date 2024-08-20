/*
 * Copyright (c) 2021 THL A29 Limited, a Tencent company. All rights reserved
 *
 * This source code file is licensed under the MIT License, you may obtain a copy of the License at
 *
 * http://opensource.org/licenses/MIT
 *
 */

package longtcp

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

var (
	ErrorContextCanceled        = fmt.Errorf("session canceled by context")
	ErrorConnectionInvalid      = fmt.Errorf("connection is invalid")
	ErrorResponseLengthInvalid  = fmt.Errorf("response data length is invalid")
	ErrorAllConnectionInvalid   = fmt.Errorf("all connections are invalid")
	ErrorSessionPoolCleaned     = fmt.Errorf("session pool cleaned")
	ErrorLessThanLongTCPHeadLen = fmt.Errorf("data length is less than long tcp head length")
	ErrorLongTCPHeadLenInvalid  = fmt.Errorf("data length of long tcp head length is invalid")
	ErrorWaitTimeout            = fmt.Errorf("wait response timeout")
)

const (
	// 二进制中命令唯一标识的长度
	UniqIDLength = 32
	// 二进制中记录数据长度的字段的长度
	DataLengthInBinary     = 16
	TotalLongTCPHeadLength = 48

	DefaultLongTCPTimeoutSeconds = 3600 * 24
	MinWaitSecs                  = 300

	checkWaitIntervalTime = 1 * time.Second
)

func longTCPHead2Byte(head *LongTCPHead) ([]byte, error) {
	if len(head.UniqID) != UniqIDLength {
		err := fmt.Errorf("token[%s] is invalid", head.UniqID)
		blog.Debugf("[longtcp] token error: [%v]", err)
		return nil, err
	}

	if head.DataLen < 0 {
		err := fmt.Errorf("data length[%d] is invalid", head.DataLen)
		blog.Debugf("[longtcp] data length error: [%v]", err)
		return nil, err
	}

	data := []byte(fmt.Sprintf("%32s%016x", head.UniqID, head.DataLen))
	return data, nil
}

func byte2LongTCPHead(data []byte) (*LongTCPHead, error) {
	if len(data) < TotalLongTCPHeadLength {
		blog.Errorf("[longtcp] long tcp head length is less than %d", TotalLongTCPHeadLength)
		return nil, ErrorLessThanLongTCPHeadLen
	}

	// check int value
	val, err := strconv.ParseInt(string(data[UniqIDLength:TotalLongTCPHeadLength]), 16, 64)
	if err != nil {
		blog.Errorf("[longtcp] read long tcp head length failed with error: [%v]", err)
		return nil, ErrorLongTCPHeadLenInvalid
	}

	return &LongTCPHead{
		UniqID:  MessageID(data[0:UniqIDLength]),
		DataLen: int(val),
	}, nil
}

// 固定长度UniqIDLength，用于识别命令
type MessageID string

type LongTCPHead struct {
	UniqID  MessageID // 固定长度，用于识别命令，在二进制中的长度固定为32位
	DataLen int       // 记录data的长度，在二进制中的长度固定为16位
}

type MessageResult struct {
	TCPHead *LongTCPHead
	Data    []byte
	Err     error
}

// 约束条件：
// 返回结果的 UniqID 需要保持不变，方便收到结果后，找到对应的chan
type Message struct {
	TCPHead      *LongTCPHead
	Data         [][]byte
	WaitResponse bool // 发送成功后，是否还需要等待对方返回结果
	RetChan      chan *MessageResult

	// 等待时间，由客户端指定;如果 MaxWaitSecs <= 0 ，则无限等待
	WaitStart   time.Time
	MaxWaitSecs int32

	F OnSendDoneFunc
}

func (m *Message) Desc() string {
	if m.TCPHead != nil {
		return string(m.TCPHead.UniqID)
	}

	return ""
}

type autoInc struct {
	sync.Mutex
	id uint64
}

type Session struct {
	ctx    context.Context
	cancel context.CancelFunc

	ip     string // 远端的ip
	port   int32  // 远端的port
	client *TCPClient
	valid  bool // 连接是否可用

	// 收到数据后的回调函数
	callback OnReceivedFunc

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

	// 记录当前session的请求数
	requestNum int64
}

// 处理收到的消息，一般是流程是将data转成需要的格式，然后业务逻辑处理，处理完，再通过 Session发送回去
type OnReceivedFunc func(id MessageID, data []byte, s *Session) error

// 发送完成后的回调
type OnSendDoneFunc func() error

// server端创建session
func NewSessionWithConn(conn *net.TCPConn, callback OnReceivedFunc) *Session {
	client := NewTCPClientWithConn(conn)

	if err := client.setIOTimeout(DefaultLongTCPTimeoutSeconds); err != nil {
		blog.Errorf("[longtcp] [%s] set io timeout error: [%s]", client.ConnDesc(), err.Error())
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
		ip:             ip,
		port:           int32(port),
		client:         client,
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

// client端创建session，需要指定目标server的ip和端口
// handshakedata : 用于建立握手协议的数据，当前兼容需要这个，后续不考虑兼容性，可以传nil
// callback : 收到数据后的处理函数
func NewSession(ip string, port int32, timeout int, handshakedata []byte, callback OnReceivedFunc) *Session {
	ctx, cancel := context.WithCancel(context.Background())

	server := fmt.Sprintf("%s:%d", ip, port)
	client := NewTCPClient(timeout)
	if err := client.Connect(server); err != nil {
		blog.Warnf("[longtcp] error: %v", err)
		cancel()
		return nil
	}

	if handshakedata != nil {
		if err := client.WriteData(handshakedata); err != nil {
			_ = client.Close()
			cancel()
			return nil
		}
	}

	sendNotifyChan := make(chan bool, 2)
	sendQueue := make([]*Message, 0, 10)
	errorChan := make(chan error, 2)

	s := &Session{
		ctx:            ctx,
		cancel:         cancel,
		ip:             ip,
		port:           port,
		client:         client,
		sendNotifyChan: sendNotifyChan,
		sendQueue:      sendQueue,
		waitMap:        make(map[MessageID]*Message),
		errorChan:      errorChan,
		valid:          true,
		callback:       callback,
		id:             0,
	}

	s.clientStart()

	blog.Infof("[longtcp] Dial to :%s:%d succeed", ip, port)

	return s
}

func (s *Session) Desc() string {
	if s.client != nil {
		return s.client.ConnDesc()
	}

	return ""
}

func (s *Session) sendRoutine(wg *sync.WaitGroup) {
	wg.Done()
	blog.Debugf("[longtcp] start server internal send...")
	for {
		select {
		case <-s.ctx.Done():
			blog.Infof("[longtcp] internal send canceled by context")
			for range s.sendNotifyChan {
				blog.Infof("[longtcp] [trace message] [session:%s] empty sendNotifyChan when context cancel",
					s.Desc())
			}
			return

		case <-s.sendNotifyChan:
			s.sendReal()
		}
	}
}

// copy all from send queue
func (s *Session) copyMessages() []*Message {
	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()

	if s.sendQueue != nil && len(s.sendQueue) > 0 {
		ret := make([]*Message, len(s.sendQueue), len(s.sendQueue))
		copy(ret, s.sendQueue)
		blog.Debugf("[longtcp] copied %d messages", len(ret))
		ids := []string{}
		for _, msg := range s.sendQueue {
			ids = append(ids, msg.Desc())
		}
		blog.Infof("[longtcp] [trace message] %v copied to real send queue", ids)
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

	if s.waitMap != nil {
		msg.WaitStart = time.Now()
		s.waitMap[msg.TCPHead.UniqID] = msg
		blog.Infof("[longtcp] [trace message] [%s] put to wait queue", msg.Desc())
	} else {
		blog.Warnf("[longtcp] [trace message] [%s] session invalid when put to wait queue", msg.Desc())
		return ErrorConnectionInvalid
	}

	return nil
}

// 删除等待的任务，并返回信息是否真正删除了
func (s *Session) removeWait(msg *Message) (bool, error) {
	s.waitMutex.Lock()
	defer s.waitMutex.Unlock()

	if s.waitMap != nil {
		if _, ok := s.waitMap[msg.TCPHead.UniqID]; ok {
			delete(s.waitMap, msg.TCPHead.UniqID)
			return true, nil
		}
	}

	return false, nil
}

// 发送结果
func (s *Session) returnWait(ret *MessageResult) error {
	s.waitMutex.Lock()
	defer s.waitMutex.Unlock()

	if s.waitMap != nil {
		if m, ok := s.waitMap[ret.TCPHead.UniqID]; ok {
			m.RetChan <- ret
			delete(s.waitMap, ret.TCPHead.UniqID)
			blog.Infof("[longtcp] [trace message] [%s] notified with response data ", ret.TCPHead.UniqID)
			return nil
		}
	}

	return fmt.Errorf("not found wait message with key %s", ret.TCPHead.UniqID)
}

// 生成唯一id
func (s *Session) uniqid() uint64 {
	s.idMutext.Lock()
	defer s.idMutext.Unlock()

	id := s.id
	s.id++

	return id
}

func formatID(id uint64, localport int32) MessageID {
	data := fmt.Sprintf("UNIQID_%011d_%013d", localport, id)
	return MessageID(data)
}

func (s *Session) encData2Message(
	data [][]byte,
	waitresponse bool,
	waitsecs int32,
	f OnSendDoneFunc) *Message {
	totallen := 0
	for _, v := range data {
		totallen += len(v)
	}

	wait := waitsecs
	if wait > 0 && wait < MinWaitSecs {
		wait = MinWaitSecs
	}

	return &Message{
		TCPHead: &LongTCPHead{
			UniqID:  formatID(s.uniqid(), s.client.LocalPort()),
			DataLen: totallen,
		},
		Data:         data,
		WaitResponse: waitresponse,
		RetChan:      make(chan *MessageResult, 1),
		MaxWaitSecs:  wait,
		F:            f,
	}
}

func (s *Session) encData2MessageWithID(id MessageID, data [][]byte, waitresponse bool) *Message {
	totallen := 0
	for _, v := range data {
		totallen += len(v)
	}
	return &Message{
		TCPHead: &LongTCPHead{
			UniqID:  id,
			DataLen: totallen,
		},
		Data:         data,
		WaitResponse: waitresponse,
		RetChan:      make(chan *MessageResult, 1),
	}
}

func (s *Session) onMessageError(m *Message, err error) {
	existed := false
	if m.WaitResponse {
		existed, _ = s.removeWait(m)
	}

	if existed {
		blog.Warnf("[longtcp] [trace message] [%s] notified with error:%v", m.Desc(), err)
		m.RetChan <- &MessageResult{
			Err:  err,
			Data: nil,
		}
	} else {
		blog.Warnf("[longtcp] [trace message] [%s] notified with error:%v, but not in wait map, do nothing",
			m.Desc(), err)
	}
}

// 取任务并依次发送
func (s *Session) sendReal() {
	blog.Debugf("[longtcp] session[%s] real send in...", s.Desc())
	msgs := s.copyMessages()

	for _, m := range msgs {
		blog.Infof("[longtcp] [trace message] [session:%s] [%s] start real send now",
			s.Desc(), m.Desc())
		if m.WaitResponse {
			err := s.putWait(m)
			if err != nil {
				if m.F != nil {
					m.F()
				}
				blog.Warnf("[longtcp] [trace message] [%s] notified with error:%v", m.Desc(), err)
				m.RetChan <- &MessageResult{
					Err:  err,
					Data: nil,
				}
				continue
			}
		}

		// encode long tcp head
		handshakedata, err := longTCPHead2Byte(m.TCPHead)
		if err != nil {
			blog.Warnf("[longtcp] session[%s] head to byte failed with error:%v", s.Desc(), err)
			if m.F != nil {
				m.F()
			}
			s.onMessageError(m, err)
			continue
		}

		// send long tcp head
		err = s.client.WriteData(handshakedata)
		if err != nil {
			blog.Warnf("[longtcp] session[%s] send header failed with error:%v", s.Desc(), err)
			if m.F != nil {
				m.F()
			}
			s.onMessageError(m, err)
			continue
		}

		blog.Debugf("[longtcp] session[%s] real sent header with ID [%s] ", s.Desc(), m.Desc())

		// send real data
		sendfailed := false
		for _, d := range m.Data {
			if d != nil {
				err = s.client.WriteData(d)
				if err != nil {
					blog.Warnf("[longtcp] session[%s] send body failed with error:%v", s.Desc(), err)
					if m.F != nil {
						m.F()
					}
					s.onMessageError(m, err)
					// continue
					sendfailed = true
					break
				}
			}
		}

		// skip to next message if failed, we should send all messages which has been copied here
		if sendfailed {
			continue
		}

		if m.F != nil {
			m.F()
		}

		blog.Debugf("[longtcp] session[%s] real sent body with ID [%s] ", s.Desc(), m.Desc())

		if !m.WaitResponse {
			blog.Infof("[longtcp] [trace message] [%s] notified after sent", m.Desc())
			m.RetChan <- &MessageResult{
				Err:  nil,
				Data: nil,
			}
		}
	}
}

func (s *Session) receiveRoutine(wg *sync.WaitGroup) {
	wg.Done()
	for {
		blog.Debugf("[longtcp] session[%s] start receive header", s.Desc())
		// TODO : implement receive data here
		// receive long tcp head firstly
		data, recvlen, err := s.client.ReadData(TotalLongTCPHeadLength)
		if err != nil {
			blog.Warnf("[longtcp] session[%s] receive header failed with error: %v", s.Desc(), err)
			s.errorChan <- err
			return
		}

		if recvlen == 0 {
			blog.Warnf("[longtcp] session[%s] receive header failed with error: %v", s.Desc(), io.EOF)
			s.errorChan <- io.EOF
			return
		}

		blog.Debugf("[longtcp] session[%s] received %d data", s.Desc(), recvlen)

		head, err := byte2LongTCPHead(data)
		if err != nil {
			blog.Errorf("[longtcp] session[%s] decode long tcp head failed with error: %v", s.Desc(), err)
			s.errorChan <- err
			return
		}

		blog.Debugf("[longtcp] session[%s] start receive body", s.Desc())
		// receive real data now
		data, recvlen, err = s.client.ReadData(int(head.DataLen))
		if err != nil {
			blog.Errorf("[longtcp] session[%s] receive body failed with error: %v", s.Desc(), err)
			s.errorChan <- err
			return
		}

		if recvlen == 0 {
			blog.Warnf("[longtcp] session[%s] receive body failed with error: %v", s.Desc(), io.EOF)
			s.errorChan <- io.EOF
			return
		}

		// blog.Debugf("[longtcp] session[%s] received %d data", s.Desc(), recvlen)
		blog.Infof("[longtcp] [trace message] [%s] received response data", head.UniqID)
		// TODO : decode msg, and call funtions to deal, and return response
		ret := &MessageResult{
			Err:     nil,
			TCPHead: head,
			Data:    data,
		}
		if ret.Err != nil {
			blog.Errorf("[longtcp] session[%s] received data is invalid with error: %v", s.Desc(), ret.Err)
			s.errorChan <- err
			return
		} else {
			blog.Debugf("[longtcp] session[%s] received request with ID: %s", s.Desc(), ret.TCPHead.UniqID)
			if s.callback != nil {
				go s.callback(ret.TCPHead.UniqID, ret.Data, s)
			} else {
				err = s.returnWait(ret)
				if err != nil {
					blog.Warnf("[longtcp] session[%s] notify wait message failed with error: %v", s.Desc(), err)
				}
			}
			blog.Debugf("[longtcp] session[%s] finishend notify result", s.Desc())
		}

		select {
		case <-s.ctx.Done():
			blog.Debugf("[longtcp] session[%s] internal recieve canceled by context", s.Desc())
			return
		default:
		}
	}
}

func (s *Session) safeSend(ch chan bool) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("send on closed chan")
		}
	}()

	ch <- true // panic if ch is closed
	err = nil
	return
}

func (s *Session) notifyAndWait(msg *Message) *MessageResult {
	blog.Debugf("[longtcp] notify send and wait for response now...")

	// TOOD : 拿锁并判断 s.Valid，避免这时连接已经失效
	s.sendMutex.Lock()

	if !s.valid {
		blog.Warnf("[longtcp] [trace message] [%s] session invalid when append to send queue", msg.Desc())
		s.sendMutex.Unlock()
		return &MessageResult{
			Err:  ErrorConnectionInvalid,
			Data: nil,
		}
	}

	s.sendQueue = append(s.sendQueue, msg)
	blog.Infof("[longtcp] [trace message] [%s] appended to send queue", msg.Desc())

	s.sendMutex.Unlock()

	blog.Debugf("[longtcp] notify by chan now, total %d messages now", len(s.sendQueue))
	// s.sendNotifyChan <- true
	err := s.safeSend(s.sendNotifyChan)
	if err != nil {
		blog.Warnf("[longtcp] [trace message] [%s] session closed when notify to send", msg.Desc())
		return &MessageResult{
			Err:  ErrorConnectionInvalid,
			Data: nil,
		}
	}

	blog.Infof("[longtcp] [trace message] [session:%s] [%s] notify and wait result after append",
		s.Desc(), msg.Desc())
	msgresult := <-msg.RetChan

	blog.Infof("[longtcp] [trace message] [session:%s] [%s] notify and wait got result now",
		s.Desc(), msg.Desc())

	return msgresult
}

// session 内部将data封装为Message发送，并通过chan接收发送结果，Message的id需要内部生成
// 如果 waitresponse为true，则需要等待返回的结果
func (s *Session) Send(
	data [][]byte,
	waitresponse bool,
	waitsecs int32,
	f OnSendDoneFunc) *MessageResult {
	if !s.valid {
		blog.Warnf("[longtcp] [trace message] connection invalid when send")
		return &MessageResult{
			Err:  ErrorConnectionInvalid,
			Data: nil,
		}
	}

	atomic.AddInt64(&s.requestNum, 1)
	defer atomic.AddInt64(&s.requestNum, -1)

	// data 转到 message
	msg := s.encData2Message(data, waitresponse, waitsecs, f)

	blog.Infof("[longtcp] [trace message] [session:%s] [%s] start notify and wait result",
		s.Desc(), msg.Desc())
	ret := s.notifyAndWait(msg)
	blog.Infof("[longtcp] [trace message] [session:%s] [%s] end notify and wait result",
		s.Desc(), msg.Desc())

	return ret
}

// session 内部将data封装为Message发送，并通过chan接收发送结果，这儿指定了id，无需自动生成
// 如果 waitresponse为true，则需要等待返回的结果
func (s *Session) SendWithID(id MessageID, data [][]byte, waitresponse bool) *MessageResult {
	if !s.valid {
		blog.Warnf("[longtcp] [trace message] [%s] connection invalid when send", id)
		return &MessageResult{
			Err:  ErrorConnectionInvalid,
			Data: nil,
		}
	}

	// data 转到 message
	msg := s.encData2MessageWithID(id, data, waitresponse)

	return s.notifyAndWait(msg)
}

// 启动收发协程
func (s *Session) clientStart() {
	blog.Debugf("[longtcp] ready start client go routines")

	// 先启动接收协程
	var wg1 = sync.WaitGroup{}
	wg1.Add(1)
	go s.receiveRoutine(&wg1)
	wg1.Wait()
	blog.Infof("[longtcp] go routine of client receive started!")

	// 再启动发送协程
	var wg2 = sync.WaitGroup{}
	wg2.Add(1)
	go s.sendRoutine(&wg2)
	wg2.Wait()
	blog.Infof("[longtcp] go routine of client send started!")

	// 最后启动状态检查协程
	var wg3 = sync.WaitGroup{}
	wg3.Add(1)
	go s.check(&wg3)
	wg3.Wait()
	blog.Infof("[longtcp] go routine of client check started!")
}

// 启动收发协程
func (s *Session) serverStart() {
	blog.Debugf("[longtcp] ready start server go routines")

	// 先启动发送协程
	var wg1 = sync.WaitGroup{}
	wg1.Add(1)
	go s.sendRoutine(&wg1)
	wg1.Wait()
	blog.Infof("[longtcp] go routine of server send started!")

	// 再启动接收协程
	var wg2 = sync.WaitGroup{}
	wg2.Add(1)
	go s.receiveRoutine(&wg2)
	wg2.Wait()
	blog.Infof("[longtcp] go routine of server receive started!")

	// 最后启动状态检查协程
	var wg3 = sync.WaitGroup{}
	wg3.Add(1)
	go s.check(&wg3)
	wg3.Wait()
	blog.Infof("[longtcp] go routine of server check started!")
}

func (s *Session) check(wg *sync.WaitGroup) {
	wg.Done()

	tick := time.NewTicker(checkWaitIntervalTime)
	defer tick.Stop()

	for {
		select {
		case <-s.ctx.Done():
			blog.Debugf("[longtcp] session check canceled by context")
			s.Clean(ErrorContextCanceled)
			return
		case err := <-s.errorChan:
			blog.Warnf("[longtcp] session %s found error:%v", s.Desc(), err)
			s.Clean(err)
			return
		case <-tick.C:
			s.checkWaitTimeout()
		}
	}
}

func (s *Session) safeClose(ch chan bool) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("close on closed chan")
		}
	}()

	close(ch) // panic if ch is closed
	err = nil
	return
}

// 清理资源，包括关闭连接，停止协程等
func (s *Session) Clean(err error) {
	// blog.Debugf("[longtcp] session %s clean now", s.Desc())
	blog.Infof("[longtcp] [trace message] [session:%s] ready clean now", s.Desc())

	s.sendMutex.Lock()

	// 通知发送队列中的任务
	blog.Infof("[longtcp] [trace message] [session:%s] has %d in send queue when clean",
		s.Desc(), len(s.sendQueue))
	if !s.valid { // 避免重复clean
		blog.Infof("[longtcp] session %s has cleaned before", s.Desc())
		return
	} else {
		s.valid = false
	}

	s.cancel()
	s.safeClose(s.sendNotifyChan)

	for _, m := range s.sendQueue {
		blog.Warnf("[longtcp] [trace message] [session:%s] [%s] notified in send queue with error:%v",
			s.Desc(), m.Desc(), err)
		m.RetChan <- &MessageResult{
			Err:  err,
			Data: nil,
		}
	}
	s.sendQueue = nil
	s.sendMutex.Unlock()

	// 通知等待结果的队列中的任务
	s.waitMutex.Lock()
	blog.Infof("[longtcp] [trace message] [session:%s] has %d in wait queue when clean",
		s.Desc(), len(s.waitMap))
	for k, m := range s.waitMap {
		blog.Warnf("[longtcp] [trace message] [session:%s] [%s] notified in wait queue with error:%v",
			s.Desc(), m.Desc(), err)
		m.RetChan <- &MessageResult{
			Err:  err,
			Data: nil,
		}
		delete(s.waitMap, k)
	}
	s.waitMap = nil
	s.waitMutex.Unlock()

	if s.client != nil {
		s.client.Close()
	}
}

func (s *Session) checkWaitTimeout() {
	s.waitMutex.Lock()
	defer s.waitMutex.Unlock()

	for _, m := range s.waitMap {
		if m.MaxWaitSecs > 0 &&
			m.WaitStart.Add(time.Duration(m.MaxWaitSecs)*time.Second).Before(time.Now()) {
			blog.Infof("[longtcp] [trace message] [%s] found message in session:[%s] timeout with %d seconds",
				m.Desc(),
				s.Desc(),
				m.MaxWaitSecs)
			delete(s.waitMap, m.TCPHead.UniqID)

			m.RetChan <- &MessageResult{
				Err:  ErrorWaitTimeout,
				Data: nil,
			}
		}
	}
}

func (s *Session) IsValid() bool {
	return s.valid
}

// 返回当前未完成的任务数
func (s *Session) Size() int64 {
	// return len(s.waitMap)
	return atomic.LoadInt64(&s.requestNum)
}

func (s *Session) IP() string {
	return s.ip
}

// ----------------------------------------------------
// 用于客户端的session pool
type ClientSessionPool struct {
	ctx    context.Context
	cancel context.CancelFunc

	ip            string // 远端的ip
	port          int32  // 远端的port
	timeout       int
	handshakedata []byte

	callback OnReceivedFunc
	size     int32
	sessions []*Session
	mutex    sync.RWMutex

	checkNotifyChan chan bool

	cleaned bool
}

const (
	checkSessionInterval = 20 * time.Second
)

var globalmutex sync.RWMutex
var globalSessionPools map[string]*ClientSessionPool
var globalSessionPoolsLocks map[string]*sync.RWMutex

type HandshadeData func() []byte

func getKey(ip string, port int32) string {
	return fmt.Sprintf("%s_%d", ip, port)
}

func lockSessionPool(key string) error {
	blog.Debugf("[longtcp] lock session [%s] in", key)

	// 要保证globalmutex的释放，避免死锁
	globalmutex.Lock()

	var lock *sync.RWMutex
	var ok bool
	if lock, ok = globalSessionPoolsLocks[key]; ok {
		blog.Debugf("[longtcp] lock [%s] existed", key)
	} else {
		lock = new(sync.RWMutex)
		globalSessionPoolsLocks[key] = lock
		blog.Debugf("[longtcp] lock [%s] not existed", key)
	}

	globalmutex.Unlock()

	lock.Lock()

	return nil
}

func unlockSessionPool(key string) error {
	blog.Debugf("[longtcp] unlock session [%s] in", key)

	// 要保证globalmutex的释放，避免死锁
	globalmutex.Lock()

	var lock *sync.RWMutex
	var ok bool
	if lock, ok = globalSessionPoolsLocks[key]; ok {
		blog.Debugf("[longtcp] lock [%s] existed when unlock", key)
	} else {
		blog.Warnf("[longtcp] lock [%s] not existed when unlock", key)
	}

	globalmutex.Unlock()

	if lock != nil {
		lock.Unlock()
	}

	return nil
}

func ensureSessionPoolInited() error {
	globalmutex.Lock()
	defer globalmutex.Unlock()

	if globalSessionPools == nil {
		globalSessionPools = make(map[string]*ClientSessionPool)
	}

	if globalSessionPoolsLocks == nil {
		globalSessionPoolsLocks = make(map[string]*sync.RWMutex)
	}

	return nil
}

// 用于初始化并返回session pool
func GetGlobalSessionPool(ip string, port int32, timeout int, callbackhandshake HandshadeData, size int32, callback OnReceivedFunc) *ClientSessionPool {
	blog.Debugf("[longtcp] get session pool with [%s:%d] in", ip, port)
	ensureSessionPoolInited()

	key := getKey(ip, port)

	lockSessionPool(key)
	defer unlockSessionPool(key)

	// 检查下是否有其它协程已经初始化过了
	v, ok := globalSessionPools[key]
	if ok {
		blog.Debugf("[longtcp] get session pool with [%s:%d] out with existed", ip, port)
		return v
	}

	// 长连接的超时时间不应该太短，不符合长连接的场景需求
	if timeout < DefaultLongTCPTimeoutSeconds {
		timeout = DefaultLongTCPTimeoutSeconds
	}

	ctx, cancel := context.WithCancel(context.Background())

	handshakedata := callbackhandshake()
	tempSessionPool := &ClientSessionPool{
		ctx:             ctx,
		cancel:          cancel,
		ip:              ip,
		port:            port,
		timeout:         timeout,
		handshakedata:   callbackhandshake(),
		callback:        callback,
		size:            size,
		sessions:        make([]*Session, size, size),
		checkNotifyChan: make(chan bool, size*2),
		cleaned:         false,
	}

	blog.Infof("[longtcp] client pool ready new client sessions to [%s:%d]", ip, port)
	failed := false
	for i := 0; i < int(size); i++ {
		if failed {
			blog.Warnf("[longtcp] session failed before with %s:%d,just let it nil", ip, port)
			tempSessionPool.sessions[i] = nil
			continue
		}

		client := NewSession(ip, port, timeout, handshakedata, callback)
		if client != nil {
			tempSessionPool.sessions[i] = client
		} else {
			failed = true
			blog.Warnf("[longtcp] client pool new client session failed with %s:%d", ip, port)
			tempSessionPool.sessions[i] = nil
		}
	}

	// 启动检查 session的协程，用于检查和恢复
	blog.Infof("[longtcp] client pool ready start check go routine")
	var wg = sync.WaitGroup{}
	wg.Add(1)
	go tempSessionPool.check(&wg)
	wg.Wait()

	globalSessionPools[key] = tempSessionPool

	blog.Debugf("[longtcp] get session pool with [%s:%d] out new pool", ip, port)

	return tempSessionPool
}

// func removeElement(arr []*ClientSessionPool, index int) []*ClientSessionPool {
// 	arr[len(arr)-1], arr[index] = arr[index], arr[len(arr)-1]
// 	return arr[:len(arr)-1]
// }

// 用于清理相应的session pool
func CleanGlobalSessionPool(ip string, port int32) {
	blog.Infof("[longtcp] clean session pool %s:%d in", ip, port)

	// globalmutex.Lock()
	// defer globalmutex.Unlock()

	key := getKey(ip, port)

	globalmutex.RLock()
	v, ok := globalSessionPools[key]
	globalmutex.RUnlock()

	if ok {
		lockSessionPool(key)
		v.Clean(ErrorSessionPoolCleaned)
		unlockSessionPool(key)
	}

	globalmutex.Lock()
	delete(globalSessionPools, key)
	delete(globalSessionPoolsLocks, key)
	globalmutex.Unlock()

	blog.Infof("[longtcp] clean session pool %s:%d out", ip, port)
}

// 获取可用session
func (sp *ClientSessionPool) GetSession() (*Session, error) {
	blog.Debugf("[longtcp] ready get session now")

	sp.mutex.RLock()
	// select most free
	var minsize int64 = 99999
	targetindex := -1
	needchecksession := false
	if !sp.cleaned {
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
	}
	sp.mutex.RUnlock()

	if sp.cleaned {
		return nil, ErrorSessionPoolCleaned
	}

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

func (sp *ClientSessionPool) Clean(err error) error {
	blog.Infof("[longtcp] clean sessions of pool %s:%d in", sp.ip, sp.port)
	sp.cancel()

	sp.mutex.Lock()
	defer sp.mutex.Unlock()

	if !sp.cleaned {
		sp.cleaned = true
		for _, s := range sp.sessions {
			if s != nil && s.IsValid() {
				s.Clean(err)
			}
		}
	}

	blog.Infof("[longtcp] clean sessions of %s:%d out", sp.ip, sp.port)

	return nil
}

func (sp *ClientSessionPool) check(wg *sync.WaitGroup) {
	wg.Done()
	ticker := time.NewTicker(checkSessionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sp.ctx.Done():
			blog.Infof("[longtcp] session pool %s:%d check routine canceled by context", sp.ip, sp.port)
			return
		case <-sp.checkNotifyChan:
			blog.Infof("[longtcp] session pool %s:%d check triggled by notify", sp.ip, sp.port)
			sp.checkSessions()
		case <-ticker.C:
			blog.Debugf("[longtcp] session pool %s:%d check triggled by ticker", sp.ip, sp.port)
			sp.checkSessions()
		}
	}
}

func (sp *ClientSessionPool) checkSessions() {
	sp.mutex.Lock()
	defer sp.mutex.Unlock()

	if sp.cleaned {
		blog.Infof("[longtcp] session pool %s:%d has cleaned, do not check now", sp.ip, sp.port)
		return
	}

	invalidIndex := make([]int, 0)
	for i, s := range sp.sessions {
		if s == nil || !s.IsValid() {
			invalidIndex = append(invalidIndex, i)
		}
	}

	if len(invalidIndex) > 0 {
		blog.Infof("[longtcp] session pool %s:%d check found %d invalid sessions", sp.ip, sp.port, len(invalidIndex))

		for i := 0; i < len(invalidIndex); i++ {
			client := NewSession(sp.ip, sp.port, sp.timeout, sp.handshakedata, sp.callback)
			if client != nil {
				sessionid := invalidIndex[i]
				if sp.sessions[sessionid] != nil {
					sp.sessions[sessionid].Clean(ErrorConnectionInvalid)
				}
				sp.sessions[sessionid] = client
				blog.Infof("[longtcp] update %dth session to new %s", sessionid, client.Desc())
			} else {
				// TODO : if failed ,we should try later?
				blog.Warnf("[longtcp] check session pool %s:%d, failed to NewSession", sp.ip, sp.port)
				return
			}
		}
	}
}
