package server

import (
	"bufio"
	"errors"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
)

var (
	activeServer    *Server
	serverMu        sync.Mutex
	ErrServerExists = errors.New("a server is already running")
)

type Server struct {
	cmd       *exec.Cmd
	stdin     chan string
	done      chan struct{}
	mu        sync.Mutex
	isRunning bool
}

func Start() error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if activeServer != nil {
		if activeServer.GetStatus() {
			return ErrServerExists
		}
	}

	s := &Server{
		stdin: make(chan string, 100),
		done:  make(chan struct{}),
	}

	if err := s.startInternal(); err != nil {
		return err
	}

	activeServer = s
	return nil
}

func Stop() error {
	serverMu.Lock()
	s := activeServer
	serverMu.Unlock()

	if s == nil || !s.GetStatus() {
		return errors.New("server is not running")
	}

	return s.RunCommand("stop")
}

func Kill() error {
	serverMu.Lock()
	s := activeServer
	serverMu.Unlock()

	if s == nil || !s.GetStatus() {
		return errors.New("server is not running")
	}

	return s.Kill()
}

func RunCommand(cmd string) error {
	serverMu.Lock()
	s := activeServer
	serverMu.Unlock()

	if s == nil || !s.GetStatus() {
		return errors.New("server is not running")
	}

	return s.RunCommand(cmd)
}

func GetStatus() bool {
	serverMu.Lock()
	s := activeServer
	serverMu.Unlock()

	if s == nil {
		return false
	}
	return s.GetStatus()
}


func (s *Server) startInternal() error {
	s.cmd = exec.Command("java",
		"-Xms2G",
		"-Xmx4G",
		"-XX:+UseG1GC",
		"-XX:+ParallelRefProcEnabled",
		"-XX:+UnlockExperimentalVMOptions",
		"-XX:+DisableExplicitGC",
		"-XX:+AlwaysPreTouch",
		"-XX:G1HeapWastePercent=5",
		"-XX:G1MixedGCCountTarget=4",
		"-XX:MaxGCPauseMillis=50",
		"-XX:G1NewSizePercent=30",
		"-XX:G1MaxNewSizePercent=40",
		"-XX:G1HeapRegionSize=8M",
		"-XX:+PerfDisableSharedMem",
		"-XX:MaxDirectMemorySize=1G",
		"-jar", "server.jar",
		"nogui",
	)
	s.cmd.Dir = "minecraft"

	stdoutPipe, _ := s.cmd.StdoutPipe()
	stderrPipe, _ := s.cmd.StderrPipe()
	stdinPipe, _ := s.cmd.StdinPipe()

	if err := s.cmd.Start(); err != nil {
		log.Println("[e] Failed to start server process:", err)
		return err
	}

	s.mu.Lock()
	s.isRunning = true
	s.mu.Unlock()

	go s.pipeAndLog(stdoutPipe, "[g] ")
	go s.pipeAndLog(stderrPipe, "[g] ")

	go func() {
		for cmd := range s.stdin {
			_, err := io.WriteString(stdinPipe, cmd+"\n")
			if err != nil {
				log.Println("[e] Failed to write to stdin:", err)
				return
			}
		}
	}()

	go func() {
		err := s.cmd.Wait()
		if err != nil {
			log.Println("[e] Server exited with error:", err)
		}

		s.mu.Lock()
		s.isRunning = false
		close(s.done)
		s.mu.Unlock()

		serverMu.Lock()
		if activeServer == s {
			activeServer = nil
		}
		serverMu.Unlock()

		log.Println("[i] Server process cleanup finished.")
	}()

	return nil
}

func (s *Server) Kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return errors.New("server is not running")
	}

	return s.cmd.Process.Kill()
}

func (s *Server) RunCommand(cmd string) error {
	if !s.GetStatus() {
		return errors.New("server is not running")
	}

	select {
	case s.stdin <- cmd:
		return nil
	default:
		return errors.New("command queue full")
	}
}

func (s *Server) GetStatus() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}

func (s *Server) pipeAndLog(pipeReader io.ReadCloser, prefix string) {
	defer pipeReader.Close()
	scanner := bufio.NewScanner(pipeReader)
	for scanner.Scan() {
		text := scanner.Text()
		log.Println(prefix, text)
		if strings.Contains(text, "[MoonriseCommon] Awaiting termination of I/O pool") {
			log.Println("[i] Server shutdown signal detected!")
		}
	}
}
