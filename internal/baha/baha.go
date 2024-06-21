package baha

type BahaThreadRecord struct {
	Bsn int `json:"bsn"`
	Sna int `json:"sna"`

	ThreadTitle string `json:"thread_title"`
}

type ReplyRecord struct {
	ReplyIndex int    `json:"reply_index"`
	AuthorName string `json:"author_name"`
	AuthorId   string `json:"author_id"`
	Content    string `json:"content"`
}

type FloorRecord struct {
	FloorIndex int    `json:"floor_index"`
	AuthorName string `json:"author_name"`
	AuthorId   string `json:"author_id"`
	Content    string `json:"content"`

	Messages []*ReplyRecord `json:"messages"`
}

type PageRecord struct {
	Bsn int `json:"bsn"`
	Sna int `json:"sna"`

	FloorRecords []*FloorRecord `json:"floor_records"`
}

type BuildingRecord struct {
	Bsn int `json:"bsn"`
	Sna int `json:"sna"`

	BuildingTitle string `json:"building_title"`

	PosterFloor *FloorRecord  `json:"poster_floor"`
	Pages       []*PageRecord `json:"pages"`
}
