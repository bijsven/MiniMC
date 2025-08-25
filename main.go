package main

import (
	"archive/tar"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"pkg.bijsven.nl/MiniMC/pkg"
	"pkg.bijsven.nl/MiniMC/pkg/server"
)

//go:embed all:client/build
var build embed.FS

type FileInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsDir     bool   `json:"is_dir"`
	Size      int64  `json:"size"`
	ModTime   string `json:"mod_time"`
	Extension string `json:"extension,omitempty"`
}

type FileContent struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type ExtractRequest struct {
	Path        string `json:"path"`
	Destination string `json:"destination,omitempty"`
}

const MinecraftDir = "./minecraft"

func main() {
	start := time.Now()
	pkg.SetLogger()

	if err := os.MkdirAll(MinecraftDir, 0755); err != nil {
		log.Fatal("Failed to create minecraft directory:", err)
	}

	e := echo.New()
	e.HideBanner = true

	e.Use(middleware.BasicAuth(func(username, password string, c echo.Context) (bool, error) {
		if username == os.Getenv("username") && password == os.Getenv("password") {
			return true, nil
		}
		return false, nil
	}))

	buildFS, err := fs.Sub(build, "client/build")
	if err != nil {
		log.Fatal("Failed to create sub filesystem:", err)
	}

	e.GET("/*", echo.WrapHandler(http.FileServer(http.FS(buildFS))))

	api := e.Group("/api")

	api.GET("/logs", logsHandler)
	api.POST("/command", commandHandler)

	files := api.Group("/files")
	files.GET("", listFiles)
	files.GET("/", listFiles)
	files.GET("/content", readFile)
	files.POST("/content", writeFile)
	files.PUT("/content", writeFile)
	files.DELETE("", deleteFile)
	files.POST("/mkdir", createDirectory)
	files.POST("/move", moveFile)
	files.POST("/copy", copyFile)
	files.POST("/extract", extractArchive)
	files.POST("/upload", uploadFile)

	version := os.Getenv("MC_VERSION")
	if version == "" {
		version = "no_version"
	}

	if err := pkg.GetPaper(version); err != nil {
		log.Println("[e]", err)
	}

	log.Printf("[i] Welcome to MiniMC! (Ready in ~%.1fs)\n", time.Since(start).Seconds())

	if err := e.Start(":8080"); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func logsHandler(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "Streaming unsupported")
	}

	ch := pkg.Subscribe()
	for _, logLine := range pkg.GetSessionLogs() {
		c.Response().Write([]byte("data: " + logLine + "\n"))
	}
	flusher.Flush()

	for msg := range ch {
		c.Response().Write([]byte("data: " + msg + "\n"))
		flusher.Flush()
	}
	return nil
}

func commandHandler(c echo.Context) error {
	cmd := c.FormValue("command")
	if cmd == "" {
		return c.NoContent(http.StatusBadRequest)
	}

	switch cmd {
	case "start":
		if err := server.Start(); err != nil {
			return c.NoContent(http.StatusInternalServerError)
		}
		log.Println("[i] Server starting")
	case "kill":
		if err := server.Kill(); err != nil {
			return c.NoContent(http.StatusInternalServerError)
		}
		log.Println("[i] Server killed")
	default:
		if err := server.RunCommand(cmd); err != nil {
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	return c.NoContent(http.StatusOK)
}

func sanitizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return MinecraftDir, nil
	}

	path = strings.TrimPrefix(path, "/")
	cleanPath := filepath.Clean(path)

	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("invalid path: directory traversal not allowed")
	}

	fullPath := filepath.Join(MinecraftDir, cleanPath)
	return fullPath, nil
}

func listFiles(c echo.Context) error {
	path := c.QueryParam("path")
	fullPath, err := sanitizePath(path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_path",
			Message: err.Error(),
		})
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "directory_not_found",
			Message: err.Error(),
		})
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		relativePath, err := filepath.Rel(MinecraftDir, filepath.Join(fullPath, entry.Name()))
		if err != nil {
			relativePath = entry.Name()
		}

		fileInfo := FileInfo{
			Name:    entry.Name(),
			Path:    relativePath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		}

		if !entry.IsDir() {
			fileInfo.Extension = filepath.Ext(entry.Name())
		}

		files = append(files, fileInfo)
	}

	return c.JSON(http.StatusOK, files)
}

func readFile(c echo.Context) error {
	path := c.QueryParam("path")
	if path == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_path",
			Message: "Path parameter is required",
		})
	}

	fullPath, err := sanitizePath(path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_path",
			Message: err.Error(),
		})
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "file_not_found",
			Message: err.Error(),
		})
	}

	if info.IsDir() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "is_directory",
			Message: "Cannot read directory as file",
		})
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "read_error",
			Message: err.Error(),
		})
	}

	return c.JSON(http.StatusOK, FileContent{
		Path:    path,
		Content: string(content),
	})
}

func writeFile(c echo.Context) error {
	var fileContent FileContent
	if err := c.Bind(&fileContent); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_json",
			Message: err.Error(),
		})
	}

	if fileContent.Path == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_path",
			Message: "Path is required",
		})
	}

	fullPath, err := sanitizePath(fileContent.Path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_path",
			Message: err.Error(),
		})
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "mkdir_error",
			Message: err.Error(),
		})
	}

	if err := os.WriteFile(fullPath, []byte(fileContent.Content), 0644); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "write_error",
			Message: err.Error(),
		})
	}

	log.Printf("[i] File written: %s", fileContent.Path)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "File written successfully",
		"path":    fileContent.Path,
	})
}

