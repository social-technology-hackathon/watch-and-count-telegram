package file

type FileBase struct {
	ID           string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     *int   `json:"file_size,omitempty"`
}

type File struct {
	FileBase
	FilePath *string `json:"file_path,omitempty"`
}
