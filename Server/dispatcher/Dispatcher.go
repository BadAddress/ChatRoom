package dispatcher

import (
	"chat/Server/utils"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"redis"
	"strconv"
	"strings"
	"sync"
	"time"
)

func PointToPointSend(cli utils.Cilent, closeNotify chan int) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("one P2P Send thread is broken")
			return
		}
	}()
	var seq int = 10
	for {
		select {
		case MSG := <-cli.MSG:
			var s utils.StatMsg
			if MSG[0] == '#' {
				time.Sleep(time.Millisecond * 100)
				s.Type = strconv.Itoa(seq) + "#userStat"
				s.State = MSG[1:]
				//return
			} else if MSG[0] == '@' {
				s.Type = strconv.Itoa(seq) + "#fileUpload"
				s.State = MSG[1:]
			} else if MSG[0] == '!' {
				s.Type = strconv.Itoa(seq) + "#tfComplete"
				s.State = MSG[1:]
			} else if MSG[0] == '*' {
				s.Type = strconv.Itoa(seq) + "#exit"
				s.State = MSG[1:]
			} else {
				s.Type = strconv.Itoa(seq) + "#normalMsg"
				s.State = MSG[:]
			}
			data, _ := json.Marshal(s)
			fmt.Println(string([]byte(data)))
			cli.CONN.Write(data)
			seq++
			//fmt.Println(string(data))
		case <-closeNotify:
			return
		}
	}
}

func MSG(cli utils.Cilent, msg string) (buf string) {
	buf = "[" + cli.Addr + "][" + cli.ID + "][" + cli.NAME + "]" + msg
	return buf
}

func Handle(Conn net.Conn, DBconn redis.Conn, PIPE chan string, MapLock sync.Mutex, OnlineUsers map[string]utils.Cilent) {
	fmt.Printf("%T\n%T\n", Conn, DBconn)
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("one thread is broken")
			return
		}
	}()
	defer Conn.Close()
	var cli utils.Cilent
	var closeNotify chan int = make(chan int, 1)

	for {
		var dataflow utils.Dataflow
		dataflow.Conn = Conn
		if ok := dataflow.ReadPkg(); !ok {
			PIPE <- ("*" + MSG(cli, ""))
			MapLock.Lock()
			delete(OnlineUsers, cli.ID)
			MapLock.Unlock()
			closeNotify <- 9
			break
		}
		// NORMAL 0    login 1    signup 2    exit 99  ...
		fmt.Println(string(dataflow.Buf[0]))
		switch dataflow.Type {
		case "0":
			fmt.Println(string(dataflow.Buf[1:dataflow.Cnt]))
			PIPE <- MSG(cli, "$"+string(dataflow.Buf[1:dataflow.Cnt]))
			continue
		case "1":
			if ok := utils.Login(dataflow, DBconn, &cli); ok {
				cliAddr := Conn.RemoteAddr().String()
				cli.Addr = cliAddr
				cli.MSG = make(chan string)
				cli.CONN = Conn
				MapLock.Lock()
				OnlineUsers[cli.ID] = cli
				MapLock.Unlock()
				time.Sleep(time.Second * 1)
				PIPE <- ("#" + MSG(cli, ""))
				go PointToPointSend(cli, closeNotify)
			}
			continue
		case "2":
			utils.Signup(dataflow, DBconn)
			continue
		case "9":
			PIPE <- ("*" + MSG(cli, ""))
			MapLock.Lock()
			delete(OnlineUsers, cli.ID)
			MapLock.Unlock()
			fmt.Println("USER EXIT")
			closeNotify <- 9
			return
		default:
			continue
		}
	}
}

type Header struct {
	UID        string
	CmdType    string
	FileSurfix string
}

type FD struct {
	Name string
	Size string
}