func deleteFile(c echo.Context) error {
	path := c.QueryParam("path")
	if path == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_path",
			Message: "Path parameter is required",
		})
	}

	fullPath, err := sanitizePath(path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_path",
			Message: err.Error(),
		})
	}

	if fullPath == MinecraftDir {
		return c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Cannot delete minecraft root directory",
		})
	}

	if err := os.RemoveAll(fullPath); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "delete_error",
			Message: err.Error(),
		})
	}

	log.Printf("[i] Deleted: %s", path)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "File/directory deleted successfully",
		"path":    path,
	})
}

func createDirectory(c echo.Context) error {
	var request struct {
		Path string `json:"path"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_json",
			Message: err.Error(),
		})
	}

	if request.Path == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_path",
			Message: "Path is required",
		})
	}

	fullPath, err := sanitizePath(request.Path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_path",
			Message: err.Error(),
		})
	}

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "mkdir_error",
			Message: err.Error(),
		})
	}

	log.Printf("[i] Directory created: %s", request.Path)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Directory created successfully",
		"path":    request.Path,
	})
}

func moveFile(c echo.Context) error {
	var request struct {
		From string `json:"from"`
		To   string `json:"to"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_json",
			Message: err.Error(),
		})
	}

	if request.From == "" || request.To == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_paths",
			Message: "Both 'from' and 'to' paths are required",
		})
	}

	fromPath, err := sanitizePath(request.From)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_from_path",
			Message: err.Error(),
		})
	}

	toPath, err := sanitizePath(request.To)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_to_path",
			Message: err.Error(),
		})
	}

	dir := filepath.Dir(toPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "mkdir_error",
			Message: err.Error(),
		})
	}

	if err := os.Rename(fromPath, toPath); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "move_error",
			Message: err.Error(),
		})
	}

	log.Printf("[i] Moved: %s -> %s", request.From, request.To)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "File/directory moved successfully",
		"from":    request.From,
		"to":      request.To,
	})
}

func copyFile(c echo.Context) error {
	var request struct {
		From string `json:"from"`
		To   string `json:"to"`
	}

	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_json",
			Message: err.Error(),
		})
	}

	if request.From == "" || request.To == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_paths",
			Message: "Both 'from' and 'to' paths are required",
		})
	}

	fromPath, err := sanitizePath(request.From)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_from_path",
			Message: err.Error(),
		})
	}

	toPath, err := sanitizePath(request.To)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_to_path",
			Message: err.Error(),
		})
	}

	info, err := os.Stat(fromPath)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "source_not_found",
			Message: err.Error(),
		})
	}

	if info.IsDir() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "is_directory",
			Message: "Directory copying not supported, use move instead",
		})
	}

	dir := filepath.Dir(toPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "mkdir_error",
			Message: err.Error(),
		})
	}

	src, err := os.Open(fromPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "open_error",
			Message: err.Error(),
		})
	}
	defer src.Close()

	dst, err := os.Create(toPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "create_error",
			Message: err.Error(),
		})
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "copy_error",
			Message: err.Error(),
		})
	}

	log.Printf("[i] Copied: %s -> %s", request.From, request.To)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "File copied successfully",
		"from":    request.From,
		"to":      request.To,
	})
}

func extractArchive(c echo.Context) error {
	var request ExtractRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_json",
			Message: err.Error(),
		})
	}

	if request.Path == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "missing_path",
			Message: "Path is required",
		})
	}

	fullPath, err := sanitizePath(request.Path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_path",
			Message: err.Error(),
		})
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "file_not_found",
			Message: err.Error(),
		})
	}

	if info.IsDir() {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "is_directory",
			Message: "Cannot extract directory",
		})
	}

	if !strings.HasSuffix(strings.ToLower(fullPath), ".tar.gz") && !strings.HasSuffix(strings.ToLower(fullPath), ".tgz") {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "unsupported_format",
			Message: "Only tar.gz and .tgz files are supported",
		})
	}

	destPath := filepath.Dir(fullPath)
	if request.Destination != "" {
		destPath, err = sanitizePath(request.Destination)
		if err != nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_destination",
				Message: err.Error(),
			})
		}
	}

	extractedFiles, err := extractTarGz(fullPath, destPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "extraction_failed",
			Message: err.Error(),
		})
	}

	log.Printf("[i] Extracted %d files from %s to %s", len(extractedFiles), request.Path, destPath)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":         "Archive extracted successfully",
		"source":          request.Path,
		"destination":     destPath,
		"extracted_files": extractedFiles,
		"count":           len(extractedFiles),
	})
}

func extractTarGz(src, dest string) ([]string, error) {
	var extractedFiles []string

	file, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		target := filepath.Join(dest, header.Name)
		target = filepath.Clean(target)

		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) &&
			target != filepath.Clean(dest) {
			return nil, fmt.Errorf("invalid file path: %s", header.Name)
		}

		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory %s: %w", target, err)
			}
			extractedFiles = append(extractedFiles, header.Name)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for file %s: %w", target, err)
		}

		if header.Typeflag == tar.TypeReg {
			outFile, err := os.Create(target)
			if err != nil {
				return nil, fmt.Errorf("failed to create file %s: %w", target, err)
			}

			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return nil, fmt.Errorf("failed to extract file %s: %w", target, err)
			}
			outFile.Close()

			if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
				log.Printf("[w] Failed to set permissions for %s: %v", target, err)
			}

			extractedFiles = append(extractedFiles, header.Name)
		}
	}

	return extractedFiles, nil
}

func uploadFile(c echo.Context) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	path := c.FormValue("path")
	if path == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing path"})
	}

	fullPath, err := sanitizePath(path)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	src, err := fileHeader.Open()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer src.Close()

	dst, err := os.Create(fullPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	log.Printf("[i] Uploaded file: %s", path)
	return c.JSON(http.StatusOK, map[string]string{"message": "File uploaded successfully", "path": path})
}
