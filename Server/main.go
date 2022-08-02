package main

import (
	"chat/Server/dispatcher"
	"chat/Server/utils"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"redis"
	"strings"
	"sync"
	"time"
)

var PIPE = make(chan string)
var OnlineUsers map[string]utils.Cilent

var MapLock sync.Mutex

func getCliID(msg string) string {
	//buf = "[" + cli.Addr + "][" + cli.ID + "][" + cli.NAME + "]" + msg
	var lo int
	for i := 0; i < len(msg)-1; i++ {
		if msg[i] == ']' && lo != 0 {
			return msg[lo:i]
		}
		if msg[i] == ']' && msg[i+1] == '[' {
			lo = i + 2
			continue
		}
	}
	return ""
}

func BatchSender() {
	MapLock.Lock()
	OnlineUsers = make(map[string]utils.Cilent)
	MapLock.Unlock()
	for {
		msg := <-PIPE
		for _, cli := range OnlineUsers {
			cli.MSG <- msg
		}
		if msg[0] == '#' {
			var s utils.StatMsg
			s.State = msg[1:]
			cliID := getCliID(msg)
			clix := OnlineUsers[cliID]
			for _, cli := range OnlineUsers {
				msg := "#" + dispatcher.MSG(cli, "")
				clix.MSG <- msg
			}
		}
	}
}

func main() {

	///////////////////////////////////////////////////////////////////
	RedisPool := &redis.Pool{
		MaxIdle:     30,  //最大空闲链接数量
		MaxActive:   0,   //表示和数据库最大链接数，0表示，并发不限制数量
		IdleTimeout: 100, //最大空闲时间，用完链接后100秒后就回收到链接池
		Dial: func() (redis.Conn, error) { //初始化链接池的代码
			return redis.Dial("tcp", "0.0.0.0:6379")
		},
	}
	fmt.Println(RedisPool)
	/////////////////////////////////////////////////////////////////////

	listener, err := net.Listen("tcp", "0.0.0.0:8666")
	if err != nil {
		fmt.Println("Server Listen failed err= ", err)
		return
	}
	defer listener.Close()
	fmt.Println("Server listen at PORT:8666")

	/////////////////////////////////////////////////////////////////////
	go Start()
	//go Start1()
	go dispatcher.FileRECV(PIPE, &OnlineUsers)
	go dispatcher.FileSEND(PIPE, &OnlineUsers)
	go BatchSender()
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("User connected fail err=", err)
			continue
		}

		DBconn := RedisPool.Get() //  *redis.activeConn
		go dispatcher.Handle(conn, DBconn, PIPE, MapLock, OnlineUsers)
	}

}
func Start() {
	fmt.Println("init...")
	http.HandleFunc("/", doExecute)
	http.ListenAndServe(":8080", nil)
}

var realPath string = "Server/"

func doExecute(response http.ResponseWriter, request *http.Request) {
	requestUrl := request.URL.String()
	filePath := requestUrl[:]
	fmt.Println(filePath)
	strs := strings.Split(filePath, "/")
	fmt.Println(strs[2])

	if strs[2] == "UserProfile" {
		path.Base(filePath)
		files, _ := ioutil.ReadDir("Server/UserProfile/")
		var fd *os.File
		for _, file := range files {
			name := strings.TrimSuffix(file.Name(), path.Ext(file.Name()))
			if name == path.Base(filePath) {
				fd, _ = os.Open("Server/UserProfile/" + file.Name())
			} else {
				continue
			}
		}
		defer fd.Close()
		bs, _ := ioutil.ReadAll(fd)
		response.Write(bs)
	} else {
		time.Sleep(time.Millisecond * 1000)
		var fd *os.File
		fmt.Println(filePath)
		fd, _ = os.Open("Server/UserSrc/" + path.Base(filePath))
		bs, _ := ioutil.ReadAll(fd)
		response.Write(bs)
		defer fd.Close()
	}
}
