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

	go func() {
		if s == nil || s.cmd == nil {
			return
		}

		err := s.cmd.Wait()
		if err != nil {
			log.Println("[e] Server process exited with error:", err)
		}

		s.mu.Lock()
		s.isRunning = false

		if s.done != nil {
			select {
			case <-s.done:
			default:
				close(s.done)
			}
		}
		s.mu.Unlock()

		serverMu.Lock()
		if activeServer == s {
			activeServer = nil
		}
		serverMu.Unlock()
	}()

	if s.isRunning {
		log.Println("[e] Server is already running!")
		return errors.New("server is already running")
	}

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
		"-jar",
		"server.jar",
		"nogui",
	)
	s.cmd.Dir = "minecraft"

	stdoutPipe, err := s.cmd.StdoutPipe()
	if err != nil {
		log.Println("[e] Failed to get stdout pipe:", err)
		return err
	}
	stderrPipe, err := s.cmd.StderrPipe()
	if err != nil {
		log.Println("[e] Failed to get stderr pipe:", err)
		return err
	}
	stdinPipe, err := s.cmd.StdinPipe()
	if err != nil {
		log.Println("[e] Failed to get stdin pipe:", err)
		return err
	}

	if err := s.cmd.Start(); err != nil {
		log.Println("[e] Failed to start server process:", err)
		return err
	}

	s.isRunning = true

	go s.pipeAndLog(stdoutPipe, "[g] ")
	go s.pipeAndLog(stderrPipe, "[g] ")

	go func() {
		for cmd := range s.stdin {
			_, _ = stdinPipe.Write([]byte(cmd + "\n"))
		}
	}()

	go func() {
		s.cmd.Wait()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.isRunning = false
		close(s.done)

		serverMu.Lock()
		activeServer = nil
		serverMu.Unlock()
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
