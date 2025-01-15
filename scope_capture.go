package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	defaultIP      = "169.254.247.73"
	defaultPort    = "5555"
	logFilePath    = "screen_grab.log"
	smallWait      = 1 * time.Second
	sendTimeout    = 1 * time.Second
	receiveTimeout = 1 * time.Second
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
	// note := flag.String("note", "", "Channel 1 label")
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

func commandRaw(conn net.Conn, scpi string) ([]byte, error) {
	log.Printf("commandRaw(): SCPI to be sent: %q", scpi)
	if err := waitForReady(conn); err != nil {
		return []byte{}, err
	}

	// Set a write deadline for sending the command
	err := conn.SetWriteDeadline(time.Now().Add(sendTimeout))
	if err != nil {
		return nil, fmt.Errorf("failed to set write deadline: %v", err)
	}

	log.Printf("commandRaw(): Sending SCPI: %q", scpi)
	_, err = fmt.Fprintf(conn, "%s\n", scpi)
	if err != nil {
		return nil, fmt.Errorf("failed to send SCPI command: %v", err)
	}

	// Set a read deadline for reading the response
	err = conn.SetReadDeadline(time.Now().Add(receiveTimeout))
	if err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %v", err)
	}

	// Use bufio.Reader to read until newline
	// reader := bufio.NewReader(conn)
	// response, err := reader.ReadBytes('\n')
	// if err != nil {
	// 	if err == io.EOF {
	// 		log.Print("commandRaw(): Reached EOF while reading response.")
	// 	} else {
	// 		return nil, fmt.Errorf("failed to read SCPI response: %v", err)
	// 	}
	// }

	// FIXME: Hack. Just read 17 bytes.  I suspect the read above was also consuming extra
	// bytes after the newline.
	data := make([]byte, 17)
	n, err := conn.Read(data)
	if err != nil {
		if err == io.EOF {
			log.Print("commandRaw(): Reached EOF while reading response.")
		} else {
			return nil, fmt.Errorf("failed to read SCPI response: %v", err)
		}
	}

	// log.Printf("Received SCPI response of %d bytes: %q", len(response), response)
	log.Printf("Received SCPI response of %d bytes: %q", n, string(data))
	// return response, nil
	return data, nil
}

func command(conn net.Conn, scpi string) (string, error) {
	log.Printf("SCPI to be sent: %s", scpi)
	if err := waitForReady(conn); err != nil {
		return "", err
	}
	_, err := fmt.Fprintf(conn, "%s\n", scpi)
	if err != nil {
		return "", fmt.Errorf("failed to send SCPI command: %v", err)
	}

	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read SCPI response: %v", err)
	}
	response = strings.TrimSpace(response)
	log.Printf("Received SCPI response: %q", response)
	return response, nil
}

