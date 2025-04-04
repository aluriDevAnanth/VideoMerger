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
	Dest struct {
		Output              string `json:"output"`
		IntermediateTextDir string `json:"intermediateTextDir"`
	} `json:"dest"`
	Source []string `json:"source"`
	Font   struct {
		Path string  `json:"path"`
		Size float64 `json:"size"`
	} `json:"font"`
	Frame struct {
		Width  int `json:"width"`
		Height int `json:"height"`
		Rate   int `json:"rate"`
	} `json:"frame"`
	Text struct {
		Color      string `json:"color"`
		Background string `json:"background"`
		Duration   int    `json:"duration"`
	} `json:"text"`
}

func hexToRGBA(hex string) color.Color {
	var r, g, b int
	fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{uint8(r), uint8(g), uint8(b), 255}
}

func getVideoFiles(sourceDir string) ([]string, error) {
	var videos []string
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".mp4" || ext == ".mov" || ext == ".avi" || ext == ".mkv" {
				videos = append(videos, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(videos)
	return videos, nil
}

func main() {
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

	videos := config.Source
	output := config.Dest.Output
	if output == "" {
		now := time.Now()
		output = fmt.Sprintf("./dest/output_%02d_%02d_%d_%02d_%02d_%02d.mp4",
			now.Day(), now.Month(), now.Year(),
			now.Hour(), now.Minute(), now.Second())
	}
	intermediateTextDir := config.Dest.IntermediateTextDir
	frameWidth := config.Frame.Width
	frameHeight := config.Frame.Height
	frameRate := config.Frame.Rate
	textDuration := config.Text.Duration
	fontPath := config.Font.Path
	fontSize := config.Font.Size
	textColor := hexToRGBA(config.Text.Color)
	bgColor := hexToRGBA(config.Text.Background)

	if len(videos) == 0 {
		vvv, err := getVideoFiles("./source")
		videos = vvv
		if err != nil {
			fmt.Printf("Error reading source directory: %v\n", err)
			return
		}
	}

	fmt.Println(videos)

	if err := os.MkdirAll(intermediateTextDir, 0755); err != nil {
		fmt.Printf("Error creating intermediate text directory: %v\n", err)
		return
	}
	defer os.RemoveAll(intermediateTextDir)

	face, err := gg.LoadFontFace(fontPath, fontSize)
	if err != nil {
		fmt.Printf("Error loading font from path '%s': %v\n", fontPath, err)
		return
	}

	for i, video := range videos {
		if i > 0 {
			text := fmt.Sprintf("Next: %s", video)
			for j := 0; j < frameRate*textDuration; j++ {
				framePath := fmt.Sprintf("%s/text_%d_frame_%05d.png", intermediateTextDir, i, j)
				dc := gg.NewContext(frameWidth, frameHeight)
				dc.SetColor(bgColor)
				dc.Clear()
				dc.SetColor(textColor)
				dc.SetFontFace(face)
				dc.DrawStringAnchored(text, float64(frameWidth)/2, float64(frameHeight)/2, 0.5, 0.5)
				if err := dc.SavePNG(framePath); err != nil {
					fmt.Printf("Error saving frame: %v\n", err)
					return
				}
			}
		}
	}

	tempFile, err := os.Create("filelist.txt")
	if err != nil {
		fmt.Printf("Error creating filelist: %v\n", err)
		return
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	for i, video := range videos {
		if i > 0 {
			textFramesPattern := fmt.Sprintf("%s/text_%d_frame_%%05d.png", intermediateTextDir, i)
			textVideo := fmt.Sprintf("text_transition_%d.mp4", i)
			cmd := exec.Command("ffmpeg", "-y", "-framerate", fmt.Sprintf("%d", frameRate), "-i", textFramesPattern, "-c:v", "libx264", "-pix_fmt", "yuv420p", textVideo)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Printf("Error creating text transition video: %v\n", err)
				return
			}
			defer os.Remove(textVideo)
			_, err := tempFile.WriteString(fmt.Sprintf("file '%s'\n", textVideo))
			if err != nil {
				fmt.Printf("Error writing to filelist: %v\n", err)
				return
			}
		}
		_, err := tempFile.WriteString(fmt.Sprintf("file '%s'\n", video))
		if err != nil {
			fmt.Printf("Error writing to filelist: %v\n", err)
			return
		}
	}

	if err := tempFile.Sync(); err != nil {
		fmt.Printf("Error syncing filelist: %v\n", err)
		return
	}

	fmt.Println(output)

	cmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", tempFile.Name(), "-c", "copy", output)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Merging videos...")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error merging videos: %v\n", err)
		return
	}

	fmt.Println("Videos merged successfully into", output)
	fmt.Println("Videos merged successfully into", output)
	fmt.Println("Videos merged successfully into", output)
}
