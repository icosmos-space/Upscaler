package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	eventLog      = "upscale:log"
	eventProgress = "upscale:progress"
	eventStatus   = "upscale:status"
)

var progressRe = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)

type App struct {
	ctx        context.Context
	mu         sync.Mutex
	cmd        *exec.Cmd
	binPath    string
	modelsPath string
}

type AppPaths struct {
	BinPath    string `json:"binPath"`
	ModelsPath string `json:"modelsPath"`
}

type ModelInfo struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type UpscaleOptions struct {
	BinPath    string `json:"binPath"`
	ModelsPath string `json:"modelsPath"`
	InputPath  string `json:"inputPath"`
	OutputPath string `json:"outputPath"`
	ModelName  string `json:"modelName"`
	Scale      int    `json:"scale"`
	Format     string `json:"format"`
	TileSize   int    `json:"tileSize"`
	GPUID      string `json:"gpuId"`
	Compress   int    `json:"compress"`
	TTA        bool   `json:"tta"`
	Verbose    bool   `json:"verbose"`
}

type CommandResult struct {
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error"`
}

type ComparePair struct {
	Name   string `json:"name"`
	Before string `json:"before"`
	After  string `json:"after"`
}

const (
	previewMaxEdge       = 2048
	maxPreviewSourceBytes = 512 << 20 // 512 MiB：仅限制源文件读取上限
)

func NewApp() *App {
	bin, models := resolveDefaultPaths()
	return &App{
		binPath:    bin,
		modelsPath: models,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) DefaultPaths() AppPaths {
	return AppPaths{
		BinPath:    a.binPath,
		ModelsPath: a.modelsPath,
	}
}

func resolveDefaultPaths() (binPath, modelsPath string) {
	root := findAssetsRoot()
	binPath = filepath.Join(root, "bin", "upscayl-bin.exe")
	modelsPath = filepath.Join(root, "models")
	return binPath, modelsPath
}

func findAssetsRoot() string {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "assets"),
			filepath.Join(exeDir, "..", "assets"),
			filepath.Join(exeDir, "..", "..", "assets"),
		)
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, "assets"),
			filepath.Join(wd, "upscaler", "assets"),
		)
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		bin := filepath.Join(abs, "bin", "upscayl-bin.exe")
		if st, err := os.Stat(bin); err == nil && !st.IsDir() {
			return abs
		}
	}
	if wd, err := os.Getwd(); err == nil {
		return filepath.Join(wd, "assets")
	}
	return "assets"
}

func (a *App) ListModels(modelsPath string) ([]ModelInfo, error) {
	if modelsPath == "" {
		modelsPath = a.modelsPath
	}
	entries, err := os.ReadDir(modelsPath)
	if err != nil {
		return []ModelInfo{}, err
	}
	seen := map[string]struct{}{}
	out := make([]ModelInfo, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.ToLower(filepath.Ext(name)) != ".param" {
			continue
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		out = append(out, ModelInfo{Name: base, Label: humanizeModel(base)})
	}
	return out, nil
}

func humanizeModel(name string) string {
	return strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ").Replace(name))
}

func imageFilters() []runtime.FileFilter {
	return []runtime.FileFilter{{
		DisplayName: "Images (*.png;*.jpg;*.jpeg;*.webp)",
		Pattern:     "*.png;*.jpg;*.jpeg;*.webp",
	}}
}

func (a *App) SelectInputFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "选择输入图片",
		Filters: imageFilters(),
	})
}

func (a *App) SelectInputDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择输入文件夹",
	})
}

func (a *App) SelectOutputFile(defaultName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "选择输出文件",
		DefaultFilename: defaultName,
		Filters:         imageFilters(),
	})
}

func (a *App) SelectOutputDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择输出文件夹",
	})
}

func (a *App) SelectBin() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 upscayl-bin.exe",
		Filters: []runtime.FileFilter{{
			DisplayName: "Executable (*.exe)",
			Pattern:     "*.exe",
		}},
	})
}

func (a *App) SelectModelsDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择模型目录",
	})
}

func (a *App) SuggestOutput(inputPath, format string) string {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return ""
	}
	info, err := os.Stat(inputPath)
	if err != nil {
		return ""
	}
	format = normalizeFormat(format)
	if info.IsDir() {
		return filepath.Join(inputPath, "upscaled")
	}
	ext := "." + format
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	return filepath.Join(filepath.Dir(inputPath), base+"_upscaled"+ext)
}

