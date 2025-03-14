package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net"
	"os"
	"scopecapture/pkg/moduleconfig"
	"scopecapture/pkg/quicklog"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	smallWait      = 1 * time.Second
	sendTimeout    = 1 * time.Second
	receiveTimeout = 1 * time.Second
	pingTimeout    = 2 * time.Second
)

var (
	log               *quicklog.LoggerT = nil // Assigned at runtime
	flagVersion       bool
	flagDebug         bool
	flagScopeHostname string
	flagScopePort     int
	flagFilename      string
	flagNote          string
	flagLabel1        string
	flagLabel2        string
	flagLabel3        string
	flagLabel4        string

	colorTimestamp = color.RGBA{0, 219, 146, 255} // LightGreen
	colorLabels    = []color.Color{
		color.RGBA{176, 176, 176, 255}, // Note: Gray
		color.RGBA{247, 250, 82, 255},  // CH1: Yellow
		color.RGBA{0, 225, 221, 255},   // CH2: Cyan
		color.RGBA{221, 0, 221, 255},   // CH3: Magenta
		color.RGBA{0, 127, 245, 255},   // CH4: Blue
	}
)

func main() {
	var err error

	// ---------------------------
	// Show Version
	// ---------------------------
	versionInfo := fmt.Sprintf("%s (%s), %s", config.AppName, config.AppTitle, moduleconfig.ModuleVersion)
	fmt.Println(versionInfo)

	// ---------------------------
	// Parse command line arguments
	// ---------------------------
	flag.BoolVar(&flagVersion, "version", false, "Print version and exit.")
	flag.BoolVar(&flagDebug, "d", false, "Enable debug printing.")
	flag.BoolVar(&flagDebug, "debug", false, "Enable debug printing.")
	flag.StringVar(&flagScopeHostname, "host", "",
		fmt.Sprintf("Hostname or IP address of the oscilloscope (Defaults to %q)",
			config.ScopeHostname))
	flag.IntVar(&flagScopePort, "port", 0,
		fmt.Sprintf("Port number of the oscilloscope (Defaults to %d)", config.ScopePort))
	flag.StringVar(&flagFilename, "file", "", "Optional name of output file")
	flag.StringVar(&flagNote, "note", "", "Note to add to the image")
	flag.StringVar(&flagNote, "n", "", "Note to add to the image")
	flag.StringVar(&flagLabel1, "label1", "", "Channel 1 label")
	flag.StringVar(&flagLabel1, "l1", "", "Channel 1 label")
	flag.StringVar(&flagLabel2, "label2", "", "Channel 2 label")
	flag.StringVar(&flagLabel2, "l2", "", "Channel 2 label")
	flag.StringVar(&flagLabel3, "label3", "", "Channel 3 label")
	flag.StringVar(&flagLabel3, "l3", "", "Channel 3 label")
	flag.StringVar(&flagLabel4, "label4", "", "Channel 4 label")
	flag.StringVar(&flagLabel4, "l4", "", "Channel 4 label")

	flag.Parse()

	if flagVersion {
		// We have already printed the version, so just exit
		os.Exit(0)
	}

	// ------------------------
	// Start logger
	// ------------------------
	logLevel := quicklog.LogLevelInfo
	if flagDebug {
		logLevel = quicklog.LogLevelDebug
	}
	config.Hostname = getComputerName()
	loggingConfig := quicklog.ConfigT{
		Directory:  pathDirLogs,
		Filename:   config.Hostname + "." + config.AppName + ".log",
		Level:      logLevel,
		MaxSize:    5,
		MaxBackups: 3,
	}
	log = quicklog.ConfigureLogger(loggingConfig)
	log.Info(versionInfo)

	// This will only show up in the log if we enabled debug logging above
	log.Debug("Debug logging enabled.")

	// ---------------------------
	// Load and parse config file
	// ---------------------------
	err = loadAndParseConfigFile()
	if err != nil {
		log.ErrorPrintf("Failed to load config file: %v", err)
		os.Exit(1)
	}
	// Now that configuration has been loaded, we can set the ip and port, and then apply the
	// command line overrides, if any.
	scopeHostname := config.ScopeHostname
	scopePort := config.ScopePort
	if flagScopeHostname != "" {
		scopeHostname = flagScopeHostname
		log.InfoPrintf("Adopting scope hostname fom command line: %q", scopeHostname)
	}
	if flagScopePort != 0 {
		scopePort = flagScopePort
		log.InfoPrintf("Adopting scope port from command line: %d", scopePort)
	}

	err = run(
		scopeHostname, scopePort, flagFilename, "png", flagNote,
		[]string{flagLabel1, flagLabel2, flagLabel3, flagLabel4})
	if err != nil {
		log.ErrorPrintf("%v", err)
		os.Exit(1)
	}
}

