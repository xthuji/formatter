package binary

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
)

// Download 从指定 URL 下载文件到本地路径，并显示进度条
// url: 下载链接
// destPath: 本地保存路径
func Download(url, destPath string) error {
	// 创建 HTTP 请求
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "formatter-binary-manager/1.0")

	// 创建带超时的 HTTP 客户端
	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download failed: HTTP %d from %s", resp.StatusCode, url)
	}

	// 打开目标文件
	outFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination file: %w", err)
	}
	defer outFile.Close()

	// 创建进度条
	var bar *progressbar.ProgressBar
	if resp.ContentLength > 0 {
		bar = progressbar.NewOptions64(
			resp.ContentLength,
			progressbar.OptionSetDescription("Downloading"),
			progressbar.OptionSetWidth(20),
			progressbar.OptionShowBytes(true),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() {
				fmt.Println()
			}),
		)
	} else {
		// 未知大小时使用 spinner 模式
		bar = progressbar.NewOptions64(
			-1,
			progressbar.OptionSetDescription("Downloading"),
			progressbar.OptionSetWidth(20),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionOnCompletion(func() {
				fmt.Println()
			}),
		)
	}
	defer bar.Finish()

	// 带进度条的复制
	written, err := io.Copy(io.MultiWriter(outFile, bar), resp.Body)
	if err != nil {
		return fmt.Errorf("download body: %w", err)
	}

	if written != resp.ContentLength && resp.ContentLength > 0 {
		return fmt.Errorf("incomplete download: got %d bytes, expected %d", written, resp.ContentLength)
	}

	return nil
}
