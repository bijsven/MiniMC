package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const (
	baseURL = "https://api.papermc.io/v2"
	mcDir   = "minecraft"
	jarName = "server.jar"
)

type ProjectResponse struct {
	Versions []string `json:"versions"`
}

type Build struct {
	Build   int    `json:"build"`
	Channel string `json:"channel"`
}

type BuildsResponse struct {
	Builds []Build `json:"builds"`
}

type DownloadInfo struct {
	Name string `json:"name"`
}

type BuildResponse struct {
	Downloads struct {
		Application DownloadInfo `json:"application"`
	} `json:"downloads"`
}

func GetPaper(version string) error {
	var manual = true
	if version == "no_version" {
		manual = false
	}

	log.Println("[i] mkdir /minecraft")
	if err := os.MkdirAll(mcDir, 0755); err != nil {
		return err
	}

	if !manual {
		log.Println("[i] get latest version")
		resp, err := http.Get(baseURL + "/projects/paper")
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return errors.New("bad status: " + resp.Status)
		}

		var project ProjectResponse
		if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
			return err
		}

		if len(project.Versions) == 0 {
			return errors.New("no versions found")
		}

		version = project.Versions[len(project.Versions)-1]
	}

	log.Println("[i] using version", version)
	log.Println("[i] get latest build")

	resp, err := http.Get(fmt.Sprintf("%s/projects/paper/versions/%s/builds", baseURL, version))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("bad status: " + resp.Status)
	}

	var builds BuildsResponse
	if err := json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return err
	}

	if len(builds.Builds) == 0 {
		return errors.New("no builds found")
	}

	latestBuild := builds.Builds[len(builds.Builds)-1]

	manifestPath := mcDir + "/manifest.json"
	if _, err := os.Stat(manifestPath); err == nil {
		mf, err := os.Open(manifestPath)
		if err != nil {
			return err
		}
		defer mf.Close()

		var oldManifest struct {
			Version string `json:"version"`
			Build   int    `json:"build"`
		}
		if err := json.NewDecoder(mf).Decode(&oldManifest); err == nil {
			if oldManifest.Version == version {
				if oldManifest.Build >= latestBuild.Build {
					log.Printf("[i] requested function rejected, because version %s (build %d) is already up-to-date (manifest-check)\n",
						oldManifest.Version, oldManifest.Build)
					return nil
				}
			} else {
				log.Printf("[!] manifest version (%s) differs from requested version (%s). "+
					"This may cause issues!\n", oldManifest.Version, version)
				if !manual {
					log.Println("[!] requested function rejected, because automatic versioning is enabled.")
					log.Println("[!] overwrite by manually setting a version in manifest.json or env to prevent unexpected issues.")
					return nil
				}
			}
		}
	}

	log.Println("[i] get download info for build", latestBuild.Build)

	resp, err = http.Get(fmt.Sprintf("%s/projects/paper/versions/%s/builds/%d", baseURL, version, latestBuild.Build))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("bad status: " + resp.Status)
	}

	var buildInfo BuildResponse
	if err := json.NewDecoder(resp.Body).Decode(&buildInfo); err != nil {
		return err
	}

	filename := buildInfo.Downloads.Application.Name
	log.Println("[i] downloading", filename)

	downloadURL := fmt.Sprintf("%s/projects/paper/versions/%s/builds/%d/downloads/%s",
		baseURL, version, latestBuild.Build, filename)

	resp, err = http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("bad status: " + resp.Status)
	}

	file, err := os.Create(mcDir + "/" + jarName)
	if err != nil {
		return err
	}
	defer file.Close()

	start := time.Now()
	var totalBytes int64
	buffer := make([]byte, 32*1024)

	for {
		bytesRead, readErr := resp.Body.Read(buffer)
		if bytesRead > 0 {
			if _, writeErr := file.Write(buffer[:bytesRead]); writeErr != nil {
				return writeErr
			}
			totalBytes += int64(bytesRead)

			elapsed := time.Since(start).Seconds()
			if elapsed < 0.1 {
				elapsed = 0.1
			}
			speed := float64(totalBytes) / 1024.0 / 1024.0 / elapsed
			log.Printf("\r[i] downloading: %.2f MB done, %.2f MB/s",
				float64(totalBytes)/1024.0/1024.0, speed)
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	log.Printf("\n[i] done dl build %d (%.2f MB)\n",
		latestBuild.Build, float64(totalBytes)/1024.0/1024.0)

	manifest := map[string]interface{}{
		"filename": filename,
		"version":  version,
		"build":    latestBuild.Build,
		"size":     totalBytes,
		"download": downloadURL,
		"date":     time.Now().Format(time.RFC3339),
	}

	manifestFile, err := os.Create(mcDir + "/manifest.json")
	if err != nil {
		return err
	}
	defer manifestFile.Close()

	enc := json.NewEncoder(manifestFile)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest); err != nil {
		return err
	}

	log.Println("[i] manifest.json written")
	return nil
}
