package store

import (
	"database/sql/driver"
	"encoding/json"
)

// refactor: reuse Block struct
type BlockWithoutTransactions struct {
	BlockNum   uint64 `json:"block_num" gorm:"primaryKey,index"`
	BlockHash  string `json:"block_hash" gorm:"index"`
	BlockTime  uint64 `json:"block_time"`
	ParentHash string `json:"parent_hash"`
}

func (BlockWithoutTransactions) TableName() string {
	return "blocks"
}

type Block struct {
	BlockNum     uint64            `json:"block_num" gorm:"primaryKey,index:block_num;index:num_and_hash,unique"`
	BlockHash    string            `json:"block_hash" gorm:"index:block_hash;index:num_and_hash,unique"`
	BlockTime    uint64            `json:"block_time"`
	ParentHash   string            `json:"parent_hash"`
	Transactions *BlockTransaction `json:"transactions" gorm:"type:text"`
}

type BlockTransaction []string

func (bt *BlockTransaction) Scan(src interface{}) error {
	return json.Unmarshal([]byte(src.(string)), bt)
}
func (bt *BlockTransaction) Value() (driver.Value, error) {
	val, err := json.Marshal(bt)
	return string(val), err
}

type Transaction struct {
	TxHash    string `json:"tx_hash" gorm:"primaryKey,index"`
	From      string `json:"from"`
	To        string `json:"to"`
	Nonce     uint64 `json:"nonce"`
	Data      string `json:"data"`
	Value     int64  `json:"value"`
	BlockNum  uint64 `json:"block_num" gorm:"index"`
	BlockHash string `json:"block_hash" gorm:"index"`
	Logs      *Logs  `json:"logs" gorm:"type:text"`
}

type Logs []*Log

func (log *Logs) Scan(src interface{}) error {
	return json.Unmarshal([]byte(src.(string)), log)
}
func (log *Logs) Value() (driver.Value, error) {
	val, err := json.Marshal(log)
	return string(val), err
}

type Log struct {
	Index uint   `json:"index"`
	Data  string `json:"data"`
}