func captureScreen(conn net.Conn, filename string, labels []string) error {
	// Send the SCPI command to capture the screen
	buff, err := commandRaw(conn, ":DISP:DATA? ON,OFF,PNG")
	if err != nil {
		return err
	}

	expectedBuffLengthBytes := expectedBuffBytes(buff)
	log.Printf("expectedBuffLengthBytes: %d", expectedBuffLengthBytes)
	// // FIXME: Hack for testing
	// expectedBuffLengthBytes += 1
	// log.Printf("expectedBuffLengthBytes: %d", expectedBuffLengthBytes)

	// Prepare buffer to hold the full response
	data := make([]byte, expectedBuffLengthBytes)
	copy(data, buff)

	// Continue reading until the full expected buffer is received
	bytesRead := len(buff)
	for bytesRead < expectedBuffLengthBytes {
		// Set a read deadline to avoid blocking forever
		err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		if err != nil {
			return fmt.Errorf("failed to set read deadline: %v", err)
		}

		// Read the remaining data directly into the buffer
		// small := make([]byte, 1)
		log.Printf("Requesting %d bytes...", len(data[bytesRead:]))
		n, err := conn.Read(data[bytesRead:])
		// n, err := conn.Read(small)
		if err != nil {
			if err == io.EOF {
				log.Printf("Reached EOF after reading %d/%d bytes", bytesRead, expectedBuffLengthBytes)
				break
			}
			// return fmt.Errorf("failed to read SCPI response: %v", err)
			log.Printf("ABORT on last read: failed to read SCPI response: %v", err)
			break
		}
		// DEBUG: Sleep 100ms
		// time.Sleep(100 * time.Millisecond)

		// data[bytesRead] = small[0]
		bytesRead += n
		log.Printf("Last byte read was %q", data[bytesRead-1])

		log.Printf("Read %d bytes (%d/%d total. %d remaining)",
			n, bytesRead, expectedBuffLengthBytes, expectedBuffLengthBytes-bytesRead)
	}

	// // Verify if we got the full response
	// if bytesRead < expectedBuffLengthBytes {
	// 	log.Printf("Incomplete data: got %d out of %d bytes", bytesRead, expectedBuffLengthBytes)
	// 	return errors.New("failed to read all expected buffer data")
	// }

	// // Save the `data` to a file for debugging
	// outFile, err := os.Create("raw_screenshot.DEBUG.png")
	// if err != nil {
	// 	return fmt.Errorf("failed to create output file: %v", err)
	// }
	// defer outFile.Close()
	// outFile.Write(data)

	// Strip TMC Blockheader and keep only the data
	tmcHeaderLen := tmcHeaderBytes(data)
	expectedDataLen := expectedDataBytes(data)
	if len(data) < tmcHeaderLen+expectedDataLen {
		return errors.New("buffer is too short for expected data")
	}
	// data = data[tmcHeaderLen : tmcHeaderLen+expectedDataLen]
	data = data[tmcHeaderLen : bytesRead-1]

	// Save the `data` to a file for debugging
	outFileDebug, err := os.Create("raw_screenshot.DEBUG.png")
	if err != nil {
		return fmt.Errorf("failed to create DEBUG output file: %v", err)
	}
	defer outFileDebug.Close()
	outFileDebug.Write(data)

	// Decode the PNG image from the buffer
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	// Create and save the image file
	outFile, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	// FIXME: Replace this Test Note
	imgWithLabels := addLabelsToImage(img, "Test Note", labels)
	return png.Encode(outFile, imgWithLabels)
}

func addLabelsToImage(img image.Image, note string, labels []string) image.Image {
	bounds := img.Bounds()
	newImg := image.NewRGBA(bounds)
	draw.Draw(newImg, bounds, img, bounds.Min, draw.Src)

	// Define erasure rectangles (blackout areas)
	eraseRects := []image.Rectangle{
		image.Rect(3, 8, 80, 28),       // Logo
		image.Rect(0, 37, 59, 450),     // Left menu
		image.Rect(705, 38, 799, 436),  // Right menu items
		image.Rect(690, 39, 704, 117),  // Right menu tab text
		image.Rect(762, 456, 799, 479), // Lower right icon
	}

	// Fill erase areas with black
	for _, rect := range eraseRects {
		draw.Draw(newImg, rect, &image.Uniform{color.Black}, image.Point{}, draw.Src)
	}

	// Draw timestamp
	addLabel(newImg, time.Now().Format("2006-01-02\n15:04:05"), 4, 2, color.White)

	// Define label colors
	colors := []color.Color{
		color.RGBA{176, 176, 176, 255}, // Note: Gray
		color.RGBA{247, 250, 82, 255},  // Label 1: Yellow
		color.RGBA{0, 225, 221, 255},   // Label 2: Cyan
		color.RGBA{221, 0, 221, 255},   // Label 3: Magenta
		color.RGBA{0, 127, 245, 255},   // Label 4: Blue
	}

	// Draw note and labels
	const labelSpacing = 16
	locationY := 44
	locationX := 800 - 10 - labelSpacing
	allLabels := append([]string{note}, labels...)
	for i, label := range allLabels {
		if label != "" {
			text := label
			if i > 0 {
				text = "CH" + string('0'+i) + ": " + label
			}
			addRotatedLabel(newImg, text, locationX, locationY, colors[i])
			locationX -= labelSpacing
		}
	}

	return newImg
}

