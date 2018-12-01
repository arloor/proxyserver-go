package connection

import (
	"bytes"
	"fmt"
	"github.com/arloor/proxyserver/util"
	"log"
	"net"
	"sync"
	"time"
)

var num = 0

var CONNECT = []byte("CONNECT")
var GET = []byte("GET")
var POST = []byte("POST")
var PUT = []byte("PUT")
var DELETE = []byte("DELETE")
var TRACE = []byte("TRACE")
var OPTIONS = []byte("OPTIONS")
var HEAD = []byte("HEAD")

type ProxyConn struct {
	ID         int
	ConnClient net.Conn
	ConnServer net.Conn
	host       string
	port       string
	closed     bool        //是否已经关闭
	isTunnel   bool        //该条连接是不是tunnel隧道
	lock       *sync.Mutex //锁来保护所有操作，需要确保所有修改都被此锁保护
	beforeCRLF []byte
}

//构造方法
func NewProxyConn(connClient net.Conn) *ProxyConn {
	proxyConn := new(ProxyConn)
	proxyConn.lock = new(sync.Mutex)
	proxyConn.ID = num
	num++
	proxyConn.ConnClient = connClient
	return proxyConn
}

//将读到的字节，加入结构体的beforeCRLF,并检测是否是完整的beforeCRLF.如果超过4096还不是正确的，则返回tolong true如果，正确parse，则返回success，
func (proxyConn *ProxyConn) AppendContentAndTryParseAndConnect(buf []byte) (connected, invalid bool) {
	var hostAndport []byte
	proxyConn.beforeCRLF = append(proxyConn.beforeCRLF, buf...)
	if len(proxyConn.beforeCRLF) > 4096 || !startWithHttpMethod(proxyConn.beforeCRLF) { //过长或不以http方法开始
		return false, true
	} else { //是以http方法开头的
		//下面parse出host,port
		endOfBeforeCRLF := bytes.Index(proxyConn.beforeCRLF, []byte("\r\n\r\n"))
		if endOfBeforeCRLF == -1 {
			return false, true
		} else {
			splits := bytes.Split(proxyConn.beforeCRLF[:endOfBeforeCRLF], []byte("\r\n"))
			for i, line := range splits {
				if i == 0 { //对请求行进行分析
					requestLineSplits := bytes.Split(line, []byte(" "))
					if len(requestLineSplits) != 3 {
						return false, true
					}
					for i, cell := range requestLineSplits {
						if i == 2 && !bytes.HasPrefix(cell, []byte("HTTP/")) {
							return false, true
						}
					}
					if bytes.HasPrefix(line, CONNECT) {
						proxyConn.isTunnel = true
					}
				} else {
					if bytes.HasPrefix(line, []byte("Host: ")) {
						hostAndport = line[6:]
						indexMaohao := bytes.Index(hostAndport, []byte(":"))
						if indexMaohao == -1 {
							proxyConn.port = "80"
							proxyConn.host = string([]byte(hostAndport))
						} else {
							proxyConn.host = string([]byte(hostAndport[:indexMaohao]))
							port := string([]byte(hostAndport[indexMaohao+1:]))
							proxyConn.port = port
						}
					}
				}
			}
		}
	}
	//至此，成功解析出了host,port
	serverConn, err := net.Dial("tcp", proxyConn.host+":"+proxyConn.port)
	if err != nil {
		log.Println("连接到", string(hostAndport), "失败,关闭此条代理,err:", err)
		proxyConn.WriteClient(util.Http503, len(util.Http503))
		proxyConn.Close();
		return false, true
	} else {
		proxyConn.ConnServer = serverConn
		log.Println(proxyConn.Info(), "连接到远程服务器成功", )
		if proxyConn.isTunnel {
			proxyConn.WriteClient(util.HttpsEstablish, len(util.HttpsEstablish))
		} else {
			proxyConn.WriteServer(proxyConn.beforeCRLF, len(proxyConn.beforeCRLF))
		}
		return true, false
	}
}

func startWithHttpMethod(src []byte) bool {
	if bytes.HasPrefix(src, CONNECT) || bytes.HasPrefix(src, GET) || bytes.HasPrefix(src, POST) || bytes.HasPrefix(src, PUT) || bytes.HasPrefix(src, DELETE) || bytes.HasPrefix(src, OPTIONS) || bytes.HasPrefix(src, TRACE) || bytes.HasPrefix(src, HEAD) {
		return true
	} else {
		return false
	}
}

func (proxyConn *ProxyConn) Close() {
	proxyConn.lock.Lock()
	defer proxyConn.lock.Unlock()
	if proxyConn.closed {
		return
	} else {
		for ; proxyConn.ConnClient.Close() != nil; {
			log.Println("关闭[", proxyConn.ID, "]失败，100毫秒后重试")
			time.Sleep(time.Microsecond * 100)
		}
		if proxyConn.ConnServer != nil {
			for ; proxyConn.ConnServer.Close() != nil; {
				log.Println("关闭[", proxyConn.ID, "]失败，100毫秒后重试")
				time.Sleep(time.Microsecond * 100)
			}
		}
		proxyConn.closed = true
	}
}

func (proxyConn *ProxyConn) Closed() bool {
	return proxyConn.closed
}

func (proxyConn *ProxyConn) IsTunnel() bool {
	return proxyConn.isTunnel
}

//toString
func (proxyConn *ProxyConn) String() string {
	return fmt.Sprint("代理连接 [ID:", proxyConn.ID, " 客户端地址:", proxyConn.ConnClient.RemoteAddr(), " 服务器地址", proxyConn.host, ":", proxyConn.port, " 隧道:", proxyConn.isTunnel, " 已关闭:", proxyConn.closed, "]")
}

//info
func (proxyConn *ProxyConn) Info() string {
	return fmt.Sprint("[ID:", proxyConn.ID, " 服务器地址 ", proxyConn.host, ":", proxyConn.port, " 隧道:", proxyConn.isTunnel, " 已关闭:", proxyConn.closed, "]")
}

func (proxyConn *ProxyConn) Lock() {
	proxyConn.lock.Lock()
}
func (proxyConn *ProxyConn) Unlock() {
	proxyConn.lock.Unlock()
}

func (proxyConn *ProxyConn) WriteServer(bytes []byte, i int) {
	for writtenNum := 0; writtenNum != i; {
		tempNum, err := proxyConn.ConnServer.Write(bytes[writtenNum:i])
		if err != nil {
			log.Println("写出错 ", err)
			proxyConn.Close()
			break
		}
		writtenNum += tempNum
	}
}

func (proxyConn *ProxyConn) WriteClient(bytes []byte, i int) {
	util.Qufan(&bytes,i)
	for writtenNum := 0; writtenNum != i; {
		tempNum, err := proxyConn.ConnClient.Write(bytes[writtenNum:i])
		if err != nil {
			log.Println("写出错 ", err)
			proxyConn.Close()
			break
		}
		writtenNum += tempNum
	}
}