func (a *App) PathKind(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	if info.IsDir() {
		return "dir"
	}
	return "file"
}

func (a *App) ResolveDroppedPaths(paths []string) (string, error) {
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			return path, nil
		}
		if isImageExt(filepath.Ext(path)) {
			return path, nil
		}
	}
	return "", fmt.Errorf("请拖入图片（jpg/png/webp）或文件夹")
}

func (a *App) ReadImageDataURL(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("路径为空")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("不能预览文件夹")
	}
	if !isImageExt(filepath.Ext(path)) {
		return "", fmt.Errorf("不是支持的图片格式")
	}
	if info.Size() > maxPreviewSourceBytes {
		return "", fmt.Errorf("源文件过大（>%dMB），无法加载预览", maxPreviewSourceBytes>>20)
	}

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", fmt.Errorf("解码图片失败: %w", err)
	}
	img = resizeForPreview(img, previewMaxEdge)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88}); err != nil {
		return "", fmt.Errorf("生成预览失败: %w", err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func resizeForPreview(src image.Image, maxEdge int) image.Image {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	long := w
	if h > long {
		long = h
	}
	if long <= maxEdge {
		return src
	}
	scale := float64(maxEdge) / float64(long)
	nw := int(float64(w)*scale + 0.5)
	nh := int(float64(h)*scale + 0.5)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
	return dst
}

func (a *App) ListComparePairs(inputPath, outputPath string) ([]ComparePair, error) {
	inputPath = strings.TrimSpace(inputPath)
	outputPath = strings.TrimSpace(outputPath)
	if inputPath == "" || outputPath == "" {
		return []ComparePair{}, nil
	}
	inInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}
	if !inInfo.IsDir() {
		after := outputPath
		if outInfo, err := os.Stat(outputPath); err == nil && outInfo.IsDir() {
			after = filepath.Join(outputPath, filepath.Base(inputPath))
			if _, err := os.Stat(after); err != nil {
				after = findSiblingOutput(outputPath, inputPath)
			}
		}
		if _, err := os.Stat(after); err != nil {
			return []ComparePair{}, nil
		}
		return []ComparePair{{
			Name:   filepath.Base(inputPath),
			Before: inputPath,
			After:  after,
		}}, nil
	}

	entries, err := os.ReadDir(inputPath)
	if err != nil {
		return nil, err
	}
	outInfo, err := os.Stat(outputPath)
	if err != nil || !outInfo.IsDir() {
		return []ComparePair{}, nil
	}

	pairs := make([]ComparePair, 0)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isImageExt(filepath.Ext(name)) {
			continue
		}
		before := filepath.Join(inputPath, name)
		after := findOutputForInput(outputPath, name)
		if after == "" {
			continue
		}
		pairs = append(pairs, ComparePair{
			Name:   name,
			Before: before,
			After:  after,
		})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return strings.ToLower(pairs[i].Name) < strings.ToLower(pairs[j].Name)
	})
	return pairs, nil
}

func findOutputForInput(outputDir, inputName string) string {
	exact := filepath.Join(outputDir, inputName)
	if _, err := os.Stat(exact); err == nil {
		return exact
	}
	base := strings.TrimSuffix(inputName, filepath.Ext(inputName))
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		candidate := filepath.Join(outputDir, base+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func findSiblingOutput(outputDir, inputPath string) string {
	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		candidate := filepath.Join(outputDir, base+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		upscaled := filepath.Join(outputDir, base+"_upscaled"+ext)
		if _, err := os.Stat(upscaled); err == nil {
			return upscaled
		}
	}
	return ""
}

func isImageExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	default:
		return false
	}
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func (a *App) OpenInExplorer(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("路径为空")
	}
	info, err := os.Stat(path)
	if err != nil {
		dir := filepath.Dir(path)
		return exec.Command("explorer", dir).Start()
	}
	if info.IsDir() {
		return exec.Command("explorer", path).Start()
	}
	return exec.Command("explorer", "/select,", path).Start()
}

func (a *App) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cmd != nil && a.cmd.Process != nil
}

