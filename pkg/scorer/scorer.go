package scorer

// ScoreItem 单个评分项
type ScoreItem struct {
	Name     string  `json:"name"`
	Value    float64 `json:"value"`
	Score    int     `json:"score"`
	MaxScore int     `json:"max_score"`
}

// ScoreDetail 评分明细
type ScoreDetail struct {
	Total int         `json:"total"`
	Max   int         `json:"max"`
	Items []ScoreItem `json:"items"`
}

// StockScore 股票评分结果
type StockScore struct {
	Code        string       `json:"code"`
	Strategy    string       `json:"strategy"`
	ShortDetail *ScoreDetail `json:"short_detail,omitempty"`
	LongDetail  *ScoreDetail `json:"long_detail,omitempty"`
}
