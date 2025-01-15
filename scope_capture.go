package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	defaultIP   = "169.254.247.73"
	defaultPort = "5555"
	logFilePath = "screen_grab.log"
)

func init() {
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Println("***** New run started...")
}

func main() {
	fileType := flag.String("type", "png", "File type to save: png, bmp, csv")
	hostname := flag.String("host", defaultIP, "Hostname or IP address of the oscilloscope")
	filename := flag.String("file", "", "Optional name of output file")
	label1 := flag.String("label1", "", "Channel 1 label")
	label2 := flag.String("label2", "", "Channel 2 label")
	label3 := flag.String("label3", "", "Channel 3 label")
	label4 := flag.String("label4", "", "Channel 4 label")
	flag.Parse()

	if err := run(*hostname, *filename, *fileType, []string{*label1, *label2, *label3, *label4}); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(hostname, filename, fileType string, labels []string) error {
	if err := testPing(hostname); err != nil {
		return err
	}

	conn, err := net.Dial("tcp", hostname+":"+defaultPort)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", hostname, err)
	}
	defer conn.Close()

	instrumentID, err := command(conn, "*IDN?")
	if err != nil {
		return err
	}
	fmt.Println("Instrument ID:", instrumentID)

	if filename == "" {
		filename = fmt.Sprintf("%s_%s.png", strings.ReplaceAll(instrumentID, ",", "_"), time.Now().Format("2006-01-02_15-04-05"))
	}

	if fileType == "png" {
		return captureScreen(conn, filename, labels)
	}

	return errors.New("unsupported file type")
}

func testPing(hostname string) error {
	conn, err := net.DialTimeout("tcp", hostname+":"+defaultPort, 2*time.Second)
	if err != nil {
		log.Printf("Ping failed: %v", err)
		return fmt.Errorf("ping failed: %v", err)
	}
	conn.Close()
	log.Println("Ping successful")
	return nil
}

func command(conn net.Conn, scpi string) (string, error) {
	log.Printf("SCPI to be sent: %s", scpi)
	response := ""
	for {
		_, err := fmt.Fprintf(conn, "*OPC?\n")
		if err != nil {
			return "", fmt.Errorf("failed to send *OPC?: %v", err)
		}
		log.Println("Sent SCPI: *OPC?")
		resp, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read *OPC? response: %v", err)
		}
		log.Println("Received response for *OPC?", strings.TrimSpace(resp))
		if strings.TrimSpace(resp) == "1" {
			break
		}
	}

	_, err := fmt.Fprintf(conn, "%s\n", scpi)
	if err != nil {
		return "", fmt.Errorf("failed to send SCPI command: %v", err)
	}
	log.Printf("Sent SCPI: %s", scpi)

	response, err = bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read SCPI response: %v", err)
	}
	log.Printf("Received SCPI response: %s", strings.TrimSpace(response))
	return strings.TrimSpace(response), nil
}

func captureScreen(conn net.Conn, filename string, labels []string) error {
	data, err := command(conn, ":DISP:DATA? ON,OFF,PNG")
	if err != nil {
		return err
	}

	log.Printf("Decoding image data of length %d", len(data))
	img, _, err := image.Decode(bytes.NewReader([]byte(data)))
	if err != nil {
		return err
	}
	log.Print("Image decoded successfully.")

	outFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer outFile.Close()

	imgWithLabels := addLabelsToImage(img, labels)
	return png.Encode(outFile, imgWithLabels)
}

func addLabelsToImage(img image.Image, labels []string) image.Image {
	bounds := img.Bounds()
	newImg := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			newImg.Set(x, y, img.At(x, y))
		}
	}

	colors := []color.Color{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{255, 255, 0, 255}}
	for i, label := range labels {
		if label != "" {
			for x := 10; x < 30; x++ {
				for y := 10 + i*30; y < 30+i*30; y++ {
					newImg.Set(x, y, colors[i])
				}
			}
		}
	}
	return newImg
}
