package linkdl

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestDouyinLiveDownload(t *testing.T) {
	if os.Getenv("LIVE_TEST") == "" {
		t.Skip("set LIVE_TEST=1 to run live network tests")
	}

	urls := []string{
		"https://www.douyin.com/user/MS4wLjABAAAAzmAbAYFkBTjYpaZAMat4cE3klzc_HemLOk0PXETHyLk?from_tab_name=main&is_search=0&list_name=follow&modal_id=7691653163225993317&nt=7",
		"https://www.douyin.com/user/MS4wLjABAAAAzmAbAYFkBTjYpaZAMat4cE3klzc_HemLOk0PXETHyLk?from_tab_name=main&is_search=0&list_name=follow&modal_id=7691283525056687013&nt=7",
	}

	outDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for i, rawURL := range urls {
		t.Run(fmt.Sprintf("URL_%d", i+1), func(t *testing.T) {
			info, err := DetectAndParse(ctx, rawURL)
			if err != nil {
				t.Fatalf("PARSE ERROR: %v", err)
			}
			t.Logf("Platform: %s", info.Platform)
			t.Logf("Title: %s", info.Title)
			t.Logf("VideoURL: %s", info.VideoURL)

			if info.VideoURL == "" {
				t.Fatal("NO VIDEO URL returned")
			}

			path, err := Download(ctx, info, outDir, func(p Progress) {
				if p.Percent%25 == 0 {
					t.Logf("  downloading... %d%%", p.Percent)
				}
			})
			if err != nil {
				t.Fatalf("DOWNLOAD ERROR: %v", err)
			}
			fi, _ := os.Stat(path)
			t.Logf("SUCCESS: %s (%d bytes, %.2f MB)", path, fi.Size(), float64(fi.Size())/1024/1024)
		})
	}
}
