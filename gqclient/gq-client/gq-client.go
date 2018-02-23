package main

import (
	"github.com/cbeuw/GoQuiet/gqclient"
	"io"
	"log"
	"net"
	"os"
	"time"
)

// ss refers to the ss-client, remote refers to the proxy server

type pipe interface {
	remoteToSS()
	ssToRemote()
	closePipe()
}

type pair struct {
	ss     net.Conn
	remote net.Conn
}

func (p *pair) closePipe() {
	p.ss.Close()
	p.remote.Close()
}

func (p *pair) remoteToSS() {
	for {
		data, err := gqclient.ReadTillDrain(p.remote)
		if err != nil {
			p.closePipe()
			return
		}
		data = gqclient.PeelRecordLayer(data)
		_, err = p.ss.Write(data)
		if err != nil {
			p.closePipe()
			return
		}
	}
}

func (p *pair) ssToRemote() {
	for {
		buf := make([]byte, 1500)
		i, err := io.ReadAtLeast(p.ss, buf, 1)
		data := buf[:i]
		data = gqclient.AddRecordLayer(data, []byte{0x17}, []byte{0x03, 0x03})
		_, err = p.remote.Write(data)
		if err != nil {
			p.closePipe()
			return
		}
	}
}

func initSequence(ssConn net.Conn, sta *gqclient.State) {
	log.Println("New conn from SS")
	var err error
	var remoteConn net.Conn
	for trial := 0; err == nil && trial < 3; trial++ {
		remoteConn, err = net.Dial("tcp", sta.SS_REMOTE_HOST+":"+sta.SS_REMOTE_PORT)
	}
	if remoteConn == nil {
		log.Println("Failed to connect to the proxy server")
		return
	}
	clientHello := gqclient.ComposeInitHandshake(sta)
	_, err = remoteConn.Write(clientHello)
	if err != nil {
		log.Println(err)
		return
	}
	discard := make([]byte, 500)
	_, err = remoteConn.Read(discard)
	if err != nil {
		log.Println(err)
		return
	}
	reply := gqclient.ComposeReply()
	_, err = remoteConn.Write(reply)
	if err != nil {
		log.Println(err)
		return
	}
	p := pair{
		ssConn,
		remoteConn,
	}
	go p.remoteToSS()
	go p.ssToRemote()

}

func main() {
	opaque := gqclient.BtoInt(gqclient.CryptoRandBytes(32))
	sta := &gqclient.State{
		SS_LOCAL_HOST: os.Getenv("SS_LOCAL_HOST"),
		// IP address of this plugin listening. Should be 127.0.0.1
		SS_LOCAL_PORT: os.Getenv("SS_LOCAL_PORT"),
		// The remote port set in SS, default to 8388
		SS_REMOTE_HOST: os.Getenv("SS_REMOTE_HOST"),
		// IP address of the proxy server with the server side of this plugin running
		SS_REMOTE_PORT: os.Getenv("SS_REMOTE_PORT"),
		// Port number of the proxy server with the server side of this plugin running
		// should be 443
		Now:    time.Now,
		Opaque: opaque,
	}
	configPath := os.Getenv("SS_PLUGIN_OPTIONS")
	err := sta.ParseConfig(configPath)
	if err != nil {
		log.Fatal(err)
	}
	sta.SetAESKey()
	listener, err := net.Listen("tcp", sta.SS_LOCAL_HOST+":"+sta.SS_LOCAL_PORT)
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go initSequence(conn, sta)
	}

}
