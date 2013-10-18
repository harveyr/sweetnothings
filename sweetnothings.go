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
	"sync"
	"strings"
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

func bold(s string) string {
	return wrapColor(s, "bold")
}

func statusLn(s string) {
	msg := fmt.Sprintf("[%s]", s)
	fmt.Println(wrapColor(msg, "blue"))
}

func logColor(s string, color string) {
	log.Println(wrapColor(s, color))
}

/**
 * SweetNothing
 */
type SweetNothing struct {
	ID        string
	Addr      string
	Body      string
	Timestamp time.Time
}

func (s SweetNothing) String() string {
	return fmt.Sprintf("%s %s", bold(s.Addr), s.Body)
}

/**
 * Peers
 */
type Peers struct {
	channels map[string]chan<- SweetNothing
	mu sync.RWMutex
}

func (p *Peers) Add(addr string) <-chan SweetNothing {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.channels[addr]; ok {
		return nil
	}
	c := make(chan SweetNothing)
	p.channels[addr] = c
	return c
}

func (p *Peers) Remove(addr string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.channels, addr)
}

func (p *Peers) List() []chan<- SweetNothing {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	l := make([]chan<- SweetNothing, 0, len(p.channels))
	for _, ch := range p.channels {
		l = append(l, ch)
	}
	return l
}

/**
 * LocalInfo
 */
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
var peers = &Peers{channels: make(map[string]chan<- SweetNothing)}

var seenIds = struct {
	m map[string]bool
	sync.Mutex
}{m: make(map[string]bool)}

func SeenId(id string) bool {
	seenIds.Lock()
	ok := seenIds.m[id]
	seenIds.m[id] = true
	seenIds.Unlock()
	return ok
}

var nicknames = struct {
	m map[string]string
	sync.Mutex
}{m: make(map[string]string)}

func nick(addr string) (n string) {
	if addr == localInfo.Addr() {
		n = "you"
	} else if n_, ok := nicknames.m[addr]; ok {
		n = n_
	} else {
		n = addr
	}
	return fmt.Sprintf("[%s]", n)
}

func setNick(addr string, nick string) {
	nicknames.m[addr] = nick
	statusLn(fmt.Sprintf("%s nicknamed %s", addr, nick))
}

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

func serveIncoming(c net.Conn) {
	var whisper SweetNothing
	for {
		dec := json.NewDecoder(c)
		err := dec.Decode(&whisper)
		if err != nil {
			break
		}

		if SeenId(whisper.ID) {
			continue
		}
		fmt.Printf("%s %s\n", bold(nick(whisper.Addr)), whisper.Body)
		broadcast(whisper)
		go dial(whisper.Addr)
	}
	c.Close()
	statusLn(fmt.Sprintf("Closed connection to %s", c.RemoteAddr()))
}

func broadcast(whisper SweetNothing) {
	for _, ch := range peers.List() {
		select {
		case ch <- whisper:
		default:
			// Message dropped
		}
	}
}

func dial(addr string) {
	if addr == localInfo.Addr() {
		return
	}

	ch := peers.Add(addr)
	if ch == nil {
		return
	}
	defer peers.Remove(addr)

	statusLn(fmt.Sprintf("Dialing %s", addr))

	c, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("[Error dialing %s]\n", addr)
		return
	}

	statusLn(fmt.Sprintf("Connected to %s", addr))

	defer func() {
		c.Close()
		statusLn(fmt.Sprintf("Closed connection to %s", c.RemoteAddr()))
	}()

	enc := json.NewEncoder(c)
	for s := range ch {
		err := enc.Encode(s)
		if err != nil {
			log.Printf("[Error encoding message] %v\n", err)
			return
		}
	}
}

func handleCommand(c string) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(c)), " ")
	switch parts[0] {
		case "/dial":
			go dial(parts[1])
		case "/setnick":
			if len(parts) == 3 {
				setNick(parts[1], parts[2])
			}
	}
}

func startInputScanner() {
	s := bufio.NewScanner(os.Stdin)
	for {
		s.Scan()
		text := s.Text()
		if len(text) == 0 {
			continue
		}
		if strings.HasPrefix(text, "/") {
			handleCommand(text)
		} else {
			whisper := SweetNothing{uniqueId(), localInfo.Addr(), s.Text(), time.Now().UTC()}
			broadcast(whisper)
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

	fmt.Println(bold("--- Sweet Nothings ---"))

	localInfo.ListenPort = port

	go startInputScanner()

	listenAddr := fmt.Sprintf("0.0.0.0:%s", localInfo.ListenPort)
	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	statusLn(fmt.Sprintf("Local address: %s", localInfo.Addr()))
	statusLn(fmt.Sprintf("Listening on %s", l.Addr()))

	for {
		con, err := l.Accept()
		if err != nil {
			log.Println("<", fmt.Sprintf("Error on accept: %s", err))
		}
		go serveIncoming(con)
	}
}
