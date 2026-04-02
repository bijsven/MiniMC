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

	if activeServer != nil && activeServer.GetStatus() {
		return ErrServerExists
	}

	s := &Server{
		stdin: make(chan string, 100),
		done:  make(chan struct{}),
	}

	activeServer = s
	return s.startInternal()
}

func Stop() error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if activeServer == nil || !activeServer.GetStatus() {
		return errors.New("server is not running")
	}

	return activeServer.RunCommand("stop")
}

func Kill() error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if activeServer == nil || !activeServer.GetStatus() {
		return errors.New("server is not running")
	}

	return activeServer.Kill()
}

func RunCommand(cmd string) error {
	serverMu.Lock()
	defer serverMu.Unlock()

	if activeServer == nil || !activeServer.GetStatus() {
		return errors.New("server is not running")
	}

	return activeServer.RunCommand(cmd)
}

func GetStatus() bool {
	serverMu.Lock()
	defer serverMu.Unlock()

	if activeServer == nil {
		return false
	}
	return activeServer.GetStatus()
}

func (s *Server) startInternal() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	s.isRunning = true

	go s.pipeAndLog(stdoutPipe, "[g] ")
	go s.pipeAndLog(stderrPipe, "[g] ")

	go func() {
		go func() {
			for cmd := range s.stdin {
				if !s.GetStatus() {
					return
				}
				_, _ = stdinPipe.Write([]byte(cmd + "\n"))
			}
		}()

		err := s.cmd.Wait()
		if err != nil {
			log.Println("[e] Server exited with error:", err)
		}

		s.mu.Lock()
		s.isRunning = false
		close(s.done)
		s.mu.Unlock()

		serverMu.Lock()
		activeServer = nil
		serverMu.Unlock()

		log.Println("[i] Server process cleanup finished.")
	}()

	return nil
}

func (s *Server) Stop() error {
	if !s.GetStatus() {
		return errors.New("server is not running")
	}
	return s.RunCommand("stop")
}

func (s *Server) Kill() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return errors.New("server is not running")
	}

	if err := s.cmd.Process.Kill(); err != nil {
		return err
	}

	return nil
}

func (s *Server) RunCommand(cmd string) error {
	if !s.GetStatus() {
		return errors.New("server is not running")
	}
	s.stdin <- cmd
	return nil
}

func (s *Server) GetStatus() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}

func (s *Server) pipeAndLog(pipeReader io.ReadCloser, prefix string) {
	scanner := bufio.NewScanner(pipeReader)
	for scanner.Scan() {
		text := scanner.Text()
		log.Println(prefix, text)
		if strings.Contains(text, "[MoonriseCommon] Awaiting termination of I/O pool for up to 60s...") {
			log.Println("[i] Server has been stopped!")
		}
	}
}