func run(
	scopeHostname string,
	scopePort int,
	filename,
	fileType string,
	note string,
	labels []string) error {
	if err := testPing(scopeHostname); err != nil {
		return err
	}

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", scopeHostname, scopePort))
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", scopeHostname, err)
	}
	defer conn.Close()

	instrumentID, err := command(conn, "*IDN?")
	if err != nil {
		return err
	}
	log.InfoPrintf("Instrument ID: %q.", instrumentID)

	if filename == "" && note != "" {
		// Set filename to note, converted to filename-safe characters
		filename = makeFilenameSafe(note) + ".png"
	}
	if filename == "" {
		id := strings.ReplaceAll(instrumentID, ",", "_")
		id = strings.ReplaceAll(id, " ", "_")
		filename = fmt.Sprintf(
			"%s_%s.png", id, time.Now().Format("2006-01-02_15-04-05"))
	}

	if fileType == "png" {
		return captureScreen(conn, filename, note, labels)
	}

	return errors.New("unsupported file type")
}

func testPing(hostname string) error {
	ip := fmt.Sprintf("%s:%d", hostname, config.ScopePort)
	log.InfoPrintf("Pinging scope at %q...", ip)
	conn, err := net.DialTimeout("tcp", ip, pingTimeout)
	if err != nil {
		log.Infof("Ping failed: %v", err)
		return fmt.Errorf("ping failed: %v", err)
	}
	conn.Close()
	log.InfoPrint("    Ping successful")
	return nil
}

func commandRaw(conn net.Conn, scpi string) ([]byte, error) {
	log.Infof("commandRaw(): SCPI to be sent: %q", scpi)
	if err := waitForReady(conn); err != nil {
		return []byte{}, err
	}

	// Set a write deadline for sending the command
	err := conn.SetWriteDeadline(time.Now().Add(sendTimeout))
	if err != nil {
		return nil, fmt.Errorf("failed to set write deadline: %v", err)
	}

	log.Infof("commandRaw(): Sending SCPI: %q", scpi)
	_, err = fmt.Fprintf(conn, "%s\n", scpi)
	if err != nil {
		return nil, fmt.Errorf("failed to send SCPI command: %v", err)
	}

	// Set a read deadline for reading the response
	err = conn.SetReadDeadline(time.Now().Add(receiveTimeout))
	if err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %v", err)
	}

	// FIXME: Hack. Just read 17 bytes.  I suspect the read above was also consuming extra
	// bytes after the newline.
	data := make([]byte, 17)
	n, err := conn.Read(data)
	if err != nil {
		if err == io.EOF {
			log.Info("commandRaw(): Reached EOF while reading response.")
		} else {
			return nil, fmt.Errorf("failed to read SCPI response: %v", err)
		}
	}

	// log.Infof("Received SCPI response of %d bytes: %q", len(response), response)
	log.Infof("Received SCPI response of %d bytes: %q", n, string(data))
	// return response, nil
	return data, nil
}

func command(conn net.Conn, scpi string) (string, error) {
	log.Infof("SCPI to be sent: %s", scpi)
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
	log.Infof("Received SCPI response: %q", response)
	return response, nil
}

