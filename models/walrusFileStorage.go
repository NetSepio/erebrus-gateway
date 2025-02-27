package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
)

type WalrusStorage struct {
	gorm.Model
	WalletAddress string          `json:"wallet_address"`
	FileBlobs     FileObjectArray `gorm:"type:jsonb" json:"file_blobs"`
}

type FileObject struct {
	Filename string `json:"filename"`
	BlobID   string `json:"blob_id"`
}

type FileObjectArray []FileObject

func (a FileObjectArray) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *FileObjectArray) Scan(value interface{}) error {
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &a)
}
