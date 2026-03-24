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
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"time"

	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/protocol"
	dcUtil "github.com/TencentBlueKing/bk-turbo/src/backend/booster/bk_dist/common/util"
	"github.com/TencentBlueKing/bk-turbo/src/backend/booster/common/blog"
)

// define const vars for http client
const (
	// TCPBUFFERLEN = 10240

	// DEFAULTTIMEOUTSECS        = 300
	// DEFAULTREADALLTIMEOUTSECS = 600

	connectTimeout = 5 * time.Second
)

// TCPClient wrapper net.TCPConn
type TCPClient struct {
	timeout int
	conn    *net.TCPConn

	desc      string
	localport int32
}

// NewTCPClient return new TCPClient
func NewTCPClient(timeout int) *TCPClient {
	if timeout <= 0 {
		timeout = DefaultLongTCPTimeoutSeconds
	}

	return &TCPClient{
		timeout:   timeout,
		localport: -1,
	}
}

// NewTCPClientWithConn return new TCPClient with specified conn
func NewTCPClientWithConn(conn *net.TCPConn) *TCPClient {
	// not sure whether it can imporve performance
	err := conn.SetNoDelay(false)
	if err != nil {
		blog.Errorf("[longtcp] set no delay to false error: [%s]", err.Error())
	}

	return &TCPClient{
		conn:      conn,
		timeout:   DefaultLongTCPTimeoutSeconds,
		localport: -1,
	}
}

// Connect connect to server
func (c *TCPClient) Connect(server string) error {
	// resolve server
	resolvedserver, err := net.ResolveTCPAddr("tcp", server)
	if err != nil {
		blog.Errorf("[longtcp] server [%s] resolve error: [%s]", server, err.Error())
		return err
	}

	t := time.Now().Local()
	// c.conn, err = net.DialTCP("tcp", nil, resolvedserver)
	conn, err := net.DialTimeout("tcp", server, connectTimeout)
	d := time.Now().Sub(t)
	if d > 50*time.Millisecond {
		blog.Debugf("[longtcp] TCP Dail to long gt50 to server(%s): %s", resolvedserver, d.String())
	}
	if d > 200*time.Millisecond {
		blog.Debugf("[longtcp] TCP Dail to long gt200 to server(%s): %s", resolvedserver, d.String())
	}

	if err != nil {
		blog.Errorf("[longtcp] connect to server error: [%s]", err.Error())
		return err
	}

	// 将 net.Conn 转换为 net.TCPConn
	var ok bool
	c.conn, ok = conn.(*net.TCPConn)
	if !ok {
		err := fmt.Errorf("failed to conver net.Conn to net.TCPConn")
		blog.Errorf("connect to server error: [%v]", err)
		return err
	}

	blog.Debugf("[longtcp] succeed to connect to server [%s] ", server)
	// blog.Infof("succeed to establish connection [%s] ", c.ConnDesc())

	// not sure whether it can imporve performance
	err = c.conn.SetNoDelay(false)
	if err != nil {
		blog.Errorf("[longtcp] set no delay to false error: [%s]", err.Error())
	}

	return nil
}

// Closed check if the TCP connection is closed
func (c *TCPClient) Closed() bool {
	if c.conn == nil {
		return true
	}

	var buf [1]byte
	c.conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	_, err := c.conn.Read(buf[:])
	if err != nil {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			return false
		}
		blog.Infof("[longtcp] tcp connection %s closed with error:%v", c.RemoteAddr(), err)
		return true
	}
	return false
}

func (c *TCPClient) setIOTimeout(timeoutsecs int) error {
	if timeoutsecs <= 0 {
		return nil
	}

	t := time.Now()
	return c.conn.SetDeadline(t.Add(time.Duration(timeoutsecs) * time.Second))
}

// WriteData write data
func (c *TCPClient) WriteData(data []byte) error {
	if data == nil {
		return fmt.Errorf("input data is nil")
	}

	// if err := c.setIOTimeout(c.timeout); err != nil {
	// 	blog.Errorf("[longtcp] [%s] set io timeout error: [%s]", c.ConnDesc(), err.Error())
	// 	return err
	// }

	writelen := 0
	expectlen := len(data)
	for writelen < expectlen {
		ret, err := c.conn.Write((data)[writelen:])
		if err != nil {
			blog.Errorf("[longtcp] [%s]write token int error: [%s]", c.ConnDesc(), err.Error())
			return err
		}
		writelen += ret
	}

	if expectlen < 32 {
		blog.Debugf("[longtcp] [%s] send string '%s' ", c.ConnDesc(), string(data))
	} else {
		blog.Debugf("[longtcp] [%s] send string length [%d] ", c.ConnDesc(), expectlen)
	}
	return nil
}

// SendFile send file
func (c *TCPClient) SendFile(infile string, compress protocol.CompressType) error {
	blog.Debugf("[longtcp] ready write file [%s] with [%s]", infile, compress.String())

	data, err := ioutil.ReadFile(infile)
	if err != nil {
		blog.Debugf("[longtcp] failed to read file[%s]", infile)
		return err
	}

	switch compress {
	case protocol.CompressNone:
		if err := c.WriteData(data); err != nil {
			return err
		}
		return nil
	case protocol.CompressLZ4:
		// compress with lz4 firstly
		outdata, _ := dcUtil.Lz4Compress(data)
		outlen := len(outdata)
		blog.Debugf("[longtcp] compressed with lz4, from [%d] to [%d]", len(data), outlen)

		if err := c.WriteData(outdata); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown compress type [%s]", compress)
	}
}