func (a *App) CancelUpscale() error {
	a.mu.Lock()
	cmd := a.cmd
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func (a *App) RunUpscale(opts UpscaleOptions) CommandResult {
	a.mu.Lock()
	if a.cmd != nil {
		a.mu.Unlock()
		return CommandResult{Error: "已有任务正在运行"}
	}
	a.mu.Unlock()

	bin := strings.TrimSpace(opts.BinPath)
	if bin == "" {
		bin = a.binPath
	}
	models := strings.TrimSpace(opts.ModelsPath)
	if models == "" {
		models = a.modelsPath
	}
	input := strings.TrimSpace(opts.InputPath)
	output := strings.TrimSpace(opts.OutputPath)
	model := strings.TrimSpace(opts.ModelName)
	if model == "" {
		model = "upscayl-standard-4x"
	}
	scale := opts.Scale
	if scale != 2 && scale != 3 && scale != 4 {
		scale = 4
	}
	format := normalizeFormat(opts.Format)

	if _, err := os.Stat(bin); err != nil {
		return failResult(fmt.Errorf("找不到 upscayl-bin: %w", err))
	}
	if _, err := os.Stat(models); err != nil {
		return failResult(fmt.Errorf("找不到模型目录: %w", err))
	}
	inInfo, err := os.Stat(input)
	if err != nil {
		return failResult(fmt.Errorf("找不到输入路径: %w", err))
	}
	if output == "" {
		return failResult(fmt.Errorf("输出路径为空"))
	}

	if inInfo.IsDir() {
		if err := os.MkdirAll(output, 0o755); err != nil {
			return failResult(fmt.Errorf("创建输出目录失败: %w", err))
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return failResult(fmt.Errorf("创建输出目录失败: %w", err))
		}
	}

	args := []string{
		"-i", input,
		"-o", output,
		"-m", models,
		"-n", model,
		"-s", strconv.Itoa(scale),
		"-z", strconv.Itoa(scale),
		"-f", format,
		"-t", strconv.Itoa(opts.TileSize),
		"-c", strconv.Itoa(clamp(opts.Compress, 0, 100)),
	}
	if gpu := strings.TrimSpace(opts.GPUID); gpu != "" && !strings.EqualFold(gpu, "auto") {
		args = append(args, "-g", gpu)
	}
	if opts.TTA {
		args = append(args, "-x")
	}
	if opts.Verbose {
		args = append(args, "-v")
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = filepath.Dir(bin)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return failResult(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return failResult(err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.cmd = nil
		a.mu.Unlock()
	}()

	a.emitStatus("running")
	a.emitLog("执行: " + quoteCmd(bin, args))

	if err := cmd.Start(); err != nil {
		a.emitStatus("error")
		return failResult(fmt.Errorf("启动失败: %w", err))
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go a.pumpOutput(stdout, &wg)
	go a.pumpOutput(stderr, &wg)
	wg.Wait()

	waitErr := cmd.Wait()
	exit := 0
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}

	if waitErr != nil {
		if exit < 0 || strings.Contains(strings.ToLower(waitErr.Error()), "killed") {
			a.emitLog("任务已取消")
			a.emitStatus("cancelled")
			return CommandResult{OK: false, ExitCode: exit, Error: "已取消"}
		}
		a.emitLog("失败: " + waitErr.Error())
		a.emitStatus("error")
		return CommandResult{OK: false, ExitCode: exit, Error: waitErr.Error()}
	}

	a.emitProgress(100)
	a.emitLog("完成: " + output)
	a.emitStatus("done")
	return CommandResult{OK: true, ExitCode: 0}
}

func (a *App) pumpOutput(r io.Reader, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		a.emitLog(line)
		if m := progressRe.FindStringSubmatch(line); len(m) == 2 {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				a.emitProgress(v)
			}
		}
	}
}

func (a *App) emitLog(line string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, eventLog, line)
	}
}

func (a *App) emitProgress(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, eventProgress, v)
	}
}

func (a *App) emitStatus(status string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, eventStatus, status)
	}
}

func normalizeFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "jpg", "jpeg":
		return "jpg"
	case "webp":
		return "webp"
	default:
		return "png"
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func failResult(err error) CommandResult {
	return CommandResult{OK: false, ExitCode: -1, Error: err.Error()}
}

func quoteCmd(bin string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, strconv.Quote(bin))
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			parts = append(parts, strconv.Quote(a))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
