package db

type ReplyRecord struct {
	Fid string `json:"fid"`

	ReplyIndex int    `json:"reply_index"`
	AuthorName string `json:"author_name"`
	AuthorId   string `json:"author_id"`
	Content    string `json:"content"`
}

type FloorRecord struct {
	Bid string `json:"bid"`
	Pid string `json:"pid"`
	Fid string `json:"fid"`

	FloorIndex int `json:"floor_index"`

	AuthorName string `json:"author_name"`
	AuthorId   string `json:"author_id"`
	Content    string `json:"content"`
}

type PageRecord struct {
	Bid       string `json:"bid"`
	Pid       string `json:"pid"`
	PageIndex int    `json:"page_index"`
}

type BuildingRecord struct {
	Id string `json:"id"`

	Bsn int `json:"bsn"`
	Sna int `json:"sna"`

	BuildingTitle string `json:"building_title"`
	LastPageIndex int    `json:"last_page_index"`
}
