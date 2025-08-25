package pkg

import (
	"io"
	"log"
	"os"
	"sync"
)

type sessionWriter struct{}

var logFile *os.File

var (
	sessionMu   sync.Mutex
	sessionLogs []string
	subscribers []chan string
)

func Subscribe() <-chan string {
	ch := make(chan string, 100)
	sessionMu.Lock()
	subscribers = append(subscribers, ch)
	sessionMu.Unlock()
	return ch
}
func GetSessionLogs() []string {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	copied := make([]string, len(sessionLogs))
	copy(copied, sessionLogs)
	return copied
}

func SetLogger() {
	var err error
	logFile, err = os.OpenFile("latest.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("[e] Could not open log file:", err)
	}

	multi := io.MultiWriter(os.Stdout, logFile, sessionWriter{})
	log.SetOutput(multi)
	log.SetFlags(0)
}

func (sessionWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	sessionMu.Lock()
	sessionLogs = append(sessionLogs, msg)
	for _, sub := range subscribers {
		select {
		case sub <- msg:
		default:
		}
	}
	sessionMu.Unlock()
	return len(p), nil
}
