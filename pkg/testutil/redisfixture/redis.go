// Package redisfixture provides a deterministic RESP fixture for cache tests.
// It exercises the production Redis client over TCP; it is not a Redis daemon
// and only models the single-key cache reservation script, not arbitrary Lua.
package redisfixture

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type entry struct {
	value string
	ttl   int
}

type Server struct {
	mu       sync.Mutex
	listener net.Listener
	conns    []net.Conn
	values   map[string]entry
	calls    map[string]int
	fail     map[string]bool // value true applies command before returning an error
}

func New(t *testing.T) *Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{listener: listener, values: map[string]entry{}, calls: map[string]int{}, fail: map[string]bool{}}
	var accepted sync.WaitGroup
	accepted.Add(1)
	go func() {
		defer accepted.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, conn)
			s.mu.Unlock()
			go s.serve(conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		accepted.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, conn := range s.conns {
			_ = conn.Close()
		}
	})
	return s
}

func (s *Server) Addr() string { return s.listener.Addr().String() }
func (s *Server) Set(key, value string, ttl int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = entry{value, ttl}
}
func (s *Server) Value(key string) (string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.values[key]
	return value.value, value.ttl
}

// Expire deterministically models eviction or expiry, without wall-clock sleeps.
func (s *Server) Expire(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
}
func (s *Server) Fail(command string, after bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail[strings.ToUpper(command)] = after
}
func (s *Server) Recover(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fail, strings.ToUpper(command))
}
func (s *Server) Calls(command string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[strings.ToUpper(command)]
}

func (s *Server) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)
	for {
		args, err := readCommand(reader)
		if err != nil {
			return
		}
		s.mu.Lock()
		command := strings.ToUpper(args[0])
		s.calls[command]++
		after, fail := s.fail[command]
		response := "-ERR injected cache failure\r\n"
		if !fail || after {
			response = s.execute(command, args[1:])
			if fail {
				response = "-ERR injected lost acknowledgement\r\n"
			}
		}
		s.mu.Unlock()
		if _, err := io.WriteString(conn, response); err != nil {
			return
		}
	}
}
func readCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "*")))
	if err != nil || n < 1 {
		return nil, fmt.Errorf("invalid RESP array")
	}
	args := make([]string, n)
	for i := range args {
		header, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		size, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(header, "$")))
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid RESP bulk string")
		}
		data := make([]byte, size+2)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, err
		}
		args[i] = string(data[:size])
	}
	return args, nil
}
func bulk(value string) string { return fmt.Sprintf("$%d\r\n%s\r\n", len(value), value) }
func (s *Server) execute(command string, args []string) string {
	switch command {
	case "HELLO":
		return "-ERR unknown command 'hello'\r\n"
	case "CLIENT", "PING":
		return "+OK\r\n"
	case "GET":
		if value, ok := s.values[args[0]]; ok {
			return bulk(value.value)
		}
		return "$-1\r\n"
	case "SET":
		ttl, nx := 0, false
		for i := 2; i < len(args); i++ {
			switch strings.ToUpper(args[i]) {
			case "NX":
				nx = true
			case "EX":
				i++
				ttl, _ = strconv.Atoi(args[i])
			}
		}
		if _, ok := s.values[args[0]]; nx && ok {
			return "$-1\r\n"
		}
		s.values[args[0]] = entry{args[1], ttl}
		return "+OK\r\n"
	case "DEL":
		// Reject multi-key operations, like a cluster with unrelated slots.
		if len(args) != 1 {
			return "-CROSSSLOT keys do not hash to the same slot\r\n"
		}
		_, found := s.values[args[0]]
		delete(s.values, args[0])
		if found {
			return ":1\r\n"
		}
		return ":0\r\n"
	case "TTL":
		if value, ok := s.values[args[0]]; ok {
			return fmt.Sprintf(":%d\r\n", value.ttl)
		}
		return ":-2\r\n"
	case "EVAL":
		if len(args) != 6 || args[1] != "1" {
			return "-ERR expected single-key compare-and-fill\r\n"
		}
		key, marker, value := args[2], args[3], args[4]
		ttl, err := strconv.Atoi(args[5])
		if err != nil {
			return "-ERR invalid ttl\r\n"
		}
		if current, ok := s.values[key]; !ok || current.value != marker {
			return ":0\r\n"
		}
		if value == "" || ttl <= 0 {
			delete(s.values, key)
		} else {
			s.values[key] = entry{value, ttl}
		}
		return ":1\r\n"
	default:
		return "-ERR unsupported fixture command\r\n"
	}
}