func FileRECV(PIPE chan string, OnlineUsers *map[string]utils.Cilent) {
	listener, err := net.Listen("tcp", "0.0.0.0:12345")
	if err != nil {
		fmt.Println("FileRECV Server Listen failed err= ", err)
		return
	}
	defer listener.Close()
	fmt.Println("FileRECV Server listen at PORT:12345")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("User connected fail err=", err)
			continue
		}
		go func(conn net.Conn, PIPE chan string, OnlineUsers *map[string]utils.Cilent) {
			var msg Header
			buf := make([]byte, 1024)
			cnt, _ := conn.Read(buf)
			err := json.Unmarshal(buf[:cnt], &msg)
			if err != nil {
				fmt.Println("ERR", err)
			}
			fmt.Println(msg.FileSurfix)
			conn.Write([]byte("ok"))

			cli := (*OnlineUsers)[string(msg.UID)]

			var fdInfo FD
			cnt, _ = conn.Read(buf)
			json.Unmarshal(buf[:cnt], &fdInfo)
			conn.Write([]byte("ok"))
			var t string
			var filename string
			if msg.CmdType == "profile" {
				filename = "Server/UserProfile/" + msg.UID + msg.FileSurfix
			} else {
				t = strconv.FormatInt(time.Now().UnixNano(), 10)

				filename = "Server/UserSrc/" + msg.UID + "pushed" + t + msg.FileSurfix
				PIPE <- ("@" + MSG(cli, "$"+msg.UID+"pushed"+t+msg.FileSurfix+"$"+fdInfo.Size))
			}

			buf = make([]byte, 4096)

			fd, err := os.Create(filename)
			defer fd.Close()
			if err != nil {
				fmt.Println("os.Create() err,", err)
				return
			}
			for {
				n, err := conn.Read(buf)
				if err != nil {
					if err == io.EOF {
						fmt.Println(filename, " received")
					} else {
						fmt.Println("conn read err,", err)
					}
					PIPE <- ("!" + MSG(cli, "$"+msg.UID+"pushed"+t+msg.FileSurfix+"$"+fdInfo.Size))
					return
				}
				fd.Write(buf[:n])
			}
		}(conn, PIPE, OnlineUsers)
	}
}

func FileSEND(PIPE chan string, OnlineUsers *map[string]utils.Cilent) {

	listener, err := net.Listen("tcp", "0.0.0.0:12344")
	if err != nil {
		fmt.Println("FileSEND Server Listen failed err= ", err)
		return
	}
	defer listener.Close()
	fmt.Println("FileSEND Server listen at PORT:12344")

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("User connected fail err=", err)
			continue
		}

		go func(conn net.Conn) {
			defer conn.Close()
			defer func() {
				if err := recover(); err != nil {
					fmt.Println("one Send thread is broken")
					return
				}
			}()
			var msg Header
			buf := make([]byte, 1024)
			cnt, _ := conn.Read(buf)
			err := json.Unmarshal(buf[:cnt], &msg)
			if err != nil {
				fmt.Println("ERR", err)
			}

			var root string
			var fd *os.File
			var fileName string

			if msg.CmdType == "getProfile" {
				root = "Server/UserProfile"
				files, err := ioutil.ReadDir(root)
				if err != nil {
					log.Fatal(err)
				}
				//fmt.Println(files)

				for _, file := range files {
					name := strings.TrimSuffix(file.Name(), path.Ext(file.Name()))
					if name == msg.UID {
						fd, err = os.OpenFile(root+"/"+file.Name(), os.O_RDONLY, 0666)
						if err != nil {
							fmt.Println("OPEN FILE ERROR")
							break
						}
						fileName = file.Name()
					}
				}
			} else {
				root = "Server/UserSrc"
				fileName = msg.CmdType
				fd, err = os.OpenFile(root+"/"+fileName, os.O_RDONLY, 0666)
				if err != nil {
					fmt.Println("OPEN FILE ERROR")
					return
				}
			}

			defer fd.Close()

			var fdInfo FD
			fi, _ := fd.Stat()
			fdInfo.Size = strconv.FormatInt(fi.Size(), 10)
			fdInfo.Name = fileName
			m, _ := json.Marshal(fdInfo)

			conn.Write(m)
			n, err := conn.Read(buf)
			if "ok" != string(buf[:n]) {
				return
			}
			buf = make([]byte, 4096)
			for {
				n, err := fd.Read(buf)
				if err != nil {
					if err == io.EOF {
						fmt.Println("transfer to cli completed")

					} else {
						fmt.Println("file read err,", err)
					}
					return
				}
				conn.Write(buf[:n])
			}
		}(conn)
	}
}
