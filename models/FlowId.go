package models

import (
	"database/sql/driver"
)

type FlowIdType string

const (
	AUTH FlowIdType = "AUTH"
	ROLE FlowIdType = "ROLE"
)

func (fit *FlowIdType) Scan(value interface{}) error {
	*fit = FlowIdType([]byte(value.(string)))
	return nil
}

func (fit FlowIdType) Value() (driver.Value, error) {
	return string(fit), nil
}

// TODO: Make relations for field `relatedRoleId`
type FlowId struct {
	FlowIdType    FlowIdType `sql:"flow_id_type"`
	UserId        string
	FlowId        string `gorm:"primary_key"`
	RelatedRoleId string
	WalletAddress string
}
