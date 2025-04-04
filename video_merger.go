package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fogleman/gg"
)

type Config struct {
	Dest   Destination `json:"dest"`
	Source []string    `json:"source"`
	Font   FontConfig  `json:"font"`
	Frame  FrameConfig `json:"frame"`
	Text   TextConfig  `json:"text"`
}

type Destination struct {
	Output              string `json:"output"`
	IntermediateTextDir string `json:"intermediateTextDir"`
}

type FontConfig struct {
	Path string  `json:"path"`
	Size float64 `json:"size"`
}

type FrameConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	Rate   int `json:"rate"`
}

type TextConfig struct {
	Color      string `json:"color"`
	Background string `json:"background"`
	Duration   int    `json:"duration"`
}

func hexToRGBA(hex string) color.Color {
	var r, g, b int
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

func hexToRGBA2(hex string) color.Color {
	hex = strings.TrimPrefix(hex, "#")

	var r, g, b uint8
	if len(hex) == 6 {
		var ri, gi, bi int
		_, err := fmt.Sscanf(hex, "%02x%02x%02x", &ri, &gi, &bi)
		if err != nil {
			return color.RGBA{0, 0, 0, 255}
		}
		r, g, b = uint8(ri), uint8(gi), uint8(bi)
	} else {
		return color.RGBA{0, 0, 0, 255}
	}

	return color.RGBA{r, g, b, 255}
}

func getVideoFiles(sourceDir string) ([]string, error) {
	var videos []string

	allowedExt := map[string]bool{
		".mp4": true,
		".mov": true,
		".avi": true,
		".mkv": true,
	}

	err := filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if allowedExt[ext] {
				videos = append(videos, filepath.Clean(path))
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	sort.Strings(videos)
	return videos, nil
}

func main() {
	// --- Load Config ---
	configFile := "config.json"
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		return
	}

	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		fmt.Printf("Error parsing config file: %v\n", err)
		return
	}

	// --- Handle Output Path ---
	output := config.Dest.Output
	if output == "" {
		now := time.Now()
		output = fmt.Sprintf("./dest/output_%02d_%02d_%d_%02d_%02d_%02d.mp4",
			now.Day(), now.Month(), now.Year(),
			now.Hour(), now.Minute(), now.Second())
	}

	// --- Load Videos ---
	videos := config.Source
	if len(videos) == 0 {
		videos, err = getVideoFiles("./source")
		if err != nil {
			fmt.Printf("Error reading source directory: %v\n", err)
			return
		}
	}
	sort.Strings(videos)

	// --- Prepare Output Directory ---
	if err := os.MkdirAll(config.Dest.IntermediateTextDir, 0755); err != nil {
		fmt.Printf("Error creating intermediate text directory: %v\n", err)
		return
	}
	defer os.RemoveAll(config.Dest.IntermediateTextDir)

	// --- Load Font ---
	face, err := gg.LoadFontFace(config.Font.Path, config.Font.Size)
	if err != nil {
		fmt.Printf("Error loading font from path '%s': %v\n", config.Font.Path, err)
		return
	}

	textColor := hexToRGBA(config.Text.Color)
	bgColor := hexToRGBA(config.Text.Background)

	// --- Generate Transition Frames ---
	for i, video := range videos {
		if i == 0 {
			continue
		}

		text := fmt.Sprintf("Next: %s", filepath.Base(video))
		numFrames := config.Frame.Rate * config.Text.Duration

		for j := 0; j < numFrames; j++ {
			framePath := fmt.Sprintf("%s/text_%d_frame_%05d.png", config.Dest.IntermediateTextDir, i, j)
			dc := gg.NewContext(config.Frame.Width, config.Frame.Height)
			dc.SetColor(bgColor)
			dc.Clear()
			dc.SetColor(textColor)
			dc.SetFontFace(face)
			dc.DrawStringAnchored(text, float64(config.Frame.Width)/2, float64(config.Frame.Height)/2, 0.5, 0.5)
			if err := dc.SavePNG(framePath); err != nil {
				fmt.Printf("Error saving frame: %v\n", err)
				return
			}
		}
	}

	// --- Create File List ---
	tempFile, err := os.CreateTemp("", "filelist_*.txt")
	if err != nil {
		fmt.Printf("Error creating filelist: %v\n", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// --- Create Transition Videos & Append to File List ---
	for i, video := range videos {
		if i > 0 {
			textFramesPattern := fmt.Sprintf("%s/text_%d_frame_%%05d.png", config.Dest.IntermediateTextDir, i)
			textVideo := fmt.Sprintf("text_transition_%d.mp4", i)

			cmd := exec.Command("ffmpeg", "-y", "-framerate", fmt.Sprintf("%d", config.Frame.Rate),
				"-i", textFramesPattern, "-c:v", "libx264", "-pix_fmt", "yuv420p", textVideo)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				fmt.Printf("Error creating text transition video: %v\n", err)
				return
			}
			defer os.Remove(textVideo)

			if _, err := tempFile.WriteString(fmt.Sprintf("file '%s'\n", textVideo)); err != nil {
				fmt.Printf("Error writing to filelist: %v\n", err)
				return
			}
		}
		if _, err := tempFile.WriteString(fmt.Sprintf("file '%s'\n", video)); err != nil {
			fmt.Printf("Error writing video to filelist: %v\n", err)
			return
		}
	}

	if err := tempFile.Sync(); err != nil {
		fmt.Printf("Error syncing filelist: %v\n", err)
		return
	}

	// --- Merge Videos ---
	fmt.Println("Merging videos into:", output)
	cmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", tempFile.Name(), "-c", "copy", output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error merging videos: %v\n", err)
		return
	}

	fmt.Println("✅ Videos merged successfully into", output)
}