func addLabel(img *image.RGBA, label string, x, y int, col color.Color) {
	face := basicfont.Face7x13
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, y+13),
	}
	d.DrawString(label)
}

func addRotatedLabel(img *image.RGBA, label string, x, y int, col color.Color) {
	face := basicfont.Face7x13
	labelImg := image.NewRGBA(image.Rect(0, 0, 200, 20))
	d := &font.Drawer{
		Dst:  labelImg,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(0, 13),
	}
	d.DrawString(label)

	// Rotate 90 degrees clockwise
	rotatedImg := rotate90(labelImg)
	draw.Draw(img, rotatedImg.Bounds().Add(image.Pt(x, y)), rotatedImg, image.Point{}, draw.Over)
}

func rotate90(img *image.RGBA) *image.RGBA {
	bounds := img.Bounds()
	rotated := image.NewRGBA(image.Rect(0, 0, bounds.Dy(), bounds.Dx()))
	for x := bounds.Min.X; x < bounds.Max.X; x++ {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			rotated.Set(bounds.Max.Y-y-1, x, img.At(x, y))
		}
	}
	return rotated
}

func tmcHeaderBytes(buff []byte) int {
	return 2 + int(buff[1]-'0')
}

func expectedDataBytes(buff []byte) int {
	headerBytes := tmcHeaderBytes(buff)
	expectedDataBytesStr := string(buff[2:headerBytes])
	log.Printf("expectedDataBytesStr: %q", expectedDataBytesStr)
	// convert string (decimal)	to int
	// expectedDataBytes, err := binary.ReadVarint(strings.NewReader(expectedDataBytesStr))
	expectedDataBytes, err := strconv.Atoi(expectedDataBytesStr)
	// FIXME: Handle error here better
	if err != nil {
		log.Printf("Error converting string to int: %v", err)
		panic(err)
	}
	return expectedDataBytes
}

func expectedBuffBytes(buff []byte) int {
	// TODO: I think this last +1 is for the terminating newline.  Confirm.
	return tmcHeaderBytes(buff) + expectedDataBytes(buff) + 1
}

// func waitForReady(conn net.Conn) error {
// 	log.Print("waitForReady(): Sending SCPI: *OPC?")
// 	_, err := fmt.Fprintf(conn, "*OPC?\n")
// 	if err != nil {
// 		return fmt.Errorf("failed to send *OPC?: %v", err)
// 	}
// 	response, err := bufio.NewReader(conn).ReadString('\n')
// 	if err != nil {
// 		return fmt.Errorf("failed to read *OPC? response: %v", err)
// 	}
// 	if strings.TrimSpace(response) != "1" {
// 		return errors.New("oscilloscope not ready")
// 	}
// 	return nil
// }

func waitForReady(conn net.Conn) error {
	reader := bufio.NewReader(conn)

	for {
		log.Print("waitForReady(): Sending SCPI: *OPC? # May I send a command? 1==yes")

		_, err := fmt.Fprintf(conn, "*OPC?\n")
		if err != nil {
			return fmt.Errorf("failed to send *OPC?: %v", err)
		}

		// Set a 1-second read timeout
		err = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		if err != nil {
			return fmt.Errorf("failed to set read deadline: %v", err)
		}

		response, err := reader.ReadString('\n')
		if err != nil {
			// If it's a timeout, continue trying
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				log.Print("waitForReady(): Timeout waiting for response, retrying...")
				continue
			}
			return fmt.Errorf("failed to read *OPC? response: %v", err)
		}

		log.Print("waitForReady(): Received response!")

		if strings.TrimSpace(response) == "1" {
			log.Print("waitForReady(): Wait done")
			break
		}

		// Optional small delay to avoid spamming the device
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}
