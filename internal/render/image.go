package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	_ "image/gif"  // register GIF decoder
	_ "image/jpeg" // register JPEG decoder
	_ "image/png"  // register PNG decoder
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// fetchImage downloads an image with a browser-ish UA and a short timeout.
// Returns the decoded image.Image, or an error if the fetch/decode failed.
func fetchImage(url string) (image.Image, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (guddit/0.3)")
	req.Header.Set("Accept", "image/*")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image fetch %s: %s", url, resp.Status)
	}
	// Limit to 5MB to avoid OOM
	data, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// Try JPEG explicitly (some servers send JPEG with wrong content-type)
		img, err = jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("decode image: %w", err)
		}
	}
	return img, nil
}

// encodePNG encodes an image.Image to PNG bytes.
func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderImageCells renders an image constrained to a number of terminal rows
// (height) and a max pixel width. The height in cells makes the layout
// predictable in TUI views where line-counting matters.
// It prints a placeholder (rows lines of spaces) first, then the image data.
func RenderImageCells(w io.Writer, caps Capabilities, url string, maxWidthPx, rows int) error {
	img, err := fetchImage(url)
	if err != nil {
		return err
	}
	pngData, err := encodePNG(img)
	if err != nil {
		return err
	}

	// Print placeholder rows first (spaces) so terminal reserves the space.
	// Each row = one newline.
	for i := 0; i < rows; i++ {
		fmt.Fprint(w, "\n")
	}
	// Move cursor back up 'rows' lines to overlay the image.
	fmt.Fprintf(w, "\x1b[%dA", rows)

	switch caps.ImageProtocol {
	case ImageProtocolITerm2:
		return renderITerm2(w, pngData, maxWidthPx, rows)
	case ImageProtocolKitty:
		return renderKitty(w, pngData, maxWidthPx, rows)
	case ImageProtocolSixel:
		return renderSixel(w, pngData, maxWidthPx, rows)
	default:
		_, err := fmt.Fprintln(w, url)
		return err
	}
}

// renderITerm2 emits the OSC 1337 inline image protocol.
// iTerm2 expects PNG data, base64 encoded.
func renderITerm2(w io.Writer, pngData []byte, maxWidthPx, rows int) error {
	b64 := base64.StdEncoding.EncodeToString(pngData)
	// iTerm2: inline=1;preserveAspectRatio=1;width=Npx;height=Mc (M=rows in cells)
	opts := fmt.Sprintf("inline=1;preserveAspectRatio=1;width=%dpx;height=%dc", maxWidthPx, rows)
	fmt.Fprintf(w, "\x1b]1337;File=%s:%s\x07", opts, b64)
	return nil
}

const kittyChunk = 4096

// renderKitty emits the Kitty graphics protocol: transmit (chunked) then place+display.
// Kitty expects PNG data, base64 encoded, sent in chunks.
// a=T (transmit), f=100 (PNG), s=width, v=height, i=image_id
func renderKitty(w io.Writer, pngData []byte, maxWidthPx, rows int) error {
	b64 := base64.StdEncoding.EncodeToString(pngData)
	// Use a simple image ID based on data hash for potential reuse
	imageID := 1

	// Transmit: send chunks with m=1 (more), last with m=0
	// First chunk includes all parameters
	fmt.Fprintf(w, "\x1b_Ga=T,f=100,i=%d,s=%d,v=%d,m=1;\x1b\\", imageID, maxWidthPx, rows*20)
	for off := 0; off < len(b64); off += kittyChunk {
		end := off + kittyChunk
		if end > len(b64) {
			end = len(b64)
		}
		last := end == len(b64)
		m := 1
		if last {
			m = 0
		}
		// Subsequent chunks only need a=T,m=X
		fmt.Fprintf(w, "\x1b_Ga=T,m=%d;%s\x1b\\", m, b64[off:end])
	}
	// Place: a=p (place), d=1 (delete after display), r=rows, q=2 (position at cursor), i=image_id
	fmt.Fprintf(w, "\x1b_Ga=p,i=%d,d=1,r=%d,q=2;\x1b\\", imageID, rows)
	return nil
}

// renderSixel converts the image to sixel via chafa (if installed).
// We pass PNG data to chafa for consistent output.
func renderSixel(w io.Writer, pngData []byte, maxWidthPx, rows int) error {
	if _, err := exec.LookPath("chafa"); err != nil {
		return fmt.Errorf("sixel needs chafa installed")
	}
	cols := maxWidthPx / 8
	if cols < 1 {
		cols = 1
	}
	cmd := exec.Command("chafa", "-f", "sixels", "--size", fmt.Sprintf("%dx%d", cols, rows), "-")
	cmd.Stdin = bytes.NewReader(pngData)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RenderImage (non-cell variant) kept for compatibility.
func RenderImage(w io.Writer, caps Capabilities, url string, maxWidth, maxHeight int) error {
	img, err := fetchImage(url)
	if err != nil {
		return err
	}
	pngData, err := encodePNG(img)
	if err != nil {
		return err
	}
	switch caps.ImageProtocol {
	case ImageProtocolITerm2:
		return renderITerm2(w, pngData, maxWidth, maxHeight)
	case ImageProtocolKitty:
		return renderKitty(w, pngData, maxWidth, maxHeight)
	case ImageProtocolSixel:
		return renderSixel(w, pngData, maxWidth, maxHeight)
	default:
		_, err := fmt.Fprintln(w, url)
		return err
	}
}

// RenderImageCellsToString fetches, encodes, and returns the terminal escape
// sequence for an image as a string. Useful for async rendering in TUI.
func RenderImageCellsToString(caps Capabilities, url string, maxWidthPx, rows int) (string, error) {
	img, err := fetchImage(url)
	if err != nil {
		return "", err
	}
	pngData, err := encodePNG(img)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	switch caps.ImageProtocol {
	case ImageProtocolITerm2:
		if err := renderITerm2(&buf, pngData, maxWidthPx, rows); err != nil {
			return "", err
		}
	case ImageProtocolKitty:
		if err := renderKitty(&buf, pngData, maxWidthPx, rows); err != nil {
			return "", err
		}
	case ImageProtocolSixel:
		if err := renderSixel(&buf, pngData, maxWidthPx, rows); err != nil {
			return "", err
		}
	default:
		return url, nil
	}
	return buf.String(), nil
}
