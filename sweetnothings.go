package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

var colors = map[string]string{
	"header": "\033[95m",
	"blue":   "\033[94m",
	"green":  "\033[92m",
	"yellow": "\033[93m",
	"red":    "\033[91m",
	"bold":   "\033[1m",
	"end":    "\033[0m",
}

func wrapColor(s string, color string) string {
	return fmt.Sprintf("%s%s%s", colors[color], s, colors["end"])
}

func logColor(s string, color string) {
	log.Println(wrapColor(s, color))
}

type SweetNothing struct {
	ID        string
	Addr      string
	Body      string
	Timestamp time.Time
}

func (s SweetNothing) String() string {
	return fmt.Sprintf("%s %s", wrapColor(s.Addr, "bold"), s.Body)
}

type LocalInfo struct {
	ip         string
	ListenPort string
}

func (i *LocalInfo) IP() string {
	if len(i.ip) == 0 {
		hostname, err := os.Hostname()
		if err != nil {
			log.Fatal("Unable to get hostname:", err)
		}
		localIps, err := net.LookupHost(hostname)
		if err != nil {
			log.Fatal("Unable to determine local ip:", err)
		}
		i.ip = localIps[0]
	}
	return i.ip
}

func (i LocalInfo) Addr() string {
	return fmt.Sprintf("%s:%s", i.IP(), i.ListenPort)
}

var localInfo = new(LocalInfo)
var recipientRegistry = make([]string, 1)

func uniqueId() string {
	now := time.Now()
	return fmt.Sprintf(
		"%d%d%d%d%d%d%d",
		now.Year(),
		now.Month(),
		now.Day(),
		now.Hour(),
		now.Minute(),
		now.Second(),
		now.Nanosecond())
}

func startListener(c chan SweetNothing) {
	listenAddr := fmt.Sprintf("0.0.0.0:%s", localInfo.ListenPort)
	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error in listen(): %s", err))
	}
	log.Println("Listening on", l.Addr())

	for {
		con, err := l.Accept()
		if err != nil {
			log.Fatal(fmt.Sprintf("Error on accept: %s", err))
		}
		var received SweetNothing
		dec := json.NewDecoder(con)
		dec.Decode(&received)
		c <- received
	}
}

func sendMessageTo(recipientAddr string, s SweetNothing) {
	c, err := net.Dial("tcp", recipientAddr)
	if err != nil {
		log.Fatalf("sendMessage error: %s", err)
	}
	enc := json.NewEncoder(c)
	enc.Encode(s)
	c.Close()
}

func sendMessage(s SweetNothing) {
	for _, addr := range recipientRegistry {
		go sendMessageTo(addr, s)
	}
}

func startScanner() {
	s := bufio.NewScanner(os.Stdin)
	for {
		s.Scan()
		text := s.Text()
		if len(text) > 0 {
			whisper := SweetNothing{uniqueId(), localInfo.Addr(), s.Text(), time.Now().UTC()}
			sendMessage(whisper)
		}
	}
	if err := s.Err(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	var port string

	flag.StringVar(&port, "p", "", "Listen port")
	flag.Parse()

	if len(port) < 4 {
		log.Fatalf("Invalid listen port (%s)", port)
	}

	localInfo.ListenPort = port
	recipientRegistry[0] = localInfo.Addr()

	go startScanner()

	listenChan := make(chan SweetNothing)
	go startListener(listenChan)
	for msg := range listenChan {
		fmt.Println(msg)
	}
}
