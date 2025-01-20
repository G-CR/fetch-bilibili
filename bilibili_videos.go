package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type Video struct {
	Title string `json:"title"`
	Bvid  string `json:"bvid"`
}

func getUpVideos(mid string, pageSize int) []string {
	client := &http.Client{}
	bvList := make([]string, 0)
	page := 1

	for {
		url := fmt.Sprintf("https://api.bilibili.com/x/space/arc/search"+
			"?mid=%s&ps=%d&pn=%d&order=pubdate", mid, pageSize, page)

		req, _ := http.NewRequest("GET", url, nil)
		// 添加必要的请求头
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Referer", "https://space.bilibili.com")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Origin", "https://space.bilibili.com")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("请求失败: %v\n", err)
			break
		}

		// 读取响应内容
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			resp.Body.Close()
			fmt.Printf("读取响应失败: %v\n", err)
			break
		}
		resp.Body.Close()

		var result struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    struct {
				List struct {
					Vlist []struct {
						Title       string `json:"title"`
						Bvid        string `json:"bvid"`
						Pic         string `json:"pic"`
						Author      string `json:"author"`
						Description string `json:"description"`
						Duration    string `json:"duration"`
					} `json:"vlist"`
				} `json:"list"`
			} `json:"data"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			fmt.Printf("解析JSON失败: %v\n响应内容: %s\n", err, string(body))
			break
		}

		if result.Code != 0 {
			fmt.Printf("API返回错误: %s (代码: %d)\n", result.Message, result.Code)
			time.Sleep(10 * time.Second)
			continue
		}

		videos := result.Data.List.Vlist
		if len(videos) == 0 {
			break
		}

		for _, video := range videos {
			bvList = append(bvList, video.Bvid)
			fmt.Printf("获取到视频: %s (BV号: %s)\n", video.Title, video.Bvid)
		}

		page++
	}

	return bvList
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("请提供UP主的mid作为命令行参数")
		return
	}

	mid := os.Args[1]
	videos := getUpVideos(mid, 30)

	fmt.Printf("\n总共获取到 %d 个视频的BV号\n", len(videos))
	fmt.Println("所有BV号: ")
	for _, bv := range videos {
		fmt.Println(bv)
	}
}
