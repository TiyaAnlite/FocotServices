package main

type ExtraSendGiftEvent struct {
	Data struct {
		WealthLevel uint32 `json:"wealth_level"`
	}
}

type OnlineRankCount struct {
	Cmd  string `json:"cmd"`
	Data struct {
		Count  uint32 `json:"count"`
		Online uint32 `json:"online_count"`
	} `json:"data"`
}

type OnlineRankV2 struct {
	Cmd  string `json:"cmd"`
	Data struct {
		OnlineList []struct {
			UID        int64  `json:"uid,omitempty"`
			Face       string `json:"face,omitempty"`
			Score      string `json:"score"`
			Name       string `json:"uname"`
			Rank       int    `json:"rank,omitempty"`
			GuardLevel int    `json:"guard_level,omitempty"`
		} `json:"online_list"`
		RankType string `json:"rank_type"`
	} `json:"data"`
}