// ReadData read data
func (c *TCPClient) ReadData(expectlen int) ([]byte, int, error) {
	// do not set timeout when read
	// if err := c.setIOTimeout(c.timeout); err != nil {
	// 	blog.Errorf("[longtcp] set io timeout error: [%s]", err.Error())
	// 	return nil, 0, err
	// }

	data := make([]byte, expectlen)
	var readlen int
	for readlen < expectlen {
		ret, err := c.conn.Read(data[readlen:])
		if err != nil {
			if err != io.EOF {
				blog.Warnf("[longtcp] [%s] read [%d] data with error: [%s]", c.ConnDesc(), readlen, err.Error())
				return nil, 0, err
			}

			readlen += ret
			blog.Debugf("[longtcp] [%s] EOF when read [%d] data", c.ConnDesc(), readlen)
			break
		}

		readlen += ret
		// blog.Debugf("[longtcp] received [%d] data ", readlen)
	}

	blog.Debugf("[longtcp] [%s] finished receive total [%d] data ", c.ConnDesc(), readlen)
	if readlen > 0 && readlen <= TotalLongTCPHeadLength {
		blog.Debugf("[longtcp] [%s] received data [%s] ", c.ConnDesc(), string(data))
	}

	return data, readlen, nil
}

// TryReadData try read data, return immediately after received any data
func (c *TCPClient) TryReadData(expectlen int) ([]byte, int, error) {
	// do not set timeout when read
	// if err := c.setIOTimeout(c.timeout); err != nil {
	// 	blog.Errorf("[longtcp] set io timeout error: [%s]", err.Error())
	// 	return nil, 0, err
	// }

	data := make([]byte, expectlen)
	var readlen int
	for readlen <= 0 {
		ret, err := c.conn.Read(data[readlen:])
		if err != nil {
			if err != io.EOF {
				blog.Errorf("[longtcp] read error: [%s]", err.Error())
				return nil, 0, err
			}

			readlen += ret
			blog.Debugf("[longtcp] EOF when read [%d] data", readlen)
			break
		}

		readlen += ret
	}

	if readlen < 32 {
		blog.Debugf("[longtcp] got string '%s'", string(data))
	} else {
		blog.Debugf("[longtcp] got string length [%d] ", readlen)
	}
	return data[0:readlen], readlen, nil
}

// ReadUntilEOF read data until EOF
func (c *TCPClient) ReadUntilEOF() ([]byte, int, error) {
	// do not set timeout when read
	// if err := c.setIOTimeout(DEFAULTREADALLTIMEOUTSECS); err != nil {
	// 	blog.Errorf("[longtcp] set io timeout error: [%s]", err.Error())
	// 	return nil, 0, err
	// }

	data := make([]byte, 1024)
	var readlen int
	for {
		ret, err := c.conn.Read(data[readlen:])
		if err != nil {
			if err != io.EOF {
				blog.Errorf("[longtcp] read error: [%s]", err.Error())
				return nil, 0, err
			}

			readlen += ret
			blog.Debugf("[longtcp] EOF when read [%d] data", readlen)
			break
		}

		readlen += ret
		if readlen >= len(data) {
			newdata := make([]byte, len(data)*2)
			copy(newdata[0:], data[:])
			data = newdata
		}
	}

	if readlen < 32 {
		blog.Debugf("[longtcp] got string '%s'", string(data))
	} else {
		blog.Debugf("[longtcp] got string length [%d] ", readlen)
	}
	return data, readlen, nil
}

func (c *TCPClient) SendMessages(messages []protocol.Message) error {
	blog.Debugf("[longtcp] send requests")

	if len(messages) == 0 {
		return fmt.Errorf("data to send is empty")
	}

	for _, v := range messages {
		if v.Data == nil {
			blog.Warnf("[longtcp] found nil data when ready send bk-common dist request")
			continue
		}

		switch v.Messagetype {
		case protocol.MessageString:
			if err := c.WriteData(v.Data); err != nil {
				return err
			}
		case protocol.MessageFile:
			if err := c.SendFile(string(v.Data), v.Compresstype); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown message type %s", v.Messagetype.String())
		}
	}

	return nil
}

// Close close conn
func (c *TCPClient) Close() error {
	blog.Debugf("[longtcp] ready close connection [%v] ", c.ConnDesc())
	return c.conn.Close()
}

// RemoteAddr return RemoteAddr
func (c *TCPClient) RemoteAddr() string {
	if c.conn != nil {
		return c.conn.RemoteAddr().String()
	}

	return ""
}

// ConnDesc return desc of conn
func (c *TCPClient) ConnDesc() string {
	if c.conn == nil {
		return ""
	}

	if c.desc != "" {
		return c.desc
	}

	c.desc = fmt.Sprintf("%s->%s", c.conn.LocalAddr().String(), c.conn.RemoteAddr().String())
	return c.desc
}

func (c *TCPClient) LocalPort() int32 {
	if c.localport != -1 {
		return c.localport
	}

	localAddr, ok := c.conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		c.localport = 0
	} else {
		c.localport = int32(localAddr.Port)
	}

	return c.localport
}
