package file

type PhotoSize struct {
	FileBase
	Width  int `json:"width"`
	Height int `json:"height"`
}
