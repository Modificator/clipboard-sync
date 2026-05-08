package clipboard

import (
"context"
"crypto/sha256"
"encoding/hex"
"fmt"
"os"
"os/exec"
"path/filepath"
"runtime"
"strings"
)

type Item struct {
Kind     string
MIMEType string
Text     string
Data     []byte
Hash     string
}

type Backend interface {
ReadText(context.Context) (Item, error)
WriteText(context.Context, string) error
ReadImage(context.Context) (Item, error)
WriteImage(context.Context, []byte, string) error
}

func NewBackend(imageHelperPath string) (Backend, error) {
switch runtime.GOOS {
case "linux":
return linuxBackend{}, nil
case "darwin":
return macBackend{imageHelperPath: imageHelperPath}, nil
default:
return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}
}

func HashText(text string) string {
sum := sha256.Sum256([]byte(strings.TrimRight(text, " \t\r\n")))
return hex.EncodeToString(sum[:])
}

func HashBytes(data []byte) string {
sum := sha256.Sum256(data)
return hex.EncodeToString(sum[:])
}

type linuxBackend struct{}

func (linuxBackend) ReadText(ctx context.Context) (Item, error) {
out, err := exec.CommandContext(ctx, "wl-paste", "-n", "-t", "text/plain").Output()
if err != nil {
return Item{}, err
}
text := string(out)
if text == "" {
return Item{}, fmt.Errorf("clipboard text is empty")
}
return Item{Kind: "text", MIMEType: "text/plain", Text: text, Hash: HashText(text)}, nil
}

func (linuxBackend) WriteText(ctx context.Context, text string) error {
cmd := exec.CommandContext(ctx, "wl-copy", "-t", "text/plain")
cmd.Stdin = strings.NewReader(text)
return cmd.Run()
}

func (linuxBackend) ReadImage(ctx context.Context) (Item, error) {
typesOut, err := exec.CommandContext(ctx, "wl-paste", "--list-types").Output()
if err != nil {
return Item{}, err
}
mime := firstImageType(string(typesOut))
if mime == "" {
return Item{}, fmt.Errorf("no image clipboard content")
}
data, err := exec.CommandContext(ctx, "wl-paste", "--type", mime).Output()
if err != nil {
return Item{}, err
}
if len(data) == 0 {
return Item{}, fmt.Errorf("image clipboard content is empty")
}
return Item{Kind: "image", MIMEType: mime, Data: data, Hash: HashBytes(data)}, nil
}

func (linuxBackend) WriteImage(ctx context.Context, data []byte, mime string) error {
if mime == "" {
mime = "image/png"
}
cmd := exec.CommandContext(ctx, "wl-copy", "--type", mime)
cmd.Stdin = strings.NewReader(string(data))
return cmd.Run()
}

type macBackend struct {
imageHelperPath string
}

func (m macBackend) ReadText(ctx context.Context) (Item, error) {
out, err := exec.CommandContext(ctx, "pbpaste").Output()
if err != nil {
return Item{}, err
}
text := string(out)
if text == "" {
return Item{}, fmt.Errorf("clipboard text is empty")
}
return Item{Kind: "text", MIMEType: "text/plain", Text: text, Hash: HashText(text)}, nil
}

func (m macBackend) WriteText(ctx context.Context, text string) error {
cmd := exec.CommandContext(ctx, "pbcopy")
cmd.Stdin = strings.NewReader(text)
return cmd.Run()
}

func (m macBackend) ReadImage(ctx context.Context) (Item, error) {
if m.imageHelperPath == "" {
return Item{}, fmt.Errorf("clipboard.image_helper_path is required on macOS")
}
tmpFile, err := os.CreateTemp("", "clipboard-sync-macos-*.png")
if err != nil {
return Item{}, err
}
path := tmpFile.Name()
_ = tmpFile.Close()
defer os.Remove(path)

cmd := exec.CommandContext(ctx, m.imageHelperPath, "-o", path)
if err := cmd.Run(); err != nil {
return Item{}, err
}
data, err := os.ReadFile(path)
if err != nil {
return Item{}, err
}
if len(data) == 0 {
return Item{}, fmt.Errorf("image clipboard content is empty")
}
return Item{Kind: "image", MIMEType: "image/png", Data: data, Hash: HashBytes(data)}, nil
}

func (m macBackend) WriteImage(ctx context.Context, data []byte, _ string) error {
if m.imageHelperPath == "" {
return fmt.Errorf("clipboard.image_helper_path is required on macOS")
}
tmpDir, err := os.MkdirTemp("", "clipboard-sync-write-*")
if err != nil {
return err
}
defer os.RemoveAll(tmpDir)

path := filepath.Join(tmpDir, "clipboard.png")
if err := os.WriteFile(path, data, 0o600); err != nil {
return err
}
return exec.CommandContext(ctx, m.imageHelperPath, path).Run()
}

func firstImageType(list string) string {
for _, line := range strings.Split(list, "\n") {
line = strings.TrimSpace(line)
if strings.HasPrefix(line, "image/") {
return line
}
}
return ""
}
