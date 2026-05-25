package business

import (
	"bufio"
	"context"
	"log"
	"os"
	"strings"

	"trading/data"
	"trading/model"
)

// StockInfoProvider 提供股票代码到名称的查询能力
type StockInfoProvider interface {
	// GetName 按代码查询股票名称，不存在返回空字符串
	GetName(ctx context.Context, code string) string
}

// stockInfoProvider 从 DB 查询，启动时自动从本地文件同步
type stockInfoProvider struct {
	repo data.StockInfoRepo
}

// NewStockInfoProvider 创建并初始化股票信息提供者
// 从 shell/code/ 目录下的文本文件加载代码-名称映射到数据库
func NewStockInfoProvider(repo data.StockInfoRepo) (StockInfoProvider, error) {
	p := &stockInfoProvider{repo: repo}
	if err := p.syncFromFiles(context.Background()); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *stockInfoProvider) GetName(ctx context.Context, code string) string {
	info, err := p.repo.FindByCode(ctx, code)
	if err != nil || info == nil {
		return ""
	}
	return info.Name
}

// syncFromFiles 从 shell/code/*.txt 文件读取并同步到数据库
func (p *stockInfoProvider) syncFromFiles(ctx context.Context) error {
	files := []string{"shell/code/上海.txt", "shell/code/深圳.txt"}

	var all []*model.StockInfo
	for _, path := range files {
		infos, err := p.parseFile(path)
		if err != nil {
			log.Printf("warn: parse stock info file %s failed: %v", path, err)
			continue
		}
		all = append(all, infos...)
	}

	if len(all) == 0 {
		log.Println("warn: no stock info loaded from files")
		return nil
	}

	if err := p.repo.SaveBatch(ctx, all); err != nil {
		return err
	}
	log.Printf("stock info synced: %d records", len(all))
	return nil
}

func (p *stockInfoProvider) parseFile(path string) ([]*model.StockInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var infos []*model.StockInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		infos = append(infos, &model.StockInfo{
			Code: parts[0],
			Name: parts[1],
		})
	}
	return infos, scanner.Err()
}
