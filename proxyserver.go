package main

import (
	"github.com/arloor/proxyserver/connection"
	"github.com/arloor/proxyserver/util"
	"log"
	"net"
)

var CONNECT = []byte("CONNECT")
var GET = []byte("GET")
var POST = []byte("POST")
var PUT = []byte("PUT")
var DELETE = []byte("DELETE")
var TRACE = []byte("TRACE")
var OPTIONS = []byte("OPTIONS")
var HEAD = []byte("HEAD")

const proxyServerAddr = "127.0.0.1:8080"

func main() {
	listener, err := net.Listen("tcp", proxyServerAddr)
	if err != nil {
		log.Fatal("在", proxyServerAddr, "监听失败，程序退出")
		return
	}
	log.Println("在", proxyServerAddr, "监听成功")

	for ; ; {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Println("accept客户端连接失败，err: ", err)
			continue
		}
		log.Println("accept客户端连接成功,客户端地址:", clientConn.RemoteAddr())
		proxyConn := connection.NewProxyConn(clientConn)
		go serveProxyConn(proxyConn)
	}
}

func serveProxyConn(proxyConn *connection.ProxyConn) {
	log.Println("service ", proxyConn)
	serveClientConn(proxyConn)
}

func serveClientConn(proxyConn *connection.ProxyConn) {
	clientConn := proxyConn.ConnClient
	for ; ; {
		var info = proxyConn.Info()
		buf := make([]byte, 1024)
		numRead, err := clientConn.Read(buf)
		util.Qufan(&buf,numRead)
		if err != nil {
			log.Println("从", info, "的本地连接读出错,关闭整条连接，err:", err)
			proxyConn.Close();
			return
		}
		log.Println("从", info, "本地读到：", numRead, "字节")

		//已连接
		if proxyConn.ConnServer != nil && !proxyConn.Closed() {
			if proxyConn.IsTunnel() { //是connect的隧道
				proxyConn.WriteServer(buf, numRead)
			} else { //不是隧道
				//todo 改变请求头
				proxyConn.WriteServer(buf, numRead)
			}
		} else if proxyConn.Closed() {
			return
		} else if proxyConn.ConnServer == nil {
			connected, invalid := proxyConn.AppendContentAndTryParseAndConnect(buf[:numRead])
			if invalid {
				proxyConn.Close()
				break
			}
			if connected {
				go serveServerConn(proxyConn)
			}
		}
	}
}

func serveServerConn(proxyConn *connection.ProxyConn) {
	var buf=make([]byte,2048)
	serverConn:=proxyConn.ConnServer
	for ; ;  {
		numRead,err:=serverConn.Read(buf)
		if err!=nil{
			proxyConn.Close()
			return
		}else{
			proxyConn.WriteClient(buf,numRead)
		}
	}
}