func captureScreen(conn net.Conn, filename string, note string, labels []string) error {
	log.InfoPrint("Capturing scope screen...")
	// Send the SCPI command to capture the screen
	buff, err := commandRaw(conn, ":DISP:DATA? ON,OFF,PNG")
	if err != nil {
		return err
	}

	expectedBuffLengthBytes := expectedBuffBytes(buff)
	log.Infof("expectedBuffLengthBytes: %d", expectedBuffLengthBytes)

	// Prepare buffer to hold the full response
	data := make([]byte, expectedBuffLengthBytes)
	copy(data, buff)

	// Continue reading until the full expected buffer is received
	bytesRead := len(buff)
	for bytesRead < expectedBuffLengthBytes {
		// Set a read deadline to avoid blocking forever
		err := conn.SetReadDeadline(time.Now().Add(receiveTimeout))
		if err != nil {
			return fmt.Errorf("failed to set read deadline: %v", err)
		}

		// Read the remaining data directly into the buffer
		// small := make([]byte, 1)
		log.Infof("Requesting %d bytes...", len(data[bytesRead:]))
		n, err := conn.Read(data[bytesRead:])
		// n, err := conn.Read(small)
		if err != nil {
			if err == io.EOF {
				log.Infof("Reached EOF after reading %d/%d bytes", bytesRead, expectedBuffLengthBytes)
				break
			}
			// return fmt.Errorf("failed to read SCPI response: %v", err)
			log.Infof("ABORT on last read: failed to read SCPI response: %v", err)
			break
		}
		// DEBUG: Sleep 100ms
		// time.Sleep(100 * time.Millisecond)

		// data[bytesRead] = small[0]
		bytesRead += n
		log.Infof("Last byte read was %q", data[bytesRead-1])

		log.Infof("Read %d bytes (%d/%d total. %d remaining)",
			n, bytesRead, expectedBuffLengthBytes, expectedBuffLengthBytes-bytesRead)
	}

	// // Verify if we got the full response
	// if bytesRead < expectedBuffLengthBytes {
	// 	log.Infof("Incomplete data: got %d out of %d bytes", bytesRead, expectedBuffLengthBytes)
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

	// FIXME: This is a workaround for a Rigol scope that incorrectly is generating bad PNG checksums.
	log.InfoPrint("Auto-correcting PNG checksum...")
	data, err = FixPNGChecksum(data)
	if err != nil {
		return fmt.Errorf("failed to fix PNG checksum: %v", err)
	}
	log.InfoPrint("    Checksum corrected.")

	// Save the raw (unannotated) scope capture to a file
	outPath := pathDirScopeCaptures + "/raw_scope_capture.png"
	if err := os.MkdirAll(pathDirScopeCaptures, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %v", pathDirScopeCaptures, err)
	}
	outFileDebug, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create DEBUG output file: %v", err)
	}
	defer outFileDebug.Close()
	outFileDebug.Write(data)
	log.InfoPrintf("Wrote raw scope capture to %q.", outPath)

	// Decode the PNG image from the buffer
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	// Create and save the image file
	outPath = pathDirScopeCaptures + "/" + filename
	outPath = appendNumericSuffixOnFileExists(outPath)
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	log.InfoPrint("Annotating scope capture...")
	imgWithLabels := addLabelsToImage(img, note, labels)
	err = png.Encode(outFile, imgWithLabels)
	if err != nil {
		return fmt.Errorf("failed to encode PNG: %v", err)
	}
	log.InfoPrintf("Wrote annotated scope capture to %q.", outPath)

	return nil
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
	now := time.Now()
	addLabel(newImg, now.Format("2006-01-02"), 2, 2, colorTimestamp)
	addLabel(newImg, now.Format("15:04:05"), 2, 15, colorTimestamp)

	// Draw note and labels
	const labelSpacing = 14
	locationY := 44
	locationX := 800 - 10 - labelSpacing
	allLabels := append([]string{note}, labels...)
	for i, label := range allLabels {
		if label != "" {
			text := label
			if i > 0 {
				text = fmt.Sprintf("CH%d: %s", i, label)
			}
			addRotatedLabel(newImg, text, locationX, locationY, colorLabels[i])
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
	// d.DrawString(label)
	// Manually draw each character with additional spacing
	for _, ch := range label {
		d.DrawString(string(ch))
		d.Dot.X += fixed.I(1) // Add 1 pixel of extra spacing
	}
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
	// d.DrawString(label)
	// Manually draw each character with additional spacing
	for _, ch := range label {
		d.DrawString(string(ch))
		d.Dot.X += fixed.I(1) // Add 1 pixel of extra spacing
	}

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
	log.Infof("expectedDataBytesStr: %q", expectedDataBytesStr)
	// convert string (decimal)	to int
	expectedDataBytes, err := strconv.Atoi(expectedDataBytesStr)
	// FIXME: Handle error here better
	if err != nil {
		log.Infof("Error converting string to int: %v", err)
		panic(err)
	}
	return expectedDataBytes
}

func expectedBuffBytes(buff []byte) int {
	// TODO: I think this last +1 is for the terminating newline.  Confirm.
	return tmcHeaderBytes(buff) + expectedDataBytes(buff) + 1
}

func waitForReady(conn net.Conn) error {
	reader := bufio.NewReader(conn)

	for {
		log.Info("waitForReady(): Sending SCPI: *OPC? # May I send a command? 1==yes")

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
				log.Info("waitForReady(): Timeout waiting for response, retrying...")
				continue
			}
			return fmt.Errorf("failed to read *OPC? response: %v", err)
		}

		log.Info("waitForReady(): Received response!")

		if strings.TrimSpace(response) == "1" {
			log.Info("waitForReady(): Wait done")
			break
		}

		// Optional small delay to avoid spamming the device
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}

