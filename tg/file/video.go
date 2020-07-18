package file

type Video struct {
	FileBase
	Width     int        `json:"width"`
	Height    int        `json:"height"`
	Duration  int        `json:"duration"`
	Thumbnail *PhotoSize `json:"thumbnail,omitempty"`
	MimeType  *string    `json:"mimetype,omitempty"`
}
