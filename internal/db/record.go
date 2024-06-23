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

	FloorIndex int    `json:"floor_index"`
	AuthorName string `json:"author_name"`
	AuthorId   string `json:"author_id"`
	Content    string `json:"content"`

	Messages []*ReplyRecord `json:"messages"`
}

type PageRecord struct {
	Bid string `json:"bid"`
	Pid string `json:"pid"`

	FloorRecords []*FloorRecord `json:"floor_records"`
}

type BuildingRecord struct {
	Id string `json:"id"`

	Bsn int `json:"bsn"`
	Sna int `json:"sna"`

	BuildingTitle string `json:"building_title"`

	PosterFloor *FloorRecord `json:"poster_floor"`

	LastPageIndex int           `json:"last_page_index"`
	Pages         []*PageRecord `json:"pages"`
}