func getComputerName() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	computername := strings.Replace(hostname, ".local", "", 1)
	fmt.Printf("hostname:%#v computername:%#v\n", hostname, computername)
	return computername
}

func makeFilenameSafe(input string) string {
	replacer := strings.NewReplacer(
		" ", "_",
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	return replacer.Replace(input)
}

func appendNumericSuffixOnFileExists(filename string) string {
	if _, err := os.Stat(filename); err == nil {
		// File exists
		base := filename[:len(filename)-4]
		ext := filename[len(filename)-4:]
		for i := 2; ; i++ {
			newFilename := fmt.Sprintf("%s_%d%s", base, i, ext)
			if _, err := os.Stat(newFilename); err != nil {
				return newFilename
			}
		}
	}
	return filename
}

// FixPNGChecksum takes a []byte PNG and corrects the chunk checksums.
func FixPNGChecksum(pngData []byte) ([]byte, error) {
	const pngHeaderSize = 8
	const chunkHeaderSize = 8
	const crcSize = 4

	// Verify PNG signature
	pngSignature := []byte{137, 80, 78, 71, 13, 10, 26, 10}
	if !bytes.Equal(pngData[:pngHeaderSize], pngSignature) {
		return nil, fmt.Errorf("invalid PNG signature")
	}

	buffer := bytes.NewBuffer(pngData[:pngHeaderSize]) // Start with the header
	data := pngData[pngHeaderSize:]

	for len(data) > 0 {
		if len(data) < chunkHeaderSize {
			return nil, fmt.Errorf("unexpected end of PNG data")
		}

		// Read chunk length
		length := binary.BigEndian.Uint32(data[:4])
		if len(data) < int(chunkHeaderSize+length+crcSize) {
			return nil, fmt.Errorf("chunk size exceeds remaining data")
		}

		chunkType := data[4:8]
		chunkData := data[8 : 8+length]
		// Correct CRC: calculated over chunkType + chunkData
		crcData := append(chunkType, chunkData...)
		correctCRC := crc32.ChecksumIEEE(crcData)

		// Write chunk length, type, data
		buffer.Write(data[:8+length])
		// Write corrected CRC
		// DEBUG: Intentionally corrupt this
		// correctCRC += 1
		binary.Write(buffer, binary.BigEndian, correctCRC)

		// Move to the next chunk
		data = data[8+length+crcSize:]

		// Stop if we hit the IEND chunk
		if bytes.Equal(chunkType, []byte("IEND")) {
			break
		}
	}

	return buffer.Bytes(), nil
}
