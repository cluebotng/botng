package model

type WPEditScore struct {
	Score          float64 `xml:"score"`
	ThinkVandalism bool    `xml:"think_vandalism"`
}

type WPEditScoreSet struct {
	WPEdit WPEditScore `xml:"WPEdit"`
}
