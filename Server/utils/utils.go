package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"redis"
	"strconv"
)

type UserInfo struct {
	Id      string `json:"id"`
	Pwd     string `json:"pwd"`
	Name    string `json:"name"`
	Profile string `json:"imageUrl"`
}

type Dataflow struct {
	Conn net.Conn
	Type string
	Buf  [8192]byte
	Cnt  int
}

type Cilent struct {
	ID   string
	NAME string
	Addr string
	CONN net.Conn
	MSG  chan string
}

func (this *Dataflow) ReadPkg() bool {
	n, err := this.Conn.Read(this.Buf[:])
	if err != nil {
		if err == io.EOF {
			fmt.Println("read Pkg EOF, err=", err)
		}
		fmt.Println("read Pkg header failed, err=", err)
		return false
	}
	this.Cnt = n
	this.Type = string(this.Buf[0])

	return true
}

type StatMsg struct {
	Type  string `json:"type"`
	State string `json:"state"`
}

func Login(dataflow Dataflow, DBconn redis.Conn, cli *Cilent) bool {
	var userInfo UserInfo
	err := json.Unmarshal(dataflow.Buf[1:dataflow.Cnt], &userInfo)
	if err != nil {
		fmt.Println("json.Unmarshal failed in function Login(), err=", err)
		return false
	}

	fmt.Println(userInfo.Id, "    ", userInfo.Name)
	var s StatMsg

	var seq int = 0
	s.Type = strconv.Itoa(seq) + "#login"

	flag := false
	pwd, err := redis.String(DBconn.Do("HGET", userInfo.Id, "PWD"))

	if err != nil {
		fmt.Println("DB Get error, err = ", err)
		flag = false
	}
	if pwd == userInfo.Pwd && userInfo.Pwd != "" {
		flag = true
	}
	fmt.Println(userInfo.Pwd)
	fmt.Println(pwd)
	if flag {
		s.State = "success"
		data, _ := json.Marshal(s)
		dataflow.Conn.Write(data)
		cli.ID = userInfo.Id
		cli.NAME, _ = redis.String(DBconn.Do("HGET", userInfo.Id, "NAME"))
		return true
	} else {
		s.State = "fail"
		data, _ := json.Marshal(s)
		dataflow.Conn.Write(data)
		return false
	}

}

func Signup(dataflow Dataflow, DBconn redis.Conn) bool {
	var userInfo UserInfo
	err := json.Unmarshal(dataflow.Buf[1:dataflow.Cnt], &userInfo)
	if err != nil {
		fmt.Println("json.Unmarshal failed in function Signup(), err=", err)
		return false
	}

	fmt.Println(userInfo.Id, "    ", userInfo.Name)

	var s StatMsg

	var seq int = -100
	s.Type = strconv.Itoa(seq) + "#signup"
	flag := true
	res, err := redis.Int(DBconn.Do("HEXISTS", userInfo.Id, "PWD"))
	if err != nil {
		fmt.Println("DB Get error, err = ", err)
		flag = false
	}
	if res == 1 {
		flag = false
	}

	if !flag {
		s.State = "fail"
		data, _ := json.Marshal(s)
		dataflow.Conn.Write(data)
		return false
	} else {
		DBconn.Do("HSET", userInfo.Id, "ID", userInfo.Id, "PWD", userInfo.Pwd, "NAME", userInfo.Name)
		if err != nil {
			fmt.Println("DB Get error, err = ", err)
			return false
		}
		s.State = "success"
		data, _ := json.Marshal(s)
		dataflow.Conn.Write(data)
		return false
	}
}
